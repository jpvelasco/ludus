#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const https = require("https");
const { execSync } = require("child_process");

const REPO = "jpvelasco/ludus";

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
  return pkg.version;
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

function download(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return download(res.headers.location).then(resolve, reject);
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

async function extract(buffer, archiveName, binDir) {
  const tmpDir = path.join(__dirname, ".tmp-install");
  fs.mkdirSync(tmpDir, { recursive: true });

  const archivePath = path.join(tmpDir, archiveName);
  fs.writeFileSync(archivePath, buffer);

  try {
    if (archiveName.endsWith(".zip")) {
      // Use PowerShell on Windows, unzip on others
      if (process.platform === "win32") {
        execSync(
          `powershell -NoProfile -Command "Expand-Archive -Force '${archivePath}' '${tmpDir}'"`,
          { stdio: "pipe" }
        );
      } else {
        execSync(`unzip -o "${archivePath}" -d "${tmpDir}"`, { stdio: "pipe" });
      }
    } else {
      execSync(`tar -xzf "${archivePath}" -C "${tmpDir}"`, { stdio: "pipe" });
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

  console.log("ludus-cli: extracting binary...");
  await extract(buffer, archiveName, binDir);

  console.log("ludus-cli: installed successfully");
}

main().catch((err) => {
  console.error(`ludus-cli: installation failed: ${err.message}`);
  process.exit(1);
});
