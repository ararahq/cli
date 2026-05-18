#!/usr/bin/env node
/**
 * Postinstall hook: downloads the platform-specific arara binary from the
 * matching GitHub release and unpacks it into ./bin/arara so the npm `bin`
 * entry can resolve. Mirrors the asset naming used by goreleaser:
 *   arara_<version>_<os>_<arch>.tar.gz   (linux/darwin)
 *   arara_<version>_<os>_<arch>.zip      (windows)
 */

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const https = require("node:https");
const { spawnSync } = require("node:child_process");
const zlib = require("node:zlib");

const REPO = "ararahq/cli";
const PKG_VERSION = require("./package.json").version;
const TARGET_DIR = path.join(__dirname, "bin");
const TARGET_BIN = path.join(TARGET_DIR, process.platform === "win32" ? "arara.exe" : "arara");

const PLATFORM_MAP = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

async function main() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];
  if (!platform || !arch) {
    fail(`Unsupported platform/arch: ${process.platform}/${process.arch}`);
  }

  const ext = platform === "windows" ? "zip" : "tar.gz";
  const archiveName = `arara_${PKG_VERSION}_${platform}_${arch}.${ext}`;
  const url = `https://github.com/${REPO}/releases/download/v${PKG_VERSION}/${archiveName}`;

  console.log(`[arara] downloading ${url}`);
  fs.mkdirSync(TARGET_DIR, { recursive: true });

  const archivePath = path.join(TARGET_DIR, archiveName);
  await download(url, archivePath);

  if (ext === "zip") {
    extractZip(archivePath, TARGET_DIR);
  } else {
    extractTarGz(archivePath, TARGET_DIR);
  }

  fs.chmodSync(TARGET_BIN, 0o755);
  fs.unlinkSync(archivePath);
  console.log(`[arara] installed to ${TARGET_BIN}`);
}

function download(url, destination) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(destination);
    const request = https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        file.close();
        fs.unlinkSync(destination);
        return resolve(download(response.headers.location, destination));
      }
      if (response.statusCode !== 200) {
        file.close();
        fs.unlinkSync(destination);
        return reject(new Error(`HTTP ${response.statusCode} from ${url}`));
      }
      response.pipe(file);
      file.on("finish", () => file.close(resolve));
    });
    request.on("error", (err) => {
      file.close();
      fs.unlinkSync(destination);
      reject(err);
    });
  });
}

function extractTarGz(archivePath, destDir) {
  // Defer to system tar — Node has no built-in tar reader and we don't want
  // to pull a runtime dep just for postinstall. tar -xzf works on Linux,
  // macOS, and Windows (recent builds ship bsdtar).
  const result = spawnSync("tar", ["-xzf", archivePath, "-C", destDir, "arara"], { stdio: "inherit" });
  if (result.status !== 0) {
    fail(`tar extraction failed: exit ${result.status}`);
  }
}

function extractZip(archivePath, destDir) {
  // PowerShell on Windows has Expand-Archive; on macOS/Linux fall back to
  // unzip if the user happens to install on a non-windows host.
  let result;
  if (process.platform === "win32") {
    result = spawnSync("powershell", ["-NoProfile", "-Command", `Expand-Archive -Force -Path '${archivePath}' -DestinationPath '${destDir}'`], { stdio: "inherit" });
  } else {
    result = spawnSync("unzip", ["-o", archivePath, "arara.exe", "-d", destDir], { stdio: "inherit" });
  }
  if (result.status !== 0) {
    fail(`zip extraction failed: exit ${result.status}`);
  }
}

function fail(message) {
  console.error(`[arara] ${message}`);
  process.exit(1);
}

main().catch((err) => fail(err.message));
