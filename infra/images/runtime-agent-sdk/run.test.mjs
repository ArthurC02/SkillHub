// Tests for extractPackage() in run.mjs — the hand-rolled zip extractor that
// replaced the `unzip` binary so this entrypoint can also run on a Windows
// host with no Docker (04 丙-82). unzip's own refusal of absolute paths and
// `../` entries used to be the only thing standing between a hostile skill
// package and the filesystem outside the staging directory (iron rule 1);
// every fixture below is one way that refusal must still hold now that it is
// hand-written.
//
// Every fixture is built in-process from raw zip bytes (central directory +
// local file headers, by hand) rather than committed as a binary file, so a
// reviewer can read exactly what bytes each "attack" consists of instead of
// trusting an opaque .zip in the repo. CRC-32 is left as 0 throughout:
// extractPackage() does not check it (neither did the shell-out to `unzip`
// verify anything beyond what unzip itself checks), so it carries no
// information for these tests.

import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { deflateRawSync } from "node:zlib";
import { extractPackage } from "./run.mjs";

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
    const nameBuf = Buffer.from(e.name, "utf8");
    const data = e.data ?? Buffer.alloc(0);
    const method = e.method ?? 0; // 0 = stored, 8 = deflate
    const compressed = method === 8 ? deflateRawSync(data) : data;
    const generalFlag = e.generalFlag ?? 0;
    const externalAttr = e.externalAttr ?? 0;
    const compressedSizeField = e.compressedSizeLie ?? compressed.length;

    const localOffset = offset;
    const lh = Buffer.alloc(30);
    lh.writeUInt32LE(0x04034b50, 0);
    lh.writeUInt16LE(20, 4); // version needed
    lh.writeUInt16LE(generalFlag, 6);
    lh.writeUInt16LE(method, 8);
    lh.writeUInt16LE(0, 10); // mod time
    lh.writeUInt16LE(0, 12); // mod date
    lh.writeUInt32LE(0, 14); // crc32 — unchecked
    lh.writeUInt32LE(compressedSizeField, 18);
    lh.writeUInt32LE(data.length, 22);
    lh.writeUInt16LE(nameBuf.length, 26);
    lh.writeUInt16LE(0, 28); // extra length
    const localRecord = Buffer.concat([lh, nameBuf, compressed]);
    localParts.push(localRecord);

    const ch = Buffer.alloc(46);
    ch.writeUInt32LE(0x02014b50, 0);
    ch.writeUInt16LE(20, 4); // version made by
    ch.writeUInt16LE(20, 6); // version needed
    ch.writeUInt16LE(generalFlag, 8);
    ch.writeUInt16LE(method, 10);
    ch.writeUInt16LE(0, 12);
    ch.writeUInt16LE(0, 14);
    ch.writeUInt32LE(0, 16); // crc32
    ch.writeUInt32LE(compressedSizeField, 20);
    ch.writeUInt32LE(data.length, 24);
    ch.writeUInt16LE(nameBuf.length, 28);
    ch.writeUInt16LE(0, 30); // extra length
    ch.writeUInt16LE(0, 32); // comment length
    ch.writeUInt16LE(0, 34); // disk number
    ch.writeUInt16LE(0, 36); // internal attr
    ch.writeUInt32LE(externalAttr, 38);
    ch.writeUInt32LE(localOffset, 42);
    centralParts.push(Buffer.concat([ch, nameBuf]));

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

// --- 1. absolute paths --------------------------------------------------------

assertRejected("a posix-absolute path", [{ name: "/etc/passwd", data: Buffer.from("x") }], /unsafe zip entry path/);
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

// --- 3. non-regular file entries ----------------------------------------------

const S_IFLNK = 0xa000;
const S_IFCHR = 0x2000;
const S_IFIFO = 0x1000;

assertRejected(
  "a symlink entry",
  [{ name: "link", data: Buffer.from("/etc/passwd"), externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0 }],
  /non-regular zip entry/,
);
assertRejected(
  "a device entry",
  [{ name: "dev", data: Buffer.alloc(0), externalAttr: ((S_IFCHR | 0o666) << 16) >>> 0 }],
  /non-regular zip entry/,
);
assertRejected(
  "a fifo entry",
  [{ name: "fifo", data: Buffer.alloc(0), externalAttr: ((S_IFIFO | 0o644) << 16) >>> 0 }],
  /non-regular zip entry/,
);

// --- 4. unsupported zip features fail loudly, never silently -----------------

assertRejected(
  "a zip64 entry",
  [{ name: "big.bin", data: Buffer.from("x"), compressedSizeLie: 0xffffffff }],
  /unsupported zip feature: zip64/,
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

// --- positive case: tree preserved, content byte-exact, both codecs used -----

test("preserves the directory tree and extracts content byte-for-byte", () => {
  const binaryContent = Buffer.from(Array.from({ length: 500 }, (_, i) => i % 256));
  const skillMd = "---\nname: demo\n---\nBody.\n";
  const entries = [
    { name: "pkg/", data: Buffer.alloc(0) },
    { name: "pkg/SKILL.md", data: Buffer.from(skillMd, "utf8"), method: 0 }, // stored
    { name: "pkg/scripts/", data: Buffer.alloc(0) },
    { name: "pkg/scripts/run.sh", data: Buffer.from("#!/bin/sh\necho hi\n"), method: 8 }, // deflated
    { name: "pkg/assets/data.bin", data: binaryContent, method: 8 }, // deflated binary
  ];
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    extractPackage(archivePath, destDir);

    assert.deepEqual(readdirSync(join(destDir, "pkg")).sort(), ["SKILL.md", "assets", "scripts"]);
    assert.equal(readFileSync(join(destDir, "pkg", "SKILL.md"), "utf8"), skillMd);
    assert.equal(readFileSync(join(destDir, "pkg", "scripts", "run.sh"), "utf8"), "#!/bin/sh\necho hi\n");
    assert.deepEqual(readFileSync(join(destDir, "pkg", "assets", "data.bin")), binaryContent);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
