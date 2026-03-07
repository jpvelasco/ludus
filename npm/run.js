#!/usr/bin/env node
"use strict";

const path = require("path");
const { spawn } = require("child_process");

const binaryName = process.platform === "win32" ? "ludus.exe" : "ludus";
const binaryPath = path.join(__dirname, "bin", binaryName);

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
        "Run 'npm rebuild ludus-cli' or reinstall the package."
    );
  } else {
    console.error(`ludus-cli: failed to start: ${err.message}`);
  }
  process.exit(1);
});

child.on("exit", (code, signal) => {
  process.exit(signal ? 1 : code || 0);
});
