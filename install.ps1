# install.ps1 — one-line installer for the AraraHQ CLI on Windows.
#
# Usage (default: latest stable, %LOCALAPPDATA%\arara\bin):
#     iwr -useb https://raw.githubusercontent.com/ararahq/cli/main/install.ps1 | iex
#
# Customize via env vars (set them BEFORE the pipe):
#     $env:ARARA_VERSION      = "v0.2.0"
#     $env:ARARA_INSTALL_DIR  = "C:\Tools\bin"
#     $env:ARARA_NO_VERIFY    = "1"   # NOT recommended
#
# Mirrors install.sh: same exit codes, same env vars, same UX. PowerShell
# 5.1+ (ships with Windows 10/11) — no extra modules required.

$ErrorActionPreference = "Stop"

$Repo   = "ararahq/cli"
$Binary = "arara.exe"

# ── Pretty output ────────────────────────────────────────────────────────
function Write-Say  ([string]$msg) { Write-Host "▸ $msg" -ForegroundColor Cyan }
function Write-Ok   ([string]$msg) { Write-Host "✓ $msg" -ForegroundColor Green }
function Write-Warn ([string]$msg) { Write-Host "! $msg" -ForegroundColor Yellow }
function Write-Die  ([string]$msg, [int]$code = 1) {
    Write-Host "✘ $msg" -ForegroundColor Red
    exit $code
}

# ── Detect arch ──────────────────────────────────────────────────────────
# Windows on ARM (Surface Pro X, etc) reports PROCESSOR_ARCHITECTURE=ARM64;
# everything else we support is amd64. Anything exotic (x86, IA64) is rejected.
function Get-Arch {
    $procArch = $env:PROCESSOR_ARCHITECTURE
    if ($procArch -eq "AMD64" -or $procArch -eq "x86_64") { return "amd64" }
    if ($procArch -eq "ARM64")                            { return "arm64" }
    Write-Die "Unsupported architecture: $procArch" 1
}

# ── Network helpers ──────────────────────────────────────────────────────
function Get-LatestTag {
    $apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
    try {
        $release = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing -Headers @{ "User-Agent" = "arara-installer" }
        return $release.tag_name
    } catch {
        Write-Die "Failed to query GitHub for latest release: $_" 2
    }
}

function Save-Download ([string]$Url, [string]$Path) {
    try {
        Invoke-WebRequest -Uri $Url -OutFile $Path -UseBasicParsing -Headers @{ "User-Agent" = "arara-installer" }
    } catch {
        Write-Die "Failed to download $Url`n$_" 2
    }
}

# ── Checksum verification ────────────────────────────────────────────────
function Test-Checksum ([string]$ArchivePath, [string]$ChecksumsPath, [string]$ArchiveName) {
    if ($env:ARARA_NO_VERIFY -eq "1") {
        Write-Warn "ARARA_NO_VERIFY=1 — skipping SHA256 verification."
        return
    }
    $expectedLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "  $([regex]::Escape($ArchiveName))$" } | Select-Object -First 1
    if (-not $expectedLine) {
        Write-Die "Checksum for $ArchiveName not found in checksums.txt" 2
    }
    $expected = ($expectedLine -split '\s+')[0].ToLower()
    $actual   = (Get-FileHash -Algorithm SHA256 $ArchivePath).Hash.ToLower()
    if ($expected -ne $actual) {
        Write-Die "Checksum mismatch — expected $expected, got $actual. Aborting." 2
    }
}

# ── Install destination ──────────────────────────────────────────────────
# Default to %LOCALAPPDATA%\arara\bin so we never need admin rights — same
# pattern Bun/Deno use on Windows. User can override via env var.
function Resolve-InstallDir {
    if ($env:ARARA_INSTALL_DIR) { return $env:ARARA_INSTALL_DIR }
    return Join-Path $env:LOCALAPPDATA "arara\bin"
}

# ── PATH hint ────────────────────────────────────────────────────────────
function Update-UserPath ([string]$InstallDir) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -and ($userPath.Split(';') -contains $InstallDir)) { return }
    Write-Warn "$InstallDir is not on your PATH."
    Write-Host "  Adding it to your User PATH (takes effect in new terminals)..."
    if ($userPath) {
        $newPath = "$userPath;$InstallDir"
    } else {
        $newPath = $InstallDir
    }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = "$env:Path;$InstallDir"   # current session too
    Write-Ok "PATH updated — reopen any other terminals to pick it up."
}

# ── Main ─────────────────────────────────────────────────────────────────
Write-Say "Installing AraraHQ CLI"

$arch = Get-Arch
$tag  = if ($env:ARARA_VERSION) { $env:ARARA_VERSION } else { Get-LatestTag }
if (-not $tag) { Write-Die "Could not determine latest release tag." 2 }
$version = $tag.TrimStart('v')

# Windows archives are zip per goreleaser format_overrides, not tar.gz.
$archiveName  = "arara_${version}_windows_${arch}.zip"
$archiveUrl   = "https://github.com/$Repo/releases/download/$tag/$archiveName"
$checksumsUrl = "https://github.com/$Repo/releases/download/$tag/checksums.txt"

Write-Say "Platform: windows/$arch    Version: $tag"

$tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "arara-install-$([guid]::NewGuid())")
try {
    $archivePath   = Join-Path $tmp $archiveName
    $checksumsPath = Join-Path $tmp "checksums.txt"

    Save-Download $archiveUrl   $archivePath
    Save-Download $checksumsUrl $checksumsPath
    Test-Checksum $archivePath  $checksumsPath $archiveName
    Write-Ok "Downloaded and verified $archiveName"

    Expand-Archive -Path $archivePath -DestinationPath $tmp -Force
    $extractedBinary = Join-Path $tmp $Binary
    if (-not (Test-Path $extractedBinary)) {
        Write-Die "Archive did not contain $Binary." 2
    }

    $installDir = Resolve-InstallDir
    if (-not (Test-Path $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }
    $target = Join-Path $installDir $Binary
    Move-Item -Path $extractedBinary -Destination $target -Force
    Write-Ok "Installed to $target"

    Update-UserPath $installDir

    try {
        $versionOutput = & $target --version
        Write-Ok $versionOutput
        Write-Host ""
        Write-Host "Next: run " -NoNewline
        Write-Host "arara login" -ForegroundColor Cyan -NoNewline
        Write-Host " to authenticate."
    } catch {
        Write-Warn "Installed binary failed to run. Open an issue at https://github.com/$Repo/issues"
        exit 2
    }
} finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
