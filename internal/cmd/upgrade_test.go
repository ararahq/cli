package cmd

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeVersionTag(t *testing.T) {
	cases := map[string]string{
		"v1.2.3":  "1.2.3",
		"1.2.3":   "1.2.3",
		"":        "",
		"v":       "",
		"v0.0.1":  "0.0.1",
		"vNext":   "Next",
		"version": "ersion",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := normalizeVersionTag(input); got != want {
				t.Errorf("normalize(%q): want %q, got %q", input, want, got)
			}
		})
	}
}

func TestDecideUpgradeOutcome_FetchErrorYieldsNoReleaseInfo(t *testing.T) {
	outcome, version := decideUpgradeOutcome("1.0.0", nil, errors.New("network down"))
	if outcome != UpgradeOutcomeNoReleaseInfo {
		t.Errorf("expected NoReleaseInfo, got %v", outcome)
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestDecideUpgradeOutcome_NilReleaseYieldsNoReleaseInfo(t *testing.T) {
	outcome, _ := decideUpgradeOutcome("1.0.0", nil, nil)
	if outcome != UpgradeOutcomeNoReleaseInfo {
		t.Errorf("expected NoReleaseInfo when release is nil, got %v", outcome)
	}
}

func TestDecideUpgradeOutcome_DevBuild(t *testing.T) {
	release := &githubRelease{TagName: "v1.5.0"}
	outcome, version := decideUpgradeOutcome("dev", release, nil)
	if outcome != UpgradeOutcomeDevBuild {
		t.Errorf("expected DevBuild, got %v", outcome)
	}
	if version != "1.5.0" {
		t.Errorf("expected 1.5.0, got %q", version)
	}
}

func TestDecideUpgradeOutcome_UpToDate(t *testing.T) {
	release := &githubRelease{TagName: "v1.0.0"}
	outcome, _ := decideUpgradeOutcome("1.0.0", release, nil)
	if outcome != UpgradeOutcomeUpToDate {
		t.Errorf("expected UpToDate, got %v", outcome)
	}
}

func TestDecideUpgradeOutcome_UpdateAvailable(t *testing.T) {
	release := &githubRelease{TagName: "v2.0.0"}
	outcome, version := decideUpgradeOutcome("1.0.0", release, nil)
	if outcome != UpgradeOutcomeAvailable {
		t.Errorf("expected Available, got %v", outcome)
	}
	if version != "2.0.0" {
		t.Errorf("expected 2.0.0, got %q", version)
	}
}

func TestRunUpgrade_StubsFetchAndCompletes(t *testing.T) {
	originalFetch := fetchLatestReleaseFunc
	t.Cleanup(func() { fetchLatestReleaseFunc = originalFetch })

	fetchLatestReleaseFunc = func() (*githubRelease, error) {
		return &githubRelease{TagName: "v9.9.9", Body: "shiny", HTMLURL: "https://example.com/r"}, nil
	}

	if err := runUpgrade(nil, nil); err != nil {
		t.Errorf("runUpgrade should not propagate fetch errors, got: %v", err)
	}
}

func TestRunUpgrade_FetchFailureIsSwallowed(t *testing.T) {
	originalFetch := fetchLatestReleaseFunc
	t.Cleanup(func() { fetchLatestReleaseFunc = originalFetch })

	fetchLatestReleaseFunc = func() (*githubRelease, error) {
		return nil, errors.New("offline")
	}

	if err := runUpgrade(nil, nil); err != nil {
		t.Errorf("runUpgrade should swallow fetch errors, got: %v", err)
	}
}

func TestSelectAssets_PicksMatchingPlatform(t *testing.T) {
	assets := []releaseAsset{
		{Name: "arara_1.0.0_linux_amd64.tar.gz", DownloadURL: "https://example.com/linux"},
		{Name: "arara_1.0.0_darwin_arm64.tar.gz", DownloadURL: "https://example.com/darwin-arm"},
		{Name: "arara_1.0.0_windows_amd64.zip", DownloadURL: "https://example.com/win"},
		{Name: "checksums.txt", DownloadURL: "https://example.com/sums"},
	}

	binary, sums, err := selectAssets(assets, "1.0.0", "darwin", "arm64")
	if err != nil {
		t.Fatalf("selectAssets: %v", err)
	}
	if binary == nil || binary.Name != "arara_1.0.0_darwin_arm64.tar.gz" {
		t.Errorf("wrong binary asset selected: %+v", binary)
	}
	if sums == nil || sums.Name != "checksums.txt" {
		t.Errorf("checksums asset not picked: %+v", sums)
	}
}

func TestSelectAssets_PicksZipForWindows(t *testing.T) {
	assets := []releaseAsset{
		{Name: "arara_1.0.0_linux_amd64.tar.gz"},
		{Name: "arara_1.0.0_windows_amd64.zip"},
	}
	binary, _, err := selectAssets(assets, "1.0.0", "windows", "amd64")
	if err != nil {
		t.Fatalf("selectAssets: %v", err)
	}
	if !strings.HasSuffix(binary.Name, ".zip") {
		t.Errorf("windows asset should be .zip, got %s", binary.Name)
	}
}

func TestSelectAssets_NoMatchReturnsError(t *testing.T) {
	assets := []releaseAsset{
		{Name: "arara_1.0.0_linux_amd64.tar.gz"},
	}
	_, _, err := selectAssets(assets, "1.0.0", "darwin", "arm64")
	if err == nil {
		t.Fatal("expected error when platform asset missing")
	}
}

func TestLookupChecksum_FindsAsset(t *testing.T) {
	dir := t.TempDir()
	checksumsPath := filepath.Join(dir, "checksums.txt")
	body := strings.Join([]string{
		"abc123  arara_1.0.0_linux_amd64.tar.gz",
		"def456  arara_1.0.0_darwin_arm64.tar.gz",
		"ghi789  arara_1.0.0_windows_amd64.zip",
	}, "\n")
	if err := os.WriteFile(checksumsPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := lookupChecksum(checksumsPath, "arara_1.0.0_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got != "def456" {
		t.Errorf("want def456, got %q", got)
	}
}

func TestLookupChecksum_MissingAsset(t *testing.T) {
	dir := t.TempDir()
	checksumsPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumsPath, []byte("abc123  other.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := lookupChecksum(checksumsPath, "arara.tar.gz")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSHA256File_KnownDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	const wantHelloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != wantHelloSHA256 {
		t.Errorf("digest mismatch: want %s, got %s", wantHelloSHA256, got)
	}
}

func TestVerifyChecksum_MismatchIsErr(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "asset.tar.gz")
	if err := os.WriteFile(binPath, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	checksumsPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumsPath, []byte("deadbeef  asset.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(binPath, checksumsPath, "asset.tar.gz")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error should mention mismatch, got: %v", err)
	}
}

func TestCheckWritable_DetectsReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — cannot create unwritable dir")
	}
	dir := t.TempDir()
	readonly := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readonly, 0o500); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(readonly, "fake-binary")
	err := checkWritable(target)
	if err == nil {
		t.Fatal("expected unwritable error")
	}
}

func TestCheckWritable_HappyPath(t *testing.T) {
	dir := t.TempDir()
	if err := checkWritable(filepath.Join(dir, "fake")); err != nil {
		t.Errorf("writable dir should pass, got: %v", err)
	}
}

func TestDownloadFile_GoldenPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("payload-bytes"))
	}))
	t.Cleanup(server.Close)

	destination := filepath.Join(t.TempDir(), "downloaded")
	if err := downloadFile(server.URL+"/asset", destination); err != nil {
		t.Fatalf("download: %v", err)
	}

	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload-bytes" {
		t.Errorf("payload mismatch: %q", string(got))
	}
}

func TestDownloadFile_404IsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	destination := filepath.Join(t.TempDir(), "downloaded")
	err := downloadFile(server.URL+"/missing", destination)
	if err == nil {
		t.Fatal("expected 404 error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status, got: %v", err)
	}
}

func TestExtractFromTarGz_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "release.tar.gz")
	if err := writeTestTarGz(archivePath, map[string][]byte{
		"arara":     []byte("#!/bin/sh\necho hi\n"),
		"README.md": []byte("readme"),
		"LICENSE":   []byte("license"),
	}); err != nil {
		t.Fatalf("build archive: %v", err)
	}

	extractDir := t.TempDir()
	binaryPath, err := extractFromTarGz(archivePath, extractDir, "arara")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	body, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "echo hi") {
		t.Errorf("extracted binary content mismatch: %q", string(body))
	}

	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("extracted binary should be executable, got mode %v", info.Mode().Perm())
	}
}

func TestExtractFromTarGz_BinaryMissing(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "release.tar.gz")
	if err := writeTestTarGz(archivePath, map[string][]byte{"README.md": []byte("readme")}); err != nil {
		t.Fatalf("build archive: %v", err)
	}

	_, err := extractFromTarGz(archivePath, t.TempDir(), "arara")
	if err == nil {
		t.Fatal("expected error when binary not in archive")
	}
}

func TestCopyFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "src")
	if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(dir, "dst")
	if err := copyFile(source, destination); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, _ := os.ReadFile(destination)
	if string(got) != "payload" {
		t.Errorf("dest mismatch: %q", string(got))
	}
}

func TestReplaceBinary_SwapsAndCleansBackup(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "arara")
	if err := os.WriteFile(currentPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	newPath := filepath.Join(dir, "new-arara")
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(newPath, currentPath); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}

	got, _ := os.ReadFile(currentPath)
	if string(got) != "new" {
		t.Errorf("current binary should hold new content, got %q", string(got))
	}
	if _, statErr := os.Stat(currentPath + ".old"); !os.IsNotExist(statErr) {
		t.Errorf("backup should be removed on success, stat err: %v", statErr)
	}
}

func TestReplaceBinary_FailsWhenCurrentMissing(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "new-arara")
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(dir, "subdir-that-does-not-exist", "arara")
	if err := replaceBinary(newPath, missing); err == nil {
		t.Fatal("expected error when current binary missing")
	}
}

// writeTestTarGz creates a tar.gz archive at destination containing the
// provided files. Used by extract tests.
func writeTestTarGz(destination string, files map[string][]byte) error {
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0o755,
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write(content); err != nil {
			return err
		}
	}
	return nil
}
