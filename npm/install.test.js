"use strict";

const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const {
  getArchiveName,
  needsDownload,
  binaryName,
  MARKER,
  validateRedirectUrl,
} = require("./install.js");

test("getArchiveName: platform/arch matrix", () => {
  assert.strictEqual(
    getArchiveName("1.2.3", "win32", "x64"),
    "ludus_1.2.3_windows_amd64.zip"
  );
  assert.strictEqual(
    getArchiveName("1.2.3", "linux", "arm64"),
    "ludus_1.2.3_linux_arm64.tar.gz"
  );
  assert.strictEqual(
    getArchiveName("0.5.1", "darwin", "arm64"),
    "ludus_0.5.1_darwin_arm64.tar.gz"
  );
  assert.strictEqual(
    getArchiveName("0.5.1", "darwin", "x64"),
    "ludus_0.5.1_darwin_amd64.tar.gz"
  );
});

test("getArchiveName: unsupported platform/arch throws", () => {
  assert.throws(() => getArchiveName("1.0.0", "sunos", "x64"), /Unsupported platform/);
  assert.throws(() => getArchiveName("1.0.0", "linux", "mips"), /Unsupported platform/);
});

// The version regex lives in getPackageVersion; exercise it directly so we don't
// depend on the repo package.json value.
const VERSION_RE = /^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$/;

test("version regex: accepts valid semver", () => {
  for (const v of ["0.0.0", "1.2.3", "0.5.1", "1.0.0-rc.1", "1.2.3+build.5"]) {
    assert.ok(VERSION_RE.test(v), `expected ${v} to be valid`);
  }
});

test("version regex: rejects injection-y / malformed strings", () => {
  for (const v of [
    "1.2.3; rm -rf /",
    "../../etc/passwd",
    "v1.2.3",
    "1.2",
    "latest",
    "1.2.3 && echo hi",
  ]) {
    assert.ok(!VERSION_RE.test(v), `expected ${v} to be rejected`);
  }
});

test("needsDownload: true when binary missing", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ludus-test-"));
  try {
    assert.strictEqual(needsDownload(dir, "1.2.3"), true);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("needsDownload: true when marker missing", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ludus-test-"));
  try {
    fs.writeFileSync(path.join(dir, binaryName()), "stub");
    assert.strictEqual(needsDownload(dir, "1.2.3"), true);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("needsDownload: true when marker version mismatches (drift)", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ludus-test-"));
  try {
    fs.writeFileSync(path.join(dir, binaryName()), "stub");
    fs.writeFileSync(path.join(dir, MARKER), "1.2.2");
    assert.strictEqual(needsDownload(dir, "1.2.3"), true);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("needsDownload: false when binary present and marker matches", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ludus-test-"));
  try {
    fs.writeFileSync(path.join(dir, binaryName()), "stub");
    fs.writeFileSync(path.join(dir, MARKER), "1.2.3");
    assert.strictEqual(needsDownload(dir, "1.2.3"), false);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("needsDownload: tolerates trailing whitespace in marker", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ludus-test-"));
  try {
    fs.writeFileSync(path.join(dir, binaryName()), "stub");
    fs.writeFileSync(path.join(dir, MARKER), "1.2.3\n");
    assert.strictEqual(needsDownload(dir, "1.2.3"), false);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

// Redirect targets must stay HTTPS on a GitHub host. `location` is what the
// server returns; `currentUrl` is the URL that produced it (used to resolve
// relative Location values).
test("validateRedirectUrl: rejects bad redirects, allows GitHub hosts", () => {
  const base = "https://github.com/jpvelasco/ludus/releases/download/v1.2.3/a.tar.gz";
  const cases = [
    [
      `${base}`,
      "https://objects.githubusercontent.com/x.tar.gz?sig=abc",
      true,
    ],
    [base, "/redirect/relative", true], // relative resolves to github.com
    [
      "https://github.com/jpvelasco/ludus/releases/download/v1.2.3/a.tar.gz",
      "https://evil.example.com/x.tar.gz",
      false,
    ],
    [
      "https://objects.githubusercontent.com/x.tar.gz",
      "http://objects.githubusercontent.com/x.tar.gz",
      false, // protocol downgrade
    ],
    [
      "https://github.com/jpvelasco/ludus/releases/download/v1.2.3/a.tar.gz",
      "ftp://github.com/x",
      false, // non-https scheme
    ],
  ];
  for (const [currentUrl, location, ok] of cases) {
    if (ok) {
      const resolved = validateRedirectUrl(currentUrl, location);
      assert.ok(
        resolved.startsWith("https://"),
        `expected https resolution, got ${resolved}`
      );
    } else {
      assert.throws(
        () => validateRedirectUrl(currentUrl, location),
        /must use HTTPS|untrusted host/,
        `expected ${location} to be rejected`
      );
    }
  }
});
