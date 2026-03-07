#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const https = require("https");
const crypto = require("crypto");
const { spawnSync } = require("child_process");

const REPO = "jpvelasco/ludus";
const MAX_REDIRECTS = 5;

const PLATFORM_MAP = {
  linux: "linux",
  darwin: "darwin",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

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

function verifyChecksum(buffer, archiveName) {
  const expected = getExpectedChecksum(archiveName);
  if (!expected) {
    console.log("ludus-cli: no checksum available, skipping verification");
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
  console.log("ludus-cli: checksum verified (SHA-256)");
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

function extract(buffer, archiveName, binDir) {
  const tmpDir = path.join(__dirname, ".tmp-install");
  fs.mkdirSync(tmpDir, { recursive: true });

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
    const binaryName = process.platform === "win32" ? "ludus.exe" : "ludus";
    const extractedBinary = path.join(tmpDir, binaryName);

    if (!fs.existsSync(extractedBinary)) {
      throw new Error(`Binary ${binaryName} not found in archive`);
    }

    fs.mkdirSync(binDir, { recursive: true });
    const destBinary = path.join(binDir, binaryName);
    fs.copyFileSync(extractedBinary, destBinary);

    if (process.platform !== "win32") {
      fs.chmodSync(destBinary, 0o755);
    }
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

async function main() {
  const version = getPackageVersion();
  if (version === "0.0.0") {
    console.error("ludus-cli: skipping binary download for development version");
    return;
  }

  const archiveName = getArchiveName(version, process.platform, process.arch);
  const url = `https://github.com/${REPO}/releases/download/v${version}/${archiveName}`;
  const binDir = path.join(__dirname, "bin");

  console.log(`ludus-cli: downloading ${archiveName}...`);
  const buffer = await download(url);

  verifyChecksum(buffer, archiveName);

  console.log("ludus-cli: extracting binary...");
  extract(buffer, archiveName, binDir);

  console.log("ludus-cli: installed successfully");
}

main().catch((err) => {
  console.error(`ludus-cli: installation failed: ${err.message}`);
  process.exit(1);
});
