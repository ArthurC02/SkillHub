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
import { agentOptions, extractPackage, outputContract } from "./run.mjs";

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
    // `uncompressedSizeLie` is how the decompression-bomb fixture is built: the
    // central directory claims a small size while the deflate stream expands to
    // something far larger, which is the whole shape of the attack.
    const uncompressedSizeField = e.uncompressedSizeLie ?? data.length;

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
    lh.writeUInt32LE(uncompressedSizeField, 22);
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
    ch.writeUInt32LE(uncompressedSizeField, 24);
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
  const { archivePath, destDir, root } = stageZip([{ name: "big.bin", data: honest, method: 8 }]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readFileSync(join(destDir, "big.bin")).length, honest.length);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("refuses a package with more entries than the limit", () => {
  // 1001 empty files: no bytes at all, and still 1001 filesystem entries.
  // artifactMaxEntries on the collection side carries the same number for the
  // same reason (artifacts.go: "an empty file costs no bytes and still costs a
  // manifest entry").
  const entries = Array.from({ length: 1001 }, (_, i) => ({ name: `f${i}`, data: Buffer.alloc(0) }));
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    assert.throws(() => extractPackage(archivePath, destDir), /1001 entries exceeds the 1000 entry limit/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("accepts a package at exactly the entry limit", () => {
  const entries = Array.from({ length: 1000 }, (_, i) => ({ name: `f${i}`, data: Buffer.alloc(0) }));
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readdirSync(destDir).length, 1000);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("refuses a package whose declared sizes add up past the total limit", () => {
  // Three entries, each declaring 100 MiB and carrying nothing: 300 MiB
  // declared, past the 256 MiB bound. Nothing is inflated and nothing is
  // written — the refusal comes off the central directory alone, which is the
  // point of bounding the declaration rather than the running output.
  const hundredMiB = 100 * 1024 * 1024;
  const entries = Array.from({ length: 3 }, (_, i) => ({
    name: `part${i}.bin`,
    data: Buffer.alloc(0),
    method: 8,
    uncompressedSizeLie: hundredMiB,
  }));
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    assert.throws(() => extractPackage(archivePath, destDir), /declared \d+ bytes exceeds the 268435456 byte limit/);
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
    assert.throws(() => extractPackage(archivePath, destDir), /stored entry declares 1 bytes but carries 1048576/);
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
  assert.ok(text.includes(dir), "the absolute path the platform collects has to appear verbatim");
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
  assert.ok(!("settingSources" in options), "settingSources must stay omitted; passing it loads no skills");
  assert.deepEqual(options.allowedTools, ["Skill", "Read", "Write", "Edit", "Glob", "Grep", "Bash"]);
});
