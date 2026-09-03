// Tests for extractPackage() in run.mjs — the hand-rolled zip extractor that
// replaced the `unzip` binary so this entrypoint can also run on a Windows
// host with no Docker (04 丙-82). unzip's own refusal of absolute paths and
// `../` entries used to be the only thing standing between a hostile skill
// package and the filesystem outside the staging directory (iron rule 1);
// every fixture below is one way that refusal must still hold now that it is
// hand-written. Every fixture carries the real CRC-32 unless a test explicitly
// lies about it, so the extractor verifies the same bytes admission approved.
//
// Every fixture is built in-process from raw zip bytes (central directory +
// local file headers, by hand) rather than committed as a binary file, so a
// reviewer can read exactly what bytes each "attack" consists of instead of
// trusting an opaque .zip in the repo.

import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "node:http";
import { test } from "node:test";
import { crc32, deflateRawSync } from "node:zlib";
import {
  agentOptions,
  extractPackage,
  gatewaySpend,
  outputContract,
  packageRoot,
  provisionPackage,
} from "./run.mjs";

test("gateway spend lookup times out instead of blocking run completion", async () => {
  const server = createServer(() => {});
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const { port } = server.address();
    const started = Date.now();
    const spend = await gatewaySpend({
      base: `http://127.0.0.1:${port}`,
      key: "test",
      initialDelayMs: 0,
      retryDelayMs: 0,
      requestTimeoutMs: 25,
      attempts: 1,
    });
    assert.equal(spend, null);
    assert.ok(Date.now() - started < 1000, "hung gateway outlived the request timeout");
  } finally {
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
});

test("package extraction failures become structured provision errors", () => {
  const root = mkdtempSync(join(tmpdir(), "skillhub-provision-"));
  const archivePath = join(root, "bad.zip");
  const destination = join(root, "dest");
  writeFileSync(archivePath, Buffer.from("not a zip"));
  mkdirSync(destination);
  try {
    assert.throws(
      () => provisionPackage(archivePath, destination, (phase, code, message) => {
        assert.equal(phase, "provision");
        assert.equal(code, "invalid_package");
        assert.match(message, /skill package extraction failed/);
        throw new Error("structured failure recorded");
      }),
      /structured failure recorded/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

// --- fixture builder ---------------------------------------------------------
//
// Mirrors exactly the field layout run.mjs's readCentralDirectory /
// readEntryData expect (ZIP local file header = 30 bytes, central directory
// header = 46 bytes, EOCD = 22 bytes, all little-endian). `compressedSizeLie`
// lets a test claim a size the real bytes don't have — that's how the zip64
// sentinel (0xffffffff) fixture is built without hand-assembling a real
// Zip64 extra field, which run.mjs never gets far enough to read anyway: the
// sentinel value alone must trip the refusal before any size is trusted.
function buildZip(entries) {
  const localParts = [];
  const centralParts = [];
  let offset = 0;

  for (const e of entries) {
    const nameBuf = Buffer.isBuffer(e.name) ? e.name : Buffer.from(e.name, "utf8");
    const extra = e.extra ?? Buffer.alloc(0);
    const data = e.data ?? Buffer.alloc(0);
    const method = e.method ?? 0; // 0 = stored, 8 = deflate
    const compressed = method === 8 ? deflateRawSync(data) : data;
    const generalFlag = e.generalFlag ?? 0;
    const externalAttr = e.externalAttr ?? 0;
    const compressedSizeField = e.compressedSizeLie ?? compressed.length;
    // `uncompressedSizeLie` is how the decompression-bomb fixture is built: the
    // central directory claims a small size while the deflate stream expands to
    // something far larger, which is the whole shape of the attack.
    const uncompressedSizeField = e.uncompressedSizeLie ?? data.length;
    const checksum = e.crcLie ?? crc32(data) >>> 0;

    const localOffset = offset;
    const lh = Buffer.alloc(30);
    lh.writeUInt32LE(0x04034b50, 0);
    lh.writeUInt16LE(20, 4); // version needed
    lh.writeUInt16LE(generalFlag, 6);
    lh.writeUInt16LE(method, 8);
    lh.writeUInt16LE(0, 10); // mod time
    lh.writeUInt16LE(0, 12); // mod date
    lh.writeUInt32LE(checksum, 14);
    lh.writeUInt32LE(compressedSizeField, 18);
    lh.writeUInt32LE(uncompressedSizeField, 22);
    lh.writeUInt16LE(nameBuf.length, 26);
    lh.writeUInt16LE(extra.length, 28);
    const localRecord = Buffer.concat([lh, nameBuf, extra, compressed]);
    localParts.push(localRecord);

    const ch = Buffer.alloc(46);
    ch.writeUInt32LE(0x02014b50, 0);
    ch.writeUInt16LE(((e.creatorSystem ?? 0) << 8) | 20, 4); // version made by
    ch.writeUInt16LE(20, 6); // version needed
    ch.writeUInt16LE(generalFlag, 8);
    ch.writeUInt16LE(method, 10);
    ch.writeUInt16LE(0, 12);
    ch.writeUInt16LE(0, 14);
    ch.writeUInt32LE(checksum, 16);
    ch.writeUInt32LE(compressedSizeField, 20);
    ch.writeUInt32LE(uncompressedSizeField, 24);
    ch.writeUInt16LE(nameBuf.length, 28);
    ch.writeUInt16LE(extra.length, 30);
    ch.writeUInt16LE(0, 32); // comment length
    ch.writeUInt16LE(0, 34); // disk number
    ch.writeUInt16LE(0, 36); // internal attr
    ch.writeUInt32LE(externalAttr, 38);
    ch.writeUInt32LE(localOffset, 42);
    centralParts.push(Buffer.concat([ch, nameBuf, extra]));

    offset += localRecord.length;
  }

  const centralDir = Buffer.concat(centralParts);
  const centralOffset = offset;
  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(0, 4);
  eocd.writeUInt16LE(0, 6);
  eocd.writeUInt16LE(entries.length, 8);
  eocd.writeUInt16LE(entries.length, 10);
  eocd.writeUInt32LE(centralDir.length, 12);
  eocd.writeUInt32LE(centralOffset, 16);
  eocd.writeUInt16LE(0, 20);

  return Buffer.concat([...localParts, centralDir, eocd]);
}

function addAdjustedPrefix(zip, prefix) {
  const oldEocd = zip.length - 22;
  const oldCentral = zip.readUInt32LE(oldEocd + 16);
  const count = zip.readUInt16LE(oldEocd + 10);
  const out = Buffer.concat([prefix, zip]);
  let pos = prefix.length + oldCentral;
  for (let i = 0; i < count; i += 1) {
    out.writeUInt32LE(out.readUInt32LE(pos + 42) + prefix.length, pos + 42);
    const nameLen = out.readUInt16LE(pos + 28);
    const extraLen = out.readUInt16LE(pos + 30);
    const commentLen = out.readUInt16LE(pos + 32);
    pos += 46 + nameLen + extraLen + commentLen;
  }
  out.writeUInt32LE(oldCentral + prefix.length, prefix.length + oldEocd + 16);
  return out;
}

const falseEntryCountZip = buildZip([
  { name: "SKILL.md", data: Buffer.from("ok") },
  { name: "hidden.txt", data: Buffer.from("ignored") },
]);
falseEntryCountZip.writeUInt16LE(1, falseEntryCountZip.length - 22 + 8);
falseEntryCountZip.writeUInt16LE(1, falseEntryCountZip.length - 22 + 10);
assertRejectedZip(
  "a central directory whose declared entry count omits records",
  falseEntryCountZip,
  /entry count mismatch/,
);

for (const [name, offset, value] of [
  ["a multi-disk EOCD", 4, 1],
  ["a central directory on another disk", 6, 1],
]) {
  const zip = buildZip([{ name: "SKILL.md", data: Buffer.from("ok") }]);
  zip.writeUInt16LE(value, zip.length - 22 + offset);
  assertRejectedZip(name, zip, /unsupported zip feature/);
}

// Writes a fixture zip to its own temp dir and returns {archivePath, destDir},
// both under a fresh mkdtemp so tests never share or leak state.
function stageZip(entries) {
  const root = mkdtempSync(join(tmpdir(), "run-test-"));
  const archivePath = join(root, "skill.zip");
  writeFileSync(archivePath, buildZip(entries));
  const destDir = join(root, "dest");
  mkdirSync(destDir, { recursive: true });
  return { archivePath, destDir, root };
}

function assertRejected(name, entries, messagePattern) {
  test(`refuses ${name}`, () => {
    const { archivePath, destDir, root } = stageZip(entries);
    try {
      assert.throws(() => extractPackage(archivePath, destDir), messagePattern);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
}

function assertRejectedZip(name, bytes, messagePattern) {
  test(`refuses ${name}`, () => {
    const root = mkdtempSync(join(tmpdir(), "run-test-"));
    try {
      const archivePath = join(root, "skill.zip");
      const destDir = join(root, "dest");
      writeFileSync(archivePath, bytes);
      mkdirSync(destDir, { recursive: true });
      assert.throws(() => extractPackage(archivePath, destDir), messagePattern);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
}

// --- 1. absolute paths --------------------------------------------------------

assertRejected(
  "a posix-absolute path",
  [{ name: "/etc/passwd", data: Buffer.from("x") }],
  /unsafe zip entry path/,
);
assertRejected(
  "a Windows drive-absolute path",
  [{ name: "C:\\Windows\\evil.txt", data: Buffer.from("x") }],
  /unsafe zip entry path/,
);
assertRejected(
  "a UNC path",
  [{ name: "\\\\server\\share\\evil.txt", data: Buffer.from("x") }],
  /unsafe zip entry path/,
);

// --- 2. ".." path segments, including the backslash variant -------------------

assertRejected(
  "a forward-slash ../ traversal",
  [{ name: "../../etc/passwd", data: Buffer.from("x") }],
  /unsafe zip entry path/,
);
assertRejected(
  "a ../ traversal nested past the entry's own directory",
  [{ name: "a/../../etc/passwd", data: Buffer.from("x") }],
  /unsafe zip entry path/,
);
assertRejected(
  "a backslash ..\\ traversal (harmless-looking on POSIX, real on Windows)",
  [{ name: "..\\evil.txt", data: Buffer.from("x") }],
  /unsafe zip entry path/,
);
assertRejected(
  "an ordinary backslash path that admission would normalize differently",
  [{ name: "scripts\\run.sh", data: Buffer.from("x") }],
  /unsafe zip entry path/,
);
for (const name of ["NUL", "con.txt", "dir/COM1.log", "file:stream", "bad?.txt", "control\u0001.txt"]) {
  assertRejected(
    `the Windows-nonportable name ${JSON.stringify(name)}`,
    [{ name, data: Buffer.from("x") }],
    /non-canonical zip entry name/,
  );
}

// --- 3. non-regular file entries ----------------------------------------------

const S_IFLNK = 0xa000;
const S_IFCHR = 0x2000;
const S_IFIFO = 0x1000;

assertRejected(
  "a symlink entry",
  [
    {
      name: "link",
      data: Buffer.from("/etc/passwd"),
      externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0,
      creatorSystem: 3,
    },
  ],
  /non-regular zip entry/,
);
assertRejected(
  "a device entry",
  [
    {
      name: "dev",
      data: Buffer.alloc(0),
      externalAttr: ((S_IFCHR | 0o666) << 16) >>> 0,
      creatorSystem: 3,
    },
  ],
  /non-regular zip entry/,
);
assertRejected(
  "a fifo entry",
  [
    {
      name: "fifo",
      data: Buffer.alloc(0),
      externalAttr: ((S_IFIFO | 0o644) << 16) >>> 0,
      creatorSystem: 3,
    },
  ],
  /non-regular zip entry/,
);

test("preserves executable mode bits from a Unix-created archive", {
  skip: process.platform === "win32",
}, () => {
  const { archivePath, destDir, root } = stageZip([
    {
      name: "run.sh",
      data: Buffer.from("#!/bin/sh\n"),
      externalAttr: ((0x8000 | 0o755) << 16) >>> 0,
      creatorSystem: 3,
    },
  ]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(statSync(join(destDir, "run.sh")).mode & 0o777, 0o755);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("does not interpret DOS external attributes as Unix file modes", () => {
  const { archivePath, destDir, root } = stageZip([
    {
      name: "ordinary.txt",
      data: Buffer.from("ok"),
      externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0,
      creatorSystem: 0,
    },
  ]);
  try {
    assert.doesNotThrow(() => extractPackage(archivePath, destDir));
    assert.equal(readFileSync(join(destDir, "ordinary.txt"), "utf8"), "ok");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

// --- 4. unsupported zip features fail loudly, never silently -----------------

assertRejected(
  "a zip64 entry",
  [{ name: "big.bin", data: Buffer.from("x"), compressedSizeLie: 0xffffffff }],
  /unsupported zip feature: zip64/,
);
assertRejected(
  "a truncated extra field",
  [{ name: "file.txt", data: Buffer.from("x"), extra: Buffer.from([0x34, 0x12, 0x05, 0x00]) }],
  /malformed zip: truncated extra field/,
);
assertRejected(
  "an encrypted entry",
  [{ name: "secret.txt", data: Buffer.from("x"), generalFlag: 0x1 }],
  /unsupported zip feature: encrypted/,
);
assertRejected(
  "an unsupported compression method",
  [{ name: "file.bin", data: Buffer.from("x"), method: 12 }], // 12 = bzip2, not implemented here
  /unsupported zip feature: compression method/,
);
assertRejected(
  "an entry whose CRC32 does not match its bytes",
  [{ name: "file.bin", data: Buffer.from("content"), crcLie: 1 }],
  /CRC32 mismatch/,
);
assertRejected(
  "an entry whose declared uncompressed size is too large",
  [
    {
      name: "file.bin",
      data: Buffer.from("content"),
      method: 8,
      uncompressedSizeLie: 20,
    },
  ],
  /uncompressed size mismatch/,
);
assertRejected(
  "an entry larger than admission's 10 MiB ceiling",
  [{ name: "large.bin", data: Buffer.alloc(10 * 1024 * 1024 + 1) }],
  /oversized zip entry/,
);
assertRejected(
  "a path deeper than admission's ten-segment ceiling",
  [{ name: `${"d/".repeat(11)}file.txt`, data: Buffer.from("x") }],
  /nested 11 directories deep/,
);
assertRejected("an empty entry name", [{ name: "", data: Buffer.alloc(0) }], /invalid name/);
assertRejected(
  "an entry name containing NUL",
  [{ name: "safe\0hidden", data: Buffer.from("x") }],
  /invalid name/,
);
assertRejected(
  "an entry name containing invalid UTF-8",
  [{ name: Buffer.from([0x62, 0x61, 0x64, 0xff]), data: Buffer.from("x") }],
  /invalid UTF-8 name/,
);
assertRejected(
  "a non-canonical dot-segment entry name",
  [{ name: "dir/./file.txt", data: Buffer.from("x") }],
  /non-canonical zip entry name/,
);
assertRejected(
  "portable names that collide by case",
  [
    { name: "dir/file.txt", data: Buffer.from("one") },
    { name: "DIR/FILE.TXT", data: Buffer.from("two") },
  ],
  /duplicate portable zip entry name/,
);
assertRejected(
  "a file that is an ancestor of another entry",
  [
    { name: "a", data: Buffer.from("file") },
    { name: "a/b", data: Buffer.from("child") },
  ],
  /ancestor|conflicts with a descendant/,
);
assertRejected(
  "a file added after its descendant",
  [
    { name: "a/b", data: Buffer.from("child") },
    { name: "a", data: Buffer.from("file") },
  ],
  /ancestor|conflicts with a descendant/,
);
assertRejected(
  "a path component longer than 255 UTF-8 bytes",
  [{ name: "x".repeat(256), data: Buffer.from("long") }],
  /non-canonical zip entry name/,
);
assertRejected(
  "a Unicode path component whose case fold shrinks below 255 bytes",
  [{ name: "K".repeat(100) + ".txt", data: Buffer.from("long") }],
  /non-canonical zip entry name/,
);

const zip64Extra = Buffer.alloc(4);
zip64Extra.writeUInt16LE(0x0001, 0);
assertRejected(
  "a gratuitous Zip64 extra field on a small entry",
  [{ name: "file.txt", data: Buffer.from("x"), extra: zip64Extra }],
  /zip64 entry/,
);

// --- positive case: tree preserved, content byte-exact, both codecs used -----

test("preserves the directory tree and extracts content byte-for-byte", () => {
  const binaryContent = Buffer.from(
    Array.from({ length: 500 }, (_, i) => i % 256),
  );
  const skillMd = "---\nname: demo\n---\nBody.\n";
  const entries = [
    { name: "pkg/", data: Buffer.alloc(0) },
    { name: "pkg/SKILL.md", data: Buffer.from(skillMd, "utf8"), method: 0 }, // stored
    { name: "pkg/scripts/", data: Buffer.alloc(0) },
    {
      name: "pkg/scripts/run.sh",
      data: Buffer.from("#!/bin/sh\necho hi\n"),
      method: 8,
    }, // deflated
    { name: "pkg/assets/data.bin", data: binaryContent, method: 8 }, // deflated binary
  ];
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    assert.equal(extractPackage(archivePath, destDir), "pkg/");

    assert.deepEqual(readdirSync(join(destDir, "pkg")).sort(), [
      "SKILL.md",
      "assets",
      "scripts",
    ]);
    assert.equal(
      readFileSync(join(destDir, "pkg", "SKILL.md"), "utf8"),
      skillMd,
    );
    assert.equal(
      readFileSync(join(destDir, "pkg", "scripts", "run.sh"), "utf8"),
      "#!/bin/sh\necho hi\n",
    );
    assert.deepEqual(
      readFileSync(join(destDir, "pkg", "assets", "data.bin")),
      binaryContent,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("selects the same single top-level package root that admission strips", () => {
  assert.equal(
    packageRoot([
      { name: "repo-main/" },
      { name: "repo-main/SKILL.md" },
      { name: "repo-main/scripts/run.sh" },
    ]),
    "repo-main/",
  );
  assert.equal(packageRoot([{ name: "SKILL.md" }, { name: "notes.md" }]), "");
  assert.equal(
    packageRoot([{ name: "one/SKILL.md" }, { name: "two/notes.md" }]),
    "",
  );
});

assertRejected(
  "a symlink disguised by a directory-shaped name",
  [{ name: "link/", externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0, creatorSystem: 3 }],
  /non-regular zip entry|type disagrees/,
);
assertRejected(
  "a directory mode disguised by a file-shaped name",
  [{ name: "dir", externalAttr: ((0x4000 | 0o755) << 16) >>> 0, creatorSystem: 3 }],
  /type disagrees/,
);

test("refuses an archive-level zip64 locator", () => {
  const ordinary = buildZip([{ name: "SKILL.md", data: Buffer.from("x") }]);
  const locator = Buffer.alloc(20);
  locator.writeUInt32LE(0x07064b50, 0);
  const malformed = Buffer.concat([
    ordinary.subarray(0, -22),
    locator,
    ordinary.subarray(-22),
  ]);
  const root = mkdtempSync(join(tmpdir(), "run-test-"));
  try {
    const archivePath = join(root, "skill.zip");
    const destDir = join(root, "dest");
    writeFileSync(archivePath, malformed);
    mkdirSync(destDir);
    assert.throws(() => extractPackage(archivePath, destDir), /zip64 archive/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

assertRejectedZip(
  "a self-extracting prefix whose ZIP offsets were adjusted",
  addAdjustedPrefix(
    buildZip([{ name: "SKILL.md", data: Buffer.from("x") }]),
    Buffer.from("MZ executable stub"),
  ),
  /prefixed zip archive/,
);

test("an EOCD signature inside a valid ZIP comment is not mistaken for the record", () => {
  const ordinary = buildZip([{ name: "SKILL.md", data: Buffer.from("x") }]);
  const comment = Buffer.from("comment-PK\x05\x06-tail", "binary");
  ordinary.writeUInt16LE(comment.length, ordinary.length - 2);
  const bytes = Buffer.concat([ordinary, comment]);
  const root = mkdtempSync(join(tmpdir(), "run-test-"));
  try {
    const archivePath = join(root, "skill.zip");
    const destDir = join(root, "dest");
    writeFileSync(archivePath, bytes);
    mkdirSync(destDir);
    assert.doesNotThrow(() => extractPackage(archivePath, destDir));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

// --- 5. decompression bombs and entry-count floods ---------------------------
//
// The seven refusals above are all about *where* an entry lands. These two are
// about *how much* of it there is, which nothing checked until 2026-08-29: the
// central directory's uncompressed size was read and used only for the zip64
// sentinel, and `inflateRawSync` was called with no ceiling at all. Under
// dockerdrv a cgroup caught the result; clean mode runs this in a host process
// where nothing does.

test("refuses an entry that inflates far past its declared size", () => {
  // 10 MB of zeroes deflates to a few kilobytes, and the central directory
  // claims 100 bytes. Every other check in this file passes it.
  const bomb = Buffer.alloc(10 * 1024 * 1024, 0);
  const { archivePath, destDir, root } = stageZip([
    { name: "bomb.bin", data: bomb, method: 8, uncompressedSizeLie: 100 },
  ]);
  try {
    assert.throws(
      () => extractPackage(archivePath, destDir),
      /inflates past its declared size of 100 bytes/,
    );
    // And nothing was written: the refusal has to come before the file lands.
    assert.deepEqual(readdirSync(destDir), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("accepts an entry that deflates honestly, so the bound is not just a wall", () => {
  // The same 10 MB, declared truthfully. It must extract — a bound that
  // rejected this would be a size limit, not a bomb check, and the previous
  // test would pass for the wrong reason.
  const honest = Buffer.alloc(10 * 1024 * 1024, 7);
  const { archivePath, destDir, root } = stageZip([
    { name: "big.bin", data: honest, method: 8 },
  ]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readFileSync(join(destDir, "big.bin")).length, honest.length);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("refuses a package with more entries than the limit", () => {
  // 2001 empty files: no bytes at all, and still 2001 filesystem entries.
  const entries = Array.from({ length: 2001 }, (_, i) => ({
    name: `f${i}`,
    data: Buffer.alloc(0),
  }));
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    assert.throws(
      () => extractPackage(archivePath, destDir),
      /2001 entries exceeds the 2000 entry limit/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("accepts a package at exactly the entry limit", () => {
  const entries = Array.from({ length: 2000 }, (_, i) => ({
    name: `f${i}`,
    data: Buffer.alloc(0),
  }));
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readdirSync(destDir).length, 2000);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("refuses a package whose declared sizes add up past the total limit", () => {
  // Three entries, each declaring 100 MiB and carrying nothing: 300 MiB
  // declared, past the 256 MiB bound. Nothing is inflated and nothing is
  // written — the refusal comes off the central directory alone, which is the
  // point of bounding the declaration rather than the running output.
  const tenMiB = 10 * 1024 * 1024;
  const entries = Array.from({ length: 11 }, (_, i) => ({
    name: `part${i}.bin`,
    data: Buffer.alloc(0),
    method: 8,
    uncompressedSizeLie: tenMiB,
  }));
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    assert.throws(
      () => extractPackage(archivePath, destDir),
      /declared \d+ bytes exceeds the 104857600 byte limit/,
    );
    assert.deepEqual(readdirSync(destDir), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("refuses a stored entry whose two sizes disagree", () => {
  // Method 0 means the compressed and uncompressed sizes are the same number.
  // An entry that declares one byte and carries a megabyte would otherwise slip
  // under the declared-total bound while landing a megabyte on disk.
  const payload = Buffer.alloc(1024 * 1024, 3);
  const { archivePath, destDir, root } = stageZip([
    { name: "stored.bin", data: payload, method: 0, uncompressedSizeLie: 1 },
  ]);
  try {
    assert.throws(
      () => extractPackage(archivePath, destDir),
      /stored entry declares 1 bytes but carries 1048576/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

// --- what the agent is told (04 丙-106) --------------------------------------
//
// The defect these pin is the one this repository keeps meeting: a convention
// with nothing behind it. `/out` was real in the collector and nowhere else, so
// an agent that produced a file said it had done so, the run reported
// `succeeded`, and the artifact list was empty. On a Windows host the write
// even landed outside the run (`C:\outnnouncement.md`).

test("the agent is told the directory that is actually collected", () => {
  const dir = join("C:", "runs", "abc", "out", "artifacts");
  const text = outputContract(dir);
  assert.ok(
    text.includes(dir),
    "the absolute path the platform collects has to appear verbatim",
  );
  // Naming the directory is only half of it. Without this the agent has no
  // reason to think its usual habit — writing next to the working directory —
  // costs anything, and that is the habit that produced an empty artifact list.
  assert.match(text, /discarded/);
});

test("the output contract does not steer anything except where files go", () => {
  const text = outputContract("/out/artifacts");
  // The system prompt is the one input that competes with the user's own, and
  // runs are judged on what the user asked for (02:TEST-*). If this ever grows
  // instructions about tone, format or language, that judgement stops being
  // about the user's prompt — so the disclaimer is part of the contract.
  assert.match(text, /does not change how you answer/);
});

test("the agent turn still carries every option that fails silently", () => {
  const options = agentOptions("/out/artifacts");

  // Measured against the pinned SDK (0.3.233), not reasoned about: it maps
  // `systemPrompt === undefined` to `p = ""`, so a preset object here would not
  // be "the same prompt plus a line" — it would switch the run to the entire
  // Claude Code system prompt. A string is the minimal delta from the empty one
  // this file used to send.
  assert.equal(typeof options.systemPrompt, "string");
  assert.ok(options.systemPrompt.includes("/out/artifacts"));

  // The header lists these as the Skill-load conditions, each of which fails
  // silently — the wrong settingSources discovers no skill at all, a missing
  // includePartialMessages reports every token count as zero. Until this test
  // nothing asserted any of them survived an edit.
  assert.equal(options.skills, "all");
  assert.equal(options.includePartialMessages, true);
  assert.equal(options.permissionMode, "bypassPermissions");
  assert.ok(
    !("settingSources" in options),
    "settingSources must stay omitted; passing it loads no skills",
  );
  assert.deepEqual(options.allowedTools, [
    "Skill",
    "Read",
    "Write",
    "Edit",
    "Glob",
    "Grep",
    "Bash",
  ]);
});
