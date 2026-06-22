#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const https = require("https");
const crypto = require("crypto");
const { spawnSync } = require("child_process");

const REPO = "jpvelasco/ludus";
const MAX_REDIRECTS = 5;
const MARKER = ".installed-version";

const PLATFORM_MAP = {
  linux: "linux",
  darwin: "darwin",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

// log writes routine progress to stderr (never stdout) so it can't corrupt
// `ludus mcp` JSON-RPC or `--json` output, and stays quiet when silent.
function log(silent, msg) {
  if (!silent) {
    console.error(msg);
  }
}

function binaryName(platform = process.platform) {
  return platform === "win32" ? "ludus.exe" : "ludus";
}

function getPackageVersion() {
  const pkg = JSON.parse(
    fs.readFileSync(path.join(__dirname, "package.json"), "utf8")
  );
  const version = pkg.version;

  // Validate semver format to prevent URL injection
  if (!/^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$/.test(version)) {
    throw new Error(`Invalid version format: ${version}`);
  }

  return version;
}

function getExpectedChecksum(archiveName) {
  const pkg = JSON.parse(
    fs.readFileSync(path.join(__dirname, "package.json"), "utf8")
  );
  return pkg.binaryChecksums?.[archiveName] || null;
}

function getArchiveName(version, platform, arch) {
  const os = PLATFORM_MAP[platform];
  const cpu = ARCH_MAP[arch];
  if (!os || !cpu) {
    throw new Error(`Unsupported platform: ${platform}/${arch}`);
  }
  const ext = platform === "win32" ? "zip" : "tar.gz";
  return `ludus_${version}_${os}_${cpu}.${ext}`;
}

// needsDownload reports whether the binary must be (re)fetched: true when the
// binary is missing, or when the recorded marker version doesn't match the
// package version (drift after a skipped/failed install or an upgrade).
function needsDownload(binDir, version) {
  if (!fs.existsSync(path.join(binDir, binaryName()))) {
    return true;
  }
  try {
    const installed = fs.readFileSync(path.join(binDir, MARKER), "utf8").trim();
    return installed !== version;
  } catch {
    return true;
  }
}

function download(url, redirectCount = 0) {
  return new Promise((resolve, reject) => {
    if (redirectCount > MAX_REDIRECTS) {
      return reject(new Error(`Too many redirects (max ${MAX_REDIRECTS})`));
    }

    https
      .get(url, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return download(res.headers.location, redirectCount + 1).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`Download failed: HTTP ${res.statusCode} for ${url}`));
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

function verifyChecksum(buffer, archiveName, { silent = false } = {}) {
  const expected = getExpectedChecksum(archiveName);
  if (!expected) {
    log(silent, "ludus-cli: no checksum available, skipping verification");
    return;
  }

  const actual = crypto.createHash("sha256").update(buffer).digest("hex");
  if (actual !== expected) {
    throw new Error(
      `Checksum mismatch for ${archiveName}\n` +
        `  Expected: ${expected}\n` +
        `  Actual:   ${actual}`
    );
  }
  log(silent, "ludus-cli: checksum verified (SHA-256)");
}

// Escape a string for use inside PowerShell single quotes.
// PowerShell single-quoted strings only need '' to represent a literal '.
function psEscape(s) {
  return s.replace(/'/g, "''");
}

function spawnOrFail(cmd, args, label) {
  const result = spawnSync(cmd, args, { stdio: "pipe" });
  if (result.error) {
    throw new Error(`${label}: ${result.error.message}`);
  }
  if (result.status !== 0) {
    const stderr = result.stderr ? result.stderr.toString().trim() : "";
    throw new Error(`${label} exited with code ${result.status}${stderr ? ": " + stderr : ""}`);
  }
}

// placeBinary moves src onto dest atomically. renameSync is atomic on POSIX and
// overwrites; on Windows it refuses to overwrite an existing file, so fall back
// to removing dest first (the binary is never running during a self-heal).
function placeBinary(src, dest) {
  try {
    fs.renameSync(src, dest);
  } catch (err) {
    if (err.code === "EEXIST" || err.code === "EPERM") {
      fs.rmSync(dest, { force: true });
      fs.renameSync(src, dest);
    } else {
      throw err;
    }
  }
}

function extract(buffer, archiveName, binDir) {
  // Unique temp dir per run so concurrent invocations don't clobber each other.
  const tmpDir = fs.mkdtempSync(path.join(__dirname, ".tmp-install-"));

  const archivePath = path.join(tmpDir, archiveName);
  fs.writeFileSync(archivePath, buffer);

  try {
    if (archiveName.endsWith(".zip")) {
      if (process.platform === "win32") {
        spawnOrFail(
          "powershell",
          [
            "-NoProfile",
            "-Command",
            `Expand-Archive -Force -Path '${psEscape(archivePath)}' -DestinationPath '${psEscape(tmpDir)}'`,
          ],
          "Expand-Archive"
        );
      } else {
        spawnOrFail("unzip", ["-o", archivePath, "-d", tmpDir], "unzip");
      }
    } else {
      spawnOrFail("tar", ["-xzf", archivePath, "-C", tmpDir], "tar");
    }

    // Find the binary in the extracted files
    const bn = binaryName();
    const extractedBinary = path.join(tmpDir, bn);

    if (!fs.existsSync(extractedBinary)) {
      throw new Error(`Binary ${bn} not found in archive`);
    }

    if (process.platform !== "win32") {
      fs.chmodSync(extractedBinary, 0o755);
    }

    fs.mkdirSync(binDir, { recursive: true });
    placeBinary(extractedBinary, path.join(binDir, bn));
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

// ensureBinary makes the platform binary present and matching the package
// version, downloading it if missing or stale. It is a no-op (cheap file reads
// only) when the binary already matches. Safe to call on every CLI invocation.
// All progress goes to stderr; routine chatter is suppressed when silent.
async function ensureBinary({ silent = false } = {}) {
  const version = getPackageVersion();
  if (version === "0.0.0") {
    log(silent, "ludus-cli: skipping binary download for development version");
    return;
  }

  const binDir = path.join(__dirname, "bin");
  if (!needsDownload(binDir, version)) {
    return;
  }

  const archiveName = getArchiveName(version, process.platform, process.arch);
  const url = `https://github.com/${REPO}/releases/download/v${version}/${archiveName}`;

  // One concise notice whenever an actual download happens (even when silent),
  // so a runtime self-heal isn't a silent multi-second hang.
  console.error(`ludus-cli: fetching ludus binary v${version}...`);

  log(silent, `ludus-cli: downloading ${archiveName}...`);
  const buffer = await download(url);

  verifyChecksum(buffer, archiveName, { silent });

  log(silent, "ludus-cli: extracting binary...");
  extract(buffer, archiveName, binDir);

  // Marker is written only after the binary is in place, so a crash mid-install
  // never leaves a marker that falsely claims success.
  fs.writeFileSync(path.join(binDir, MARKER), version);

  log(silent, "ludus-cli: installed successfully");
}

if (require.main === module) {
  ensureBinary({ silent: false }).catch((err) => {
    console.error(`ludus-cli: installation failed: ${err.message}`);
    process.exit(1);
  });
}

module.exports = {
  ensureBinary,
  needsDownload,
  getArchiveName,
  getPackageVersion,
  binaryName,
  MARKER,
};
