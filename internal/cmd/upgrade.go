package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/version"
)

const (
	githubReleasesURL       = "https://api.github.com/repos/ararahq/cli/releases/latest"
	githubRequestTimeout    = 10 * time.Second
	githubAcceptHeader      = "application/vnd.github+json"
	upgradeResultLabelWidth = 14
	devVersionIdentifier    = "dev"

	upgradeDownloadTimeout = 60 * time.Second
	checksumsAssetName     = "checksums.txt"
	binaryNameUnix         = "arara"
	binaryNameWindows      = "arara.exe"
	executablePerms        = 0o755

	// maxExtractedBytes caps how much we'll read out of a release archive
	// for any single file. The arara binary is currently ~14MB; 200MB
	// leaves headroom for future bundled assets while bounding the worst
	// case if a MITM serves a decompression bomb (gosec G110).
	maxExtractedBytes int64 = 200 * 1024 * 1024
)

var upgradeInstallFlag bool

type githubRelease struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	PublishedAt string         `json:"published_at"`
	HTMLURL     string         `json:"html_url"`
	Assets      []releaseAsset `json:"assets,omitempty"`
}

type releaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// fetchLatestReleaseFunc is the indirection used by runUpgrade so tests can
// stub the GitHub call without spinning up a real httptest.Server.
var fetchLatestReleaseFunc = fetchLatestRelease

// UpgradeOutcome enumerates the conclusions runUpgrade can reach about the
// installed binary versus the latest release. Used by tests to assert
// branching without rendering full output.
type UpgradeOutcome int

const (
	UpgradeOutcomeUnknown UpgradeOutcome = iota
	UpgradeOutcomeUpToDate
	UpgradeOutcomeAvailable
	UpgradeOutcomeDevBuild
	UpgradeOutcomeNoReleaseInfo
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Check for and install CLI updates",
	Long:  "Check for the latest version of the Arara CLI from GitHub releases.\nIf a newer version is available, shows the changelog and download instructions.",
	RunE:  runUpgrade,
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeInstallFlag, "install", false, "download and install the latest binary in place")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(command *cobra.Command, arguments []string) error {
	currentVersion := version.Version

	output.PrintInfo(fmt.Sprintf("Current version: %s", currentVersion))
	output.PrintInfo("Checking for updates...")

	release, fetchError := fetchLatestReleaseFunc()
	outcome, latestVersion := decideUpgradeOutcome(currentVersion, release, fetchError)

	switch outcome {
	case UpgradeOutcomeNoReleaseInfo:
		handleNoReleaseFound(currentVersion)
	case UpgradeOutcomeDevBuild:
		printDevVersionMessage(release, latestVersion)
	case UpgradeOutcomeUpToDate:
		output.PrintSuccess("You're running the latest version!")
	case UpgradeOutcomeAvailable:
		if upgradeInstallFlag {
			return installUpgrade(release, latestVersion)
		}
		printUpdateAvailable(release, latestVersion)
	}

	return nil
}

// installUpgrade downloads the release asset matching the current OS/arch,
// verifies the checksum against checksums.txt, extracts the binary, and
// atomically replaces the running binary on disk. Falls back to instructing
// the user when the destination is not writable (likely needs sudo).
func installUpgrade(release *githubRelease, latestVersion string) error {
	output.PrintInfo(fmt.Sprintf("Downloading arara %s for %s/%s...", latestVersion, runtime.GOOS, runtime.GOARCH))

	binaryAsset, checksumsAsset, selectError := selectAssets(release.Assets, latestVersion, runtime.GOOS, runtime.GOARCH)
	if selectError != nil {
		output.PrintError(selectError.Error())
		return selectError
	}

	currentBinaryPath, exeError := resolveCurrentBinaryPath()
	if exeError != nil {
		return exeError
	}

	tempDir, mkErr := os.MkdirTemp("", "arara-upgrade-*")
	if mkErr != nil {
		return fmt.Errorf("create temp dir: %w", mkErr)
	}
	defer os.RemoveAll(tempDir)

	archivePath, downloadErr := downloadAndVerifyArchive(tempDir, binaryAsset, checksumsAsset)
	if downloadErr != nil {
		return downloadErr
	}

	return extractAndReplaceBinary(archivePath, tempDir, currentBinaryPath, latestVersion)
}

// resolveCurrentBinaryPath returns the executable path of the running CLI,
// with symlinks resolved so the rename step lands on the real file rather
// than a Homebrew/etc shim.
//
// If EvalSymlinks fails — common when the path is already not a symlink,
// or when permissions on a parent dir restrict the readlink — we treat
// that as "not a symlink, keep the raw path" rather than failing the
// whole upgrade. The os.Executable result is good enough as a fallback.
func resolveCurrentBinaryPath() (string, error) {
	exePath, exeError := os.Executable()
	if exeError != nil {
		return "", fmt.Errorf("locate current binary: %w", exeError)
	}
	resolved, evalErr := filepath.EvalSymlinks(exePath)
	if evalErr != nil {
		return exePath, nil //nolint:nilerr // intentional: fall back to raw path when symlink resolution fails
	}
	return resolved, nil
}

// downloadAndVerifyArchive fetches the release archive plus the checksums
// manifest (best-effort) and validates SHA-256 when the manifest is
// present. Returns the local path to the validated archive.
func downloadAndVerifyArchive(tempDir string, binaryAsset, checksumsAsset *releaseAsset) (string, error) {
	archivePath := filepath.Join(tempDir, binaryAsset.Name)
	if downloadErr := downloadFile(binaryAsset.DownloadURL, archivePath); downloadErr != nil {
		return "", fmt.Errorf("download binary: %w", downloadErr)
	}

	if checksumsAsset == nil {
		output.PrintWarning("No checksums.txt asset on this release — skipping verification.")
		return archivePath, nil
	}

	checksumsPath := filepath.Join(tempDir, checksumsAsset.Name)
	if downloadErr := downloadFile(checksumsAsset.DownloadURL, checksumsPath); downloadErr != nil {
		output.PrintWarning(fmt.Sprintf("Could not verify checksum: %s", downloadErr.Error()))
		return archivePath, nil
	}

	if verifyError := verifyChecksum(archivePath, checksumsPath, binaryAsset.Name); verifyError != nil {
		return "", verifyError
	}
	output.PrintSuccess("Checksum verified.")
	return archivePath, nil
}

// extractAndReplaceBinary unpacks the binary out of the archive and swaps
// it into place. On permission failure it surfaces a remediation hint
// rather than leaving the user with a half-extracted state.
func extractAndReplaceBinary(archivePath, tempDir, currentBinaryPath, latestVersion string) error {
	binaryPath, extractError := extractBinary(archivePath, tempDir)
	if extractError != nil {
		return fmt.Errorf("extract binary: %w", extractError)
	}

	if replaceError := replaceBinary(binaryPath, currentBinaryPath); replaceError != nil {
		output.PrintError(replaceError.Error())
		output.PrintInfo(fmt.Sprintf("Run with sudo or move %s manually to %s", binaryPath, currentBinaryPath))
		return replaceError
	}

	output.PrintSuccess(fmt.Sprintf("Upgraded to arara %s at %s", latestVersion, currentBinaryPath))
	return nil
}

// selectAssets picks the binary archive and (optionally) the checksums.txt
// for the running OS/arch. Asset names follow goreleaser's default template
// `arara_<version>_<os>_<arch>.<ext>`.
func selectAssets(assets []releaseAsset, version string, goos string, goarch string) (*releaseAsset, *releaseAsset, error) {
	expectedArchiveSubstring := fmt.Sprintf("_%s_%s", goos, goarch)
	expectedExtension := ".tar.gz"
	if goos == "windows" {
		expectedExtension = ".zip"
	}

	var binaryAsset *releaseAsset
	var checksumsAsset *releaseAsset
	for index := range assets {
		asset := &assets[index]
		switch {
		case asset.Name == checksumsAssetName:
			checksumsAsset = asset
		case strings.Contains(asset.Name, expectedArchiveSubstring) && strings.HasSuffix(asset.Name, expectedExtension):
			binaryAsset = asset
		}
	}

	if binaryAsset == nil {
		return nil, nil, fmt.Errorf("no asset for %s/%s in release %s", goos, goarch, version)
	}
	return binaryAsset, checksumsAsset, nil
}

func downloadFile(url string, destination string) error {
	httpClient := &http.Client{Timeout: upgradeDownloadTimeout}
	response, fetchErr := httpClient.Get(url)
	if fetchErr != nil {
		return fetchErr
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", response.StatusCode, url)
	}

	file, createErr := os.Create(destination)
	if createErr != nil {
		return createErr
	}
	defer file.Close()

	if _, copyErr := io.Copy(file, response.Body); copyErr != nil {
		return copyErr
	}
	return nil
}

// verifyChecksum compares the SHA-256 of `archivePath` against the digest
// listed for `assetName` in a goreleaser-style checksums.txt file.
func verifyChecksum(archivePath string, checksumsPath string, assetName string) error {
	wanted, lookupErr := lookupChecksum(checksumsPath, assetName)
	if lookupErr != nil {
		return lookupErr
	}

	got, hashErr := sha256File(archivePath)
	if hashErr != nil {
		return fmt.Errorf("hash downloaded archive: %w", hashErr)
	}

	if got != wanted {
		return fmt.Errorf("checksum mismatch for %s — expected %s, got %s", assetName, wanted, got)
	}
	return nil
}

func lookupChecksum(checksumsPath string, assetName string) (string, error) {
	content, readErr := os.ReadFile(checksumsPath)
	if readErr != nil {
		return "", fmt.Errorf("read checksums: %w", readErr)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s in %s", assetName, checksumsPath)
}

func sha256File(path string) (string, error) {
	file, openErr := os.Open(path)
	if openErr != nil {
		return "", openErr
	}
	defer file.Close()

	hasher := sha256.New()
	if _, copyErr := io.Copy(hasher, file); copyErr != nil {
		return "", copyErr
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// extractBinary unpacks the goreleaser archive (tar.gz on unix, zip on
// windows) into `destDir` and returns the path to the `arara` binary.
func extractBinary(archivePath string, destDir string) (string, error) {
	binaryName := binaryNameUnix
	if runtime.GOOS == "windows" {
		binaryName = binaryNameWindows
	}

	if strings.HasSuffix(archivePath, ".zip") {
		return extractFromZip(archivePath, destDir, binaryName)
	}
	return extractFromTarGz(archivePath, destDir, binaryName)
}

func extractFromTarGz(archivePath string, destDir string, targetName string) (string, error) {
	file, openErr := os.Open(archivePath)
	if openErr != nil {
		return "", openErr
	}
	defer file.Close()

	gzipReader, gzipErr := gzip.NewReader(file)
	if gzipErr != nil {
		return "", gzipErr
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, headerErr := tarReader.Next()
		if headerErr == io.EOF {
			break
		}
		if headerErr != nil {
			return "", headerErr
		}
		if filepath.Base(header.Name) != targetName {
			continue
		}

		outPath := filepath.Join(destDir, targetName)
		if writeErr := writeTarEntry(tarReader, outPath, header.FileInfo().Mode()); writeErr != nil {
			return "", writeErr
		}
		return outPath, nil
	}
	return "", fmt.Errorf("binary %s not found in archive", targetName)
}

func writeTarEntry(reader io.Reader, destination string, mode os.FileMode) error {
	out, createErr := os.Create(destination)
	if createErr != nil {
		return createErr
	}
	defer out.Close()
	if copyErr := copyBounded(out, reader, destination); copyErr != nil {
		return copyErr
	}
	return os.Chmod(destination, mode|executablePerms)
}

// copyBounded streams up to maxExtractedBytes from src to dst and errors
// if more data is available. Defends against archive-based decompression
// bombs (gosec G110): a malicious release archive could otherwise blow
// the disk/RAM of a user running `arara upgrade --install`.
func copyBounded(dst io.Writer, src io.Reader, label string) error {
	limited := io.LimitReader(src, maxExtractedBytes+1)
	written, copyErr := io.Copy(dst, limited)
	if copyErr != nil {
		return copyErr
	}
	if written > maxExtractedBytes {
		return fmt.Errorf("archive entry %s exceeds %d bytes — refusing to extract suspect payload", label, maxExtractedBytes)
	}
	return nil
}

func extractFromZip(archivePath string, destDir string, targetName string) (string, error) {
	reader, openErr := zip.OpenReader(archivePath)
	if openErr != nil {
		return "", openErr
	}
	defer reader.Close()

	for _, file := range reader.File {
		if filepath.Base(file.Name) != targetName {
			continue
		}
		zipFile, zipErr := file.Open()
		if zipErr != nil {
			return "", zipErr
		}

		outPath := filepath.Join(destDir, targetName)
		out, createErr := os.Create(outPath)
		if createErr != nil {
			_ = zipFile.Close()
			return "", createErr
		}
		copyErr := copyBounded(out, zipFile, targetName)
		_ = zipFile.Close()
		_ = out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		_ = os.Chmod(outPath, executablePerms)
		return outPath, nil
	}
	return "", fmt.Errorf("binary %s not found in zip", targetName)
}

// replaceBinary swaps the new binary into place atomically. Uses rename on
// the same volume; falls back to a copy when crossing devices (Linux /tmp
// on a different fs). Aborts when the destination is not writable so the
// caller can suggest sudo without leaving the filesystem in a half state.
func replaceBinary(newPath string, currentPath string) error {
	if _, statErr := os.Stat(currentPath); statErr != nil {
		return fmt.Errorf("stat current binary at %s: %w", currentPath, statErr)
	}

	if writableErr := checkWritable(currentPath); writableErr != nil {
		return writableErr
	}

	backupPath := currentPath + ".old"
	_ = os.Remove(backupPath)
	if renameErr := os.Rename(currentPath, backupPath); renameErr != nil {
		return fmt.Errorf("backup current binary: %w", renameErr)
	}

	if renameErr := os.Rename(newPath, currentPath); renameErr != nil {
		// Cross-device rename failed — fall back to copy + remove.
		if copyErr := copyFile(newPath, currentPath); copyErr != nil {
			_ = os.Rename(backupPath, currentPath)
			return fmt.Errorf("install new binary: %w", copyErr)
		}
	}

	if chmodErr := os.Chmod(currentPath, executablePerms); chmodErr != nil {
		return fmt.Errorf("chmod new binary: %w", chmodErr)
	}

	_ = os.Remove(backupPath)
	return nil
}

func checkWritable(path string) error {
	dir := filepath.Dir(path)
	probe, openErr := os.OpenFile(filepath.Join(dir, ".arara-upgrade-probe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if openErr != nil {
		return fmt.Errorf("destination %s is not writable: %w", dir, openErr)
	}
	probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

func copyFile(source string, destination string) error {
	in, openErr := os.Open(source)
	if openErr != nil {
		return openErr
	}
	defer in.Close()

	out, createErr := os.Create(destination)
	if createErr != nil {
		return createErr
	}
	defer out.Close()

	if _, copyErr := io.Copy(out, in); copyErr != nil {
		return copyErr
	}
	return nil
}

// decideUpgradeOutcome separates the version-comparison policy from the I/O
// wrapper around it so tests can drive each branch with synthetic inputs.
func decideUpgradeOutcome(currentVersion string, release *githubRelease, fetchError error) (UpgradeOutcome, string) {
	if fetchError != nil || release == nil {
		return UpgradeOutcomeNoReleaseInfo, ""
	}

	latestVersion := normalizeVersionTag(release.TagName)

	if currentVersion == devVersionIdentifier {
		return UpgradeOutcomeDevBuild, latestVersion
	}

	if latestVersion == currentVersion {
		return UpgradeOutcomeUpToDate, latestVersion
	}

	return UpgradeOutcomeAvailable, latestVersion
}

func fetchLatestRelease() (*githubRelease, error) {
	httpClient := &http.Client{
		Timeout: githubRequestTimeout,
	}

	request, requestError := http.NewRequest(http.MethodGet, githubReleasesURL, nil)
	if requestError != nil {
		return nil, fmt.Errorf("failed to create GitHub request: %w", requestError)
	}

	request.Header.Set("Accept", githubAcceptHeader)

	response, responseError := httpClient.Do(request)
	if responseError != nil {
		return nil, fmt.Errorf("failed to reach GitHub releases API: %w", responseError)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", response.StatusCode)
	}

	bodyBytes, readError := io.ReadAll(response.Body)
	if readError != nil {
		return nil, fmt.Errorf("failed to read GitHub response: %w", readError)
	}

	var release githubRelease
	if unmarshalError := json.Unmarshal(bodyBytes, &release); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse GitHub release: %w", unmarshalError)
	}

	if release.TagName == "" {
		return nil, fmt.Errorf("no release tag found in GitHub response")
	}

	return &release, nil
}

func normalizeVersionTag(tag string) string {
	if len(tag) > 0 && tag[0] == 'v' {
		return tag[1:]
	}

	return tag
}

func handleNoReleaseFound(currentVersion string) {
	if currentVersion == devVersionIdentifier {
		output.PrintSuccess("You're running the latest version (dev)")
		return
	}

	output.PrintWarning("Could not check for updates (no releases found or network issue)")
	output.PrintInfo("You can check manually at https://github.com/ararahq/cli/releases")
}

func printDevVersionMessage(release *githubRelease, latestVersion string) {
	output.PrintInfo(fmt.Sprintf("You're running a dev build. Latest release: %s", latestVersion))
	output.PrintInfo(fmt.Sprintf("Download at: %s", release.HTMLURL))
}

func printUpdateAvailable(release *githubRelease, latestVersion string) {
	output.PrintWarning(fmt.Sprintf("A new version is available: %s", latestVersion))
	fmt.Println()

	if release.Body != "" {
		fmt.Println(output.BoldStyle.Render("Changelog:"))
		fmt.Println(release.Body)
		fmt.Println()
	}

	output.PrintInfo(fmt.Sprintf("Download at: %s", release.HTMLURL))
	output.PrintInfo("Or run: brew upgrade arara (if installed via Homebrew)")
}
