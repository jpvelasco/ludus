#!/usr/bin/env node
"use strict";

// embed-checksums.js — Embeds SHA-256 checksums from GoReleaser's checksums.txt
// into npm/package.json's binaryChecksums field.
//
// Usage: node scripts/embed-checksums.js dist/checksums.txt npm/package.json
//
// GoReleaser checksums.txt format:
//   <sha256hex>  <filename>

const fs = require("fs");

const [checksumFile, packageFile] = process.argv.slice(2);
if (!checksumFile || !packageFile) {
  console.error("Usage: node embed-checksums.js <checksums.txt> <package.json>");
  process.exit(1);
}

const checksums = fs.readFileSync(checksumFile, "utf8");
const pkg = JSON.parse(fs.readFileSync(packageFile, "utf8"));

pkg.binaryChecksums = {};
for (const line of checksums.split("\n")) {
  const match = line.match(/^([0-9a-f]{64})\s+(.+)$/);
  if (match) {
    const [, hash, filename] = match;
    pkg.binaryChecksums[filename] = hash;
  }
}

const count = Object.keys(pkg.binaryChecksums).length;
if (count === 0) {
  console.error("Warning: no checksums found in " + checksumFile);
  process.exit(1);
}

fs.writeFileSync(packageFile, JSON.stringify(pkg, null, 2) + "\n");
console.log(`Embedded ${count} checksums into ${packageFile}`);
