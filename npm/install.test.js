"use strict";

const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");

const {
  getArchiveName,
  needsDownload,
  binaryName,
  MARKER,
  validateRedirectUrl,
  verifyChecksum,
  extract,
} = require("./install.js");

function extractionDir(t) {
  const dir = fs.mkdtempSync(path.join(__dirname, ".tmp-install-test-'[test]-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

test("extract: Windows ZIP ignores incompatible inherited archive modules", {
  skip: process.platform !== "win32",
}, (t) => {
  const dir = extractionDir(t);
  const moduleDir = path.join(dir, "Microsoft.PowerShell.Archive");
  fs.mkdirSync(moduleDir);
  fs.writeFileSync(path.join(moduleDir, "Microsoft.PowerShell.Archive.psm1"),
    "function Expand-Archive { Write-Error 'Incompatible archive module' }\n" +
    "Export-ModuleMember -Function Expand-Archive\n");
  const previous = process.env.PSModulePath;
  t.after(() => {
    if (previous === undefined) delete process.env.PSModulePath;
    else process.env.PSModulePath = previous;
  });
  process.env.PSModulePath = dir;

  // A stored ZIP containing ludus.exe with the contents "test binary".
  const archive = Buffer.from(
    "UEsDBBQAAAAAAEavb13pThlLCwAAAAsAAAAJAAAAbHVkdXMuZXhldGVzdCBiaW5hcnlQSwECFAAUAAAAAABGr29d6U4ZSwsAAAALAAAACQAAAAAAAAAAAAAAgAEAAAAAbHVkdXMuZXhlUEsFBgAAAAABAAEANwAAADIAAAAAAA==",
    "base64"
  );
  const binDir = path.join(dir, "bin");
  extract(archive, "ludus_test_windows_amd64.zip", binDir);
  assert.strictEqual(fs.readFileSync(path.join(binDir, "ludus.exe"), "utf8"), "test binary");
});

test("extract: Windows ZIP errors fail at extraction", {
  skip: process.platform !== "win32",
}, (t) => {
  const binDir = path.join(extractionDir(t), "bin");
  assert.throws(
    () => extract(Buffer.from("invalid ZIP"), "invalid.zip", binDir),
    /ZIP extraction exited with code [1-9]/
  );
  assert.strictEqual(fs.existsSync(binDir), false);
});

test("extract: tar.gz installs the platform binary", (t) => {
  const dir = extractionDir(t);
  const bn = binaryName();
  fs.writeFileSync(path.join(dir, bn), "test binary");
  const archivePath = path.join(dir, "test.tar.gz");
  const result = spawnSync("tar", ["-czf", archivePath, "-C", dir, bn]);
  assert.ifError(result.error);
  assert.strictEqual(result.status, 0, result.stderr?.toString());
  const binDir = path.join(dir, "bin");
  extract(fs.readFileSync(archivePath), "test.tar.gz", binDir);
  assert.strictEqual(fs.readFileSync(path.join(binDir, bn), "utf8"), "test binary");
});

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

test("verifyChecksum: fails closed when the archive has no checksum", () => {
  const archiveName = "ludus_1.2.3_linux_amd64.tar.gz";
  assert.throws(
    () => verifyChecksum(Buffer.from("archive"), archiveName, { silent: true }),
    new RegExp(`no checksum available for ${archiveName}`)
  );
});

test("verifyChecksum: rejects mismatched checksum", () => {
  const archiveName = "ludus_1.2.3_linux_amd64.tar.gz";
  assert.throws(
    () => verifyChecksum(Buffer.from("archive"), archiveName, {
      expected: "0".repeat(64),
      silent: true,
    }),
    new RegExp(`Checksum mismatch for ${archiveName}`)
  );
});

test("verifyChecksum: accepts matching checksum", () => {
  const archiveName = "ludus_1.2.3_linux_amd64.tar.gz";
  const buffer = Buffer.from("archive");
  const expected = crypto.createHash("sha256").update(buffer).digest("hex");
  assert.doesNotThrow(() =>
    verifyChecksum(buffer, archiveName, { expected, silent: true })
  );
});
