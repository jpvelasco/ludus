#!/usr/bin/env node
"use strict";

const path = require("path");
const { spawn } = require("child_process");
const { ensureBinary, binaryName } = require("./install.js");

const binaryPath = path.join(__dirname, "bin", binaryName());

function spawnBinary() {
  const child = spawn(binaryPath, process.argv.slice(2), {
    stdio: "inherit",
  });

  // Forward signals so the Go binary shuts down cleanly
  ["SIGINT", "SIGTERM", "SIGHUP"].forEach((sig) => {
    process.on(sig, () => child.kill(sig));
  });

  child.on("error", (err) => {
    if (err.code === "ENOENT") {
      console.error(
        `ludus-cli: binary not found at ${binaryPath}\n` +
          "Reinstall with: npm install -g ludus-cli@latest"
      );
    } else {
      console.error(`ludus-cli: failed to start: ${err.message}`);
    }
    process.exit(1);
  });

  child.on("exit", (code, signal) => {
    process.exit(signal ? 1 : code || 0);
  });
}

async function main() {
  // Escape hatch: deliberately manage the binary yourself / air-gapped setups.
  // Skip the self-heal and spawn whatever is present (ENOENT guidance if not).
  if (process.env.LUDUS_SKIP_AUTO_DOWNLOAD) {
    spawnBinary();
    return;
  }

  try {
    // Self-heal: if postinstall was skipped (ignore-scripts, pnpm), failed
    // mid-download, or the binary drifted from this package version, fetch the
    // correct one. No-op (cheap file reads) when already in sync. silent:true
    // keeps routine chatter off; an actual download still prints one stderr line.
    await ensureBinary({ silent: true });
  } catch (err) {
    const code = err && err.code;
    if (code === "EACCES" || code === "EPERM") {
      console.error(
        `ludus-cli: cannot write the ludus binary (permission denied).\n` +
          "Reinstall with appropriate privileges:\n" +
          "  sudo npm install -g ludus-cli@latest    (macOS/Linux)\n" +
          "  run your shell as Administrator, then the same command (Windows)"
      );
    } else {
      console.error(
        `ludus-cli: could not fetch the ludus binary: ${err.message}\n` +
          "Check your network/proxy and retry. If you manage the binary yourself,\n" +
          "set LUDUS_SKIP_AUTO_DOWNLOAD=1 to bypass this step."
      );
    }
    process.exit(1);
  }

  spawnBinary();
}

main();
