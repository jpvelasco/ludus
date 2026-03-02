#!/usr/bin/env node
"use strict";

const path = require("path");
const { execFileSync } = require("child_process");

const binaryName = process.platform === "win32" ? "ludus.exe" : "ludus";
const binaryPath = path.join(__dirname, "bin", binaryName);

try {
  execFileSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  process.exit(err.status || 1);
}
