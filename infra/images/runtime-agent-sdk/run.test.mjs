import assert from "node:assert/strict";
import {
  existsSync,
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
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";
import {
  agentOptions,
  declaredSkillRoot,
  extractPackage,
  installSkillFromArchive,
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

async function withJsonServer(status, body, handler) {
  const server = createServer((_req, res) => {
    res.writeHead(status, { "content-type": "application/json" });
    res.end(JSON.stringify(body));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const { port } = server.address();
    return await handler(`http://127.0.0.1:${port}`);
  } finally {
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
}

test("gateway spend lookup returns null immediately when base or key is missing, without waiting out the retry budget", async () => {
  const started = Date.now();
  const spend = await gatewaySpend({
    base: "",
    key: "",
    initialDelayMs: 5000,
    retryDelayMs: 5000,
    requestTimeoutMs: 2000,
    attempts: 2,
  });
  assert.equal(spend, null);
  assert.ok(Date.now() - started < 500, "missing config should skip the retry wait entirely");
});

test("gateway spend lookup returns the reading once two consecutive polls agree, not the first one", async () => {
  let calls = 0;
  const server = createServer((_req, res) => {
    calls += 1;
    const spend = calls === 1 ? 10 : 42;
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({ info: { spend } }));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const { port } = server.address();
    const spend = await gatewaySpend({
      base: `http://127.0.0.1:${port}`,
      key: "test",
      initialDelayMs: 0,
      retryDelayMs: 5,
      requestTimeoutMs: 200,
      attempts: 3,
    });
    assert.equal(spend, 42);
  } finally {
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
});

test("gateway spend lookup ignores a non-ok response body even when it carries a spend field", async () => {
  await withJsonServer(500, { info: { spend: 42 } }, async (base) => {
    const spend = await gatewaySpend({
      base,
      key: "test",
      initialDelayMs: 0,
      retryDelayMs: 5,
      requestTimeoutMs: 200,
      attempts: 2,
    });
    assert.equal(spend, null);
  });
});

test("gateway spend lookup ignores a spend field that is not a number", async () => {
  await withJsonServer(200, { info: { spend: "42" } }, async (base) => {
    const spend = await gatewaySpend({
      base,
      key: "test",
      initialDelayMs: 0,
      retryDelayMs: 5,
      requestTimeoutMs: 200,
      attempts: 2,
    });
    assert.equal(spend, null);
  });
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

// Mirrors run.mjs's own field layout byte-for-byte (30-byte local header,
// 46-byte central directory header, 22-byte EOCD, little-endian), so fixtures
// exercise the exact parser rather than an approximation of it.
function buildZip(entries) {
  const localParts = [];
  const centralParts = [];
  let offset = 0;

  for (const e of entries) {
    const nameBuf = Buffer.isBuffer(e.name) ? e.name : Buffer.from(e.name, "utf8");
    const extra = e.extra ?? Buffer.alloc(0);
    const data = e.data ?? Buffer.alloc(0);
    const method = e.method ?? 0;
    const compressed = method === 8 ? deflateRawSync(data) : data;
    const generalFlag = e.generalFlag ?? 0;
    const externalAttr = e.externalAttr ?? 0;
    const compressedSizeField = e.compressedSizeLie ?? compressed.length;
    const uncompressedSizeField = e.uncompressedSizeLie ?? data.length;
    const checksum = e.crcLie ?? crc32(data) >>> 0;

    const localOffset = offset;
    const lh = Buffer.alloc(30);
    lh.writeUInt32LE(0x04034b50, 0);
    lh.writeUInt16LE(20, 4);
    lh.writeUInt16LE(generalFlag, 6);
    lh.writeUInt16LE(method, 8);
    lh.writeUInt16LE(0, 10);
    lh.writeUInt16LE(0, 12);
    lh.writeUInt32LE(checksum, 14);
    lh.writeUInt32LE(compressedSizeField, 18);
    lh.writeUInt32LE(uncompressedSizeField, 22);
    lh.writeUInt16LE(nameBuf.length, 26);
    lh.writeUInt16LE(extra.length, 28);
    const localRecord = Buffer.concat([lh, nameBuf, extra, compressed]);
    localParts.push(localRecord);

    const ch = Buffer.alloc(46);
    ch.writeUInt32LE(0x02014b50, 0);
    ch.writeUInt16LE(((e.creatorSystem ?? 0) << 8) | 20, 4);
    ch.writeUInt16LE(20, 6);
    ch.writeUInt16LE(generalFlag, 8);
    ch.writeUInt16LE(method, 10);
    ch.writeUInt16LE(0, 12);
    ch.writeUInt16LE(0, 14);
    ch.writeUInt32LE(checksum, 16);
    ch.writeUInt32LE(compressedSizeField, 20);
    ch.writeUInt32LE(uncompressedSizeField, 24);
    ch.writeUInt16LE(nameBuf.length, 28);
    ch.writeUInt16LE(extra.length, 30);
    ch.writeUInt16LE(0, 32);
    ch.writeUInt16LE(0, 34);
    ch.writeUInt16LE(0, 36);
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

{
  const zip = buildZip([{ name: "SKILL.md", data: Buffer.from("x") }]);
  const eocdOffset = zip.length - 22;
  const cdOffset = zip.readUInt32LE(eocdOffset + 16);
  zip.writeUInt32LE(0xdeadbeef, cdOffset);
  assertRejectedZip(
    "a central directory entry with a corrupted signature",
    zip,
    /central directory entry signature mismatch/,
  );
}

{
  const zip = buildZip([
    { name: "a", data: Buffer.from("x") },
    { name: "b", data: Buffer.from("y") },
  ]);
  const secondLocalHeaderOffset = zip.indexOf(Buffer.from([0x50, 0x4b, 0x03, 0x04]), 4);
  zip.writeUInt32LE(0xdeadbeef, secondLocalHeaderOffset);
  assertRejectedZip(
    "a local file header with a corrupted signature",
    zip,
    /local file header signature mismatch/,
  );
}

{
  const zip = buildZip([{ name: "SKILL.md", data: Buffer.from("x") }]);
  const eocdOffset = zip.length - 22;
  const cdOffset = zip.readUInt32LE(eocdOffset + 16);
  zip.writeUInt32LE(zip.length + 100, cdOffset + 42);
  assertRejectedZip(
    "a central directory entry whose local header offset points past the end of the file",
    zip,
    /out of range/i,
  );
}

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
  [{ name: "file.bin", data: Buffer.from("x"), method: 12 }],
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
test("accepts a path exactly at admission's ten-segment depth ceiling", () => {
  const segments = Array.from({ length: 10 }, (_, i) => `d${i}`);
  const name = [...segments, "file.txt"].join("/");
  const { archivePath, destDir, root } = stageZip([{ name, data: Buffer.from("x") }]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readFileSync(join(destDir, ...segments, "file.txt"), "utf8"), "x");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
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

test("preserves the directory tree and extracts content byte-for-byte", () => {
  const binaryContent = Buffer.from(
    Array.from({ length: 500 }, (_, i) => i % 256),
  );
  const skillMd = "---\nname: demo\n---\nBody.\n";
  const entries = [
    { name: "pkg/", data: Buffer.alloc(0) },
    { name: "pkg/SKILL.md", data: Buffer.from(skillMd, "utf8"), method: 0 },
    { name: "pkg/scripts/", data: Buffer.alloc(0) },
    {
      name: "pkg/scripts/run.sh",
      data: Buffer.from("#!/bin/sh\necho hi\n"),
      method: 8,
    },
    { name: "pkg/assets/data.bin", data: binaryContent, method: 8 },
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

const packageRootCases = JSON.parse(
  readFileSync(
    fileURLToPath(new URL("../../../contracts/packaging/package-root-cases.json", import.meta.url)),
    "utf8",
  ),
).cases;

test("the package-root cases are there to read", () => {
  assert.ok(packageRootCases.length > 0);
});

for (const { name, entries, root } of packageRootCases) {
  test(`selects the package root admission records Skill paths under: ${name}`, () => {
    assert.equal(packageRoot(entries.map((entry) => ({ name: entry }))), root);
  });
}

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

test("refuses an entry that inflates far past its declared size", () => {
  const bomb = Buffer.alloc(10 * 1024 * 1024, 0);
  const { archivePath, destDir, root } = stageZip([
    { name: "bomb.bin", data: bomb, method: 8, uncompressedSizeLie: 100 },
  ]);
  try {
    assert.throws(
      () => extractPackage(archivePath, destDir),
      /inflates past its declared size of 100 bytes/,
    );
    assert.deepEqual(readdirSync(destDir), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("accepts an entry that deflates honestly, so the bound is not just a wall", () => {
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

test("accepts a package whose declared bytes total exactly the 100 MiB limit", () => {
  const tenMiB = 10 * 1024 * 1024;
  const entries = Array.from({ length: 10 }, (_, i) => ({
    name: `part${i}.bin`,
    data: Buffer.alloc(tenMiB, 7),
    method: 8,
  }));
  const { archivePath, destDir, root } = stageZip(entries);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readdirSync(destDir).length, 10);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("refuses a stored entry whose two sizes disagree", () => {
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

test("the agent is told the directory that is actually collected", () => {
  const dir = join("C:", "runs", "abc", "out", "artifacts");
  const text = outputContract(dir);
  assert.ok(
    text.includes(dir),
    "the absolute path the platform collects has to appear verbatim",
  );
  assert.match(text, /discarded/);
});

test("the output contract does not steer anything except where files go", () => {
  const text = outputContract("/out/artifacts");
  assert.match(text, /does not change how you answer/);
});

test("the agent turn still carries every option that fails silently", () => {
  const options = agentOptions("/out/artifacts");

  assert.equal(typeof options.systemPrompt, "string");
  assert.ok(options.systemPrompt.includes("/out/artifacts"));

  assert.equal(options.cwd, process.env.SKILLHUB_WORKDIR ?? "/work");

  const previousModel = process.env.SKILLHUB_MODEL;
  process.env.SKILLHUB_MODEL = "claude-test-model";
  try {
    assert.equal(agentOptions("/out/artifacts").model, "claude-test-model");
  } finally {
    if (previousModel === undefined) delete process.env.SKILLHUB_MODEL;
    else process.env.SKILLHUB_MODEL = previousModel;
  }

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

const NL = String.fromCharCode(10);

const PLUGIN_SKILL_MD =
  "---" + NL + "name: tidy-notes" + NL + "description: Tidy notes." + NL + "---" + NL + NL + "Body." + NL;
const SIBLING_SKILL_MD =
  "---" + NL + "name: split-csv" + NL + "description: Split a csv." + NL + "---" + NL + NL + "Body." + NL;

function pluginEntries() {
  return [
    { name: "plugin.json", data: Buffer.from('{"name":"desk-tools"}'), method: 0 },
    { name: "mcp.json", data: Buffer.from('{"mcpServers":{}}'), method: 0 },
    { name: "skills/tidy-notes/SKILL.md", data: Buffer.from(PLUGIN_SKILL_MD, "utf8"), method: 0 },
    { name: "skills/tidy-notes/notes.md", data: Buffer.from("reference", "utf8"), method: 0 },
    { name: "skills/split-csv/SKILL.md", data: Buffer.from(SIBLING_SKILL_MD, "utf8"), method: 0 },
  ];
}

function stageInstall(entries) {
  const { archivePath, root } = stageZip(entries);
  const inputDir = join(root, "input");
  const skillDir = join(root, "skills");
  mkdirSync(inputDir, { recursive: true });
  const failures = [];
  const install = (declaredPath) =>
    installSkillFromArchive(
      { archivePath, staging: join(inputDir, "package"), skillDir, declaredPath },
      (phase, code, message) => failures.push({ phase, code, message }),
    );
  return { install, inputDir, skillDir, failures, root };
}

test("a declared directory installs that skill of a plugin and nothing else", () => {
  const { install, inputDir, skillDir, failures, root } = stageInstall(pluginEntries());
  try {
    assert.equal(install("skills/tidy-notes"), "tidy-notes");
    assert.deepEqual(failures, []);
    assert.deepEqual(readdirSync(skillDir), ["tidy-notes"]);
    assert.deepEqual(readdirSync(join(skillDir, "tidy-notes")).sort(), [
      "SKILL.md",
      "notes.md",
    ]);
    assert.equal(
      readFileSync(join(skillDir, "tidy-notes", "SKILL.md"), "utf8"),
      PLUGIN_SKILL_MD,
    );
    assert.deepEqual(
      readdirSync(inputDir),
      [],
      "the rest of the plugin stayed behind in the input directory, so the sandbox holds bytes the run never asked for",
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a declared directory of a plugin inside a repository directory installs that skill and nothing else", () => {
  const wrapped = pluginEntries().map((entry) => ({ ...entry, name: `desk-tools-main/${entry.name}` }));
  const { install, inputDir, skillDir, failures, root } = stageInstall(wrapped);
  try {
    assert.equal(install("skills/tidy-notes"), "tidy-notes");
    assert.deepEqual(failures, []);
    assert.deepEqual(readdirSync(skillDir), ["tidy-notes"]);
    assert.equal(readFileSync(join(skillDir, "tidy-notes", "SKILL.md"), "utf8"), PLUGIN_SKILL_MD);
    assert.deepEqual(readdirSync(inputDir), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a declared directory that holds no SKILL.md fails the run instead of installing nothing", () => {
  const { install, skillDir, failures, root } = stageInstall(pluginEntries());
  try {
    assert.equal(install("skills/nope"), undefined);
    assert.equal(failures.length, 1, JSON.stringify(failures));
    assert.equal(failures[0].code, "invalid_package");
    assert.match(failures[0].message, /holds no SKILL.md/);
    assert.equal(existsSync(skillDir), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a plugin with no declared directory fails rather than installing the whole plugin as one skill", () => {
  const { install, skillDir, failures, root } = stageInstall(pluginEntries());
  try {
    assert.equal(install(""), undefined);
    assert.equal(failures.length, 1, JSON.stringify(failures));
    assert.equal(failures[0].code, "invalid_package");
    assert.equal(existsSync(skillDir), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a single-skill package still installs from the root the runtime resolves itself", () => {
  const { install, skillDir, failures, root } = stageInstall([
    { name: "pkg/SKILL.md", data: Buffer.from(PLUGIN_SKILL_MD, "utf8"), method: 0 },
  ]);
  try {
    assert.equal(install(""), "tidy-notes");
    assert.deepEqual(failures, []);
    assert.deepEqual(readdirSync(join(skillDir, "tidy-notes")), ["SKILL.md"]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a declared directory that escapes the package is refused before anything is extracted", () => {
  for (const escape of ["../../etc", "/etc", "skills/../../etc", ".."]) {
    const { install, inputDir, skillDir, failures, root } = stageInstall(pluginEntries());
    try {
      assert.equal(install(escape), undefined, escape);
      assert.equal(failures.length, 1, `${escape}: ${JSON.stringify(failures)}`);
      assert.equal(failures[0].code, "invalid_package");
      assert.equal(existsSync(skillDir), false, escape);
      assert.deepEqual(
        readdirSync(join(inputDir, "package")),
        [],
        `${escape}: the archive was unpacked before the path was judged`,
      );
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  }
});

test("the declared directory is read as a relative path inside the package and nothing else", () => {
  assert.equal(declaredSkillRoot(""), "");
  assert.equal(declaredSkillRoot(undefined), "");
  assert.equal(declaredSkillRoot("skills/a"), "skills/a/");
  assert.equal(declaredSkillRoot("skills/a/"), "skills/a/");
  assert.equal(declaredSkillRoot("./skills/a"), "skills/a/");
  assert.equal(declaredSkillRoot("a/b/c"), "a/b/c/");
  for (const rejected of ["/etc", "..", "../x", "a/../../b", ".", "./", "a" + String.fromCharCode(92) + "b"]) {
    assert.equal(declaredSkillRoot(rejected), null, rejected);
  }
});

function withEocdField(zip, offset, write) {
  const copy = Buffer.from(zip);
  write(copy, copy.length - 22 + offset);
  return copy;
}

function centralDirectoryOffset(zip) {
  return zip.readUInt32LE(zip.length - 22 + 16);
}

function spliceBeforeEocd(zip, bytes) {
  return Buffer.concat([zip.subarray(0, -22), bytes, zip.subarray(-22)]);
}

function extractBytes(bytes) {
  const root = mkdtempSync(join(tmpdir(), "run-test-"));
  const archivePath = join(root, "skill.zip");
  const destDir = join(root, "dest");
  writeFileSync(archivePath, bytes);
  mkdirSync(destDir);
  try {
    return { root: extractPackage(archivePath, destDir), files: readdirSync(destDir).sort() };
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function oneEntryZip() {
  return buildZip([{ name: "SKILL.md", data: Buffer.from("x") }]);
}

assertRejectedZip(
  "an EOCD whose entry count on this disk disagrees with its total",
  withEocdField(oneEntryZip(), 8, (b, at) => b.writeUInt16LE(0, at)),
  /unsupported zip feature: zip64 archive/,
);
assertRejectedZip(
  "an EOCD whose entry count is the zip64 sentinel",
  withEocdField(
    withEocdField(oneEntryZip(), 8, (b, at) => b.writeUInt16LE(0xffff, at)),
    10,
    (b, at) => b.writeUInt16LE(0xffff, at),
  ),
  /unsupported zip feature: zip64 archive/,
);
assertRejectedZip(
  "an EOCD whose central directory size is the zip64 sentinel",
  withEocdField(oneEntryZip(), 12, (b, at) => b.writeUInt32LE(0xffffffff, at)),
  /unsupported zip feature: zip64 archive/,
);
assertRejectedZip(
  "an EOCD whose central directory offset is the zip64 sentinel",
  withEocdField(oneEntryZip(), 16, (b, at) => b.writeUInt32LE(0xffffffff, at)),
  /unsupported zip feature: zip64 archive/,
);

test("accepts an empty archive that holds only its end record", () => {
  assert.deepEqual(extractBytes(buildZip([])), { root: "", files: [] });
});

{
  const zip = withEocdField(
    withEocdField(oneEntryZip(), 8, (b, at) => b.writeUInt16LE(2, at)),
    10,
    (b, at) => b.writeUInt16LE(2, at),
  );
  assertRejectedZip(
    "a declared entry count larger than the records present",
    zip,
    /malformed zip: truncated central directory entry$/,
  );
}

{
  let zip = spliceBeforeEocd(oneEntryZip(), Buffer.alloc(46));
  zip = withEocdField(zip, 8, (b, at) => b.writeUInt16LE(2, at));
  zip = withEocdField(zip, 10, (b, at) => b.writeUInt16LE(2, at));
  zip = withEocdField(zip, 12, (b, at) => b.writeUInt32LE(b.readUInt32LE(at) + 46, at));
  assertRejectedZip(
    "a second record of exactly 46 bytes as a bad signature rather than as truncated",
    zip,
    /central directory entry signature mismatch/,
  );
}

{
  const zip = oneEntryZip();
  const cd = centralDirectoryOffset(zip);
  zip.writeUInt16LE(zip.readUInt16LE(cd + 28) + 1, cd + 28);
  assertRejectedZip(
    "a central directory entry whose name runs one byte past the directory",
    zip,
    /malformed zip: central directory entry exceeds its bounds/,
  );
}

assertRejected(
  "an entry whose uncompressed size is the zip64 sentinel",
  [{ name: "big.bin", data: Buffer.from("x"), uncompressedSizeLie: 0xffffffff }],
  /unsupported zip feature: zip64 entry \(big\.bin\)/,
);

{
  const zip = oneEntryZip();
  zip.writeUInt32LE(0xffffffff, centralDirectoryOffset(zip) + 42);
  assertRejectedZip(
    "an entry whose local header offset is the zip64 sentinel",
    zip,
    /unsupported zip feature: zip64 entry \(SKILL\.md\)/,
  );
}

assertRejected(
  "an entry with a NUL in its name and a zip64 extra by reporting the name",
  [{ name: "a\0b", data: Buffer.from("x"), extra: zip64Extra }],
  /invalid name$/,
);
assertRejected(
  "an entry with a zip64 extra and a zip64 size by reporting the extra",
  [{ name: "z.bin", data: Buffer.from("x"), extra: Buffer.from([0x01, 0x00, 0x00, 0x00]), compressedSizeLie: 0xffffffff }],
  /zip64 entry \(z\.bin\)/,
);
assertRejected(
  "an entry that is both encrypted and of an unknown method by reporting the encryption",
  [{ name: "s.bin", data: Buffer.from("x"), generalFlag: 0x1, method: 12 }],
  /encrypted entry \(s\.bin\)/,
);
assertRejected(
  "an entry with an unknown method and an unsafe path by reporting the method",
  [{ name: "../m.bin", data: Buffer.from("x"), method: 12 }],
  /compression method 12 \(\.\.\/m\.bin\)/,
);
assertRejected(
  "an unsafe path that is also a symlink by reporting the path",
  [{ name: "../link", data: Buffer.alloc(0), externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0, creatorSystem: 3 }],
  /unsafe zip entry path: \.\.\/link/,
);
assertRejected(
  "a duplicate name that is also a symlink by reporting the duplicate",
  [
    { name: "a.txt", data: Buffer.from("x") },
    { name: "A.TXT", data: Buffer.alloc(0), externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0, creatorSystem: 3 },
  ],
  /duplicate portable zip entry name: A\.TXT/,
);
assertRejected(
  "an oversized symlink by reporting it as non-regular",
  [{ name: "big-link", data: Buffer.alloc(10 * 1024 * 1024 + 1), externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0, creatorSystem: 3 }],
  /non-regular zip entry: big-link/,
);
assertRejected(
  "an oversized directory-moded file by reporting the type disagreement",
  [{ name: "big", data: Buffer.alloc(10 * 1024 * 1024 + 1), externalAttr: ((0x4000 | 0o755) << 16) >>> 0, creatorSystem: 3 }],
  /type disagrees with its name: big/,
);
assertRejected(
  "an oversized entry nested too deep by reporting the size",
  [{ name: `${"d/".repeat(11)}big.bin`, data: Buffer.alloc(10 * 1024 * 1024 + 1) }],
  /oversized zip entry/,
);
assertRejected(
  "a file that an earlier file makes an ancestor of it",
  [
    { name: "a", data: Buffer.from("file") },
    { name: "a/b", data: Buffer.from("child") },
  ],
  /refusing zip file ancestor conflict: a\/b$/,
);
assertRejected(
  "a file added after an entry that needs it as a directory",
  [
    { name: "a/b", data: Buffer.from("child") },
    { name: "a", data: Buffer.from("file") },
  ],
  /refusing zip file that conflicts with a descendant: a$/,
);
assertRejected(
  "a symlink from a macOS-created archive",
  [{ name: "link", data: Buffer.from("/etc/passwd"), externalAttr: ((S_IFLNK | 0o777) << 16) >>> 0, creatorSystem: 19 }],
  /non-regular zip entry: link/,
);
assertRejected(
  "a regular-file mode on a directory-shaped name",
  [{ name: "dir/", externalAttr: ((0x8000 | 0o644) << 16) >>> 0, creatorSystem: 3 }],
  /type disagrees with its name: dir\//,
);

test("accepts a directory entry listed after a file inside it", () => {
  const { archivePath, destDir, root } = stageZip([
    { name: "a/b", data: Buffer.from("child") },
    { name: "a/", data: Buffer.alloc(0) },
  ]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readFileSync(join(destDir, "a", "b"), "utf8"), "child");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("accepts a directory entry whose mode says directory", () => {
  const { archivePath, destDir, root } = stageZip([
    { name: "d/", externalAttr: ((0x4000 | 0o755) << 16) >>> 0, creatorSystem: 3 },
  ]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(statSync(join(destDir, "d")).isDirectory(), true);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("accepts a file entry whose mode says regular file", () => {
  const { archivePath, destDir, root } = stageZip([
    { name: "f.txt", data: Buffer.from("ok"), externalAttr: ((0x8000 | 0o644) << 16) >>> 0, creatorSystem: 3 },
  ]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(readFileSync(join(destDir, "f.txt"), "utf8"), "ok");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("accepts a directory entry exactly at the depth ceiling despite its trailing slash", () => {
  const segments = Array.from({ length: 11 }, (_, i) => `d${i}`);
  const { archivePath, destDir, root } = stageZip([{ name: `${segments.join("/")}/` }]);
  try {
    extractPackage(archivePath, destDir);
    assert.equal(statSync(join(destDir, ...segments)).isDirectory(), true);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("skips a central directory entry comment to reach the next record", () => {
  const zip = buildZip([
    { name: "a.txt", data: Buffer.from("one") },
    { name: "b.txt", data: Buffer.from("two") },
  ]);
  const cd = centralDirectoryOffset(zip);
  const firstRecordEnd = cd + 46 + zip.readUInt16LE(cd + 28);
  const comment = Buffer.from("note");
  const bytes = Buffer.concat([zip.subarray(0, firstRecordEnd), comment, zip.subarray(firstRecordEnd)]);
  bytes.writeUInt16LE(comment.length, cd + 32);
  const eocd = bytes.length - 22;
  bytes.writeUInt32LE(bytes.readUInt32LE(eocd + 12) + comment.length, eocd + 12);
  assert.deepEqual(extractBytes(bytes).files, ["a.txt", "b.txt"]);
});

assertRejectedZip(
  "bytes between the central directory and its end record",
  spliceBeforeEocd(oneEntryZip(), Buffer.from("junk!")),
  /unsupported prefixed or malformed zip archive/,
);

async function withSpendServer(replies, handler) {
  const seen = [];
  const server = createServer((req, res) => {
    seen.push({ url: req.url, authorization: req.headers.authorization });
    const reply = replies[Math.min(seen.length, replies.length) - 1];
    res.writeHead(reply.status ?? 200, { "content-type": "application/json" });
    res.end(reply.raw ?? JSON.stringify(reply.body));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const { port } = server.address();
    return await handler(`http://127.0.0.1:${port}`, seen);
  } finally {
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
}

const spend = (value) => ({ body: { info: { spend: value } } });
const quickPolls = { initialDelayMs: 0, retryDelayMs: 5, requestTimeoutMs: 500 };

for (const [label, config] of [
  ["the key", { base: "http://127.0.0.1:9", key: "" }],
  ["the base", { base: "", key: "test" }],
]) {
  test(`gateway spend lookup returns null immediately when only ${label} is missing`, async () => {
    const started = Date.now();
    const value = await gatewaySpend({ ...config, initialDelayMs: 5000, retryDelayMs: 5000, attempts: 2 });
    assert.equal(value, null);
    assert.ok(Date.now() - started < 500, "missing config should skip the wait");
  });
}

test("gateway spend lookup asks the key-info path once per attempt with the bearer key, dropping a trailing slash", async () => {
  await withSpendServer([spend(3), spend(4), spend(5)], async (base, seen) => {
    assert.equal(await gatewaySpend({ base: `${base}/`, key: "k1", ...quickPolls, attempts: 3 }), 5);
    assert.deepEqual(seen, Array(3).fill({ url: "/key/info", authorization: "Bearer k1" }));
  });
});

test("gateway spend lookup stops polling as soon as two readings agree", async () => {
  await withSpendServer([spend(7), spend(7), spend(9)], async (base, seen) => {
    assert.equal(await gatewaySpend({ base, key: "k", ...quickPolls, attempts: 6 }), 7);
    assert.equal(seen.length, 2);
  });
});

test("gateway spend lookup does not treat two zero readings as converged", async () => {
  await withSpendServer([spend(0)], async (base, seen) => {
    assert.equal(await gatewaySpend({ base, key: "k", ...quickPolls, attempts: 3 }), null);
    assert.equal(seen.length, 3);
  });
});

test("gateway spend lookup keeps the last positive reading when later polls fail", async () => {
  await withSpendServer([spend(42), { status: 500, body: {} }], async (base) => {
    assert.equal(await gatewaySpend({ base, key: "k", ...quickPolls, attempts: 3 }), 42);
  });
});

test("gateway spend lookup treats an unparseable body as no reading", async () => {
  await withSpendServer([{ raw: "not json" }], async (base) => {
    assert.equal(await gatewaySpend({ base, key: "k", ...quickPolls, attempts: 2 }), null);
  });
});

test("gateway spend lookup does not wait after its last attempt", async () => {
  await withSpendServer([spend(1)], async (base) => {
    const started = Date.now();
    assert.equal(await gatewaySpend({ base, key: "k", initialDelayMs: 0, retryDelayMs: 5000, requestTimeoutMs: 500, attempts: 1 }), 1);
    assert.ok(Date.now() - started < 2000, "waited a retry delay after the final attempt");
  });
});

const RUN_SCRIPT = fileURLToPath(new URL("./run.mjs", import.meta.url));

function runWorkload(script, extraEnv = {}) {
  const root = mkdtempSync(join(tmpdir(), "run-main-"));
  try {
    const sdk = join(root, "fake-sdk.mjs");
    writeFileSync(
      sdk,
      "export async function* query() {\n" +
        `  for (const step of ${JSON.stringify(script)}) {\n` +
        "    if (step.throw) throw new Error(step.throw);\n" +
        "    yield step;\n" +
        "  }\n" +
        "}\n",
    );
    const hooks = join(root, "hooks.mjs");
    writeFileSync(
      hooks,
      "export async function resolve(specifier, context, next) {\n" +
        `  if (specifier === "@anthropic-ai/claude-agent-sdk") return { url: ${JSON.stringify(pathToFileURL(sdk).href)}, shortCircuit: true };\n` +
        "  return next(specifier, context);\n" +
        "}\n",
    );
    const register = join(root, "register.mjs");
    writeFileSync(
      register,
      `import { register } from "node:module";\nregister(${JSON.stringify(pathToFileURL(hooks).href)});\n`,
    );
    const outDir = join(root, "out");
    const inputDir = join(root, "input");
    const skillDir = join(root, "skills");
    mkdirSync(outDir);
    mkdirSync(inputDir);
    writeFileSync(join(inputDir, "ready"), "");
    writeFileSync(join(outDir, ".collected"), "");
    const env = { ...process.env };
    delete env.ANTHROPIC_BASE_URL;
    delete env.ANTHROPIC_AUTH_TOKEN;
    delete env.SKILLHUB_MAX_INPUT_TOKENS;
    delete env.SKILLHUB_MAX_OUTPUT_TOKENS;
    Object.assign(env, {
      SKILLHUB_WORKDIR: join(root, "work"),
      SKILLHUB_OUTDIR: outDir,
      SKILLHUB_SKILL_DIR: skillDir,
      SKILLHUB_INPUT_DIR: inputDir,
      SKILLHUB_USER_PROMPT: "do the task",
      SKILLHUB_RUN_ID: "run-1",
      SKILLHUB_MODEL: "test-model",
      SKILLHUB_SKILL_VERSION_ID: "sv-1",
      ...extraEnv,
    });
    const child = spawnSync(process.execPath, ["--import", pathToFileURL(register).href, RUN_SCRIPT], {
      env,
      encoding: "utf8",
      timeout: 30_000,
    });
    const tracePath = join(outDir, "trace", "events.jsonl");
    const events = existsSync(tracePath)
      ? readFileSync(tracePath, "utf8").trim().split("\n").map((line) => JSON.parse(line))
      : [];
    return {
      status: child.status,
      stderr: child.stderr,
      events,
      result: JSON.parse(readFileSync(join(outDir, "result.json"), "utf8")),
    };
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

const startEvent = (usage) => ({ type: "stream_event", event: { type: "message_start", message: { usage } } });
const deltaEvent = (usage) => ({ type: "stream_event", event: { type: "message_delta", usage } });

test("a completed agent turn emits each block's event in order and reports usage from the result", () => {
  const skillFile = "/skills-root-placeholder/tidy/SKILL.md";
  const script = [
    startEvent({ input_tokens: 100, output_tokens: 1, cache_read_input_tokens: 7 }),
    deltaEvent({ output_tokens: 20 }),
    {
      type: "assistant",
      message: {
        content: [
          { type: "text", text: "thinking out loud" },
          { type: "text", text: "   " },
          { type: "tool_use", id: "t1", name: "Skill", input: { command: "tidy-notes", description: "why" } },
          { type: "tool_use", id: "t2", name: "Bash", input: { command: "echo hi" } },
          { type: "tool_use", id: "t3", name: "Read", input: { file_path: skillFile } },
        ],
      },
    },
    {
      type: "user",
      message: {
        content: [
          { type: "text", text: "a user's text is not agent output" },
          { type: "tool_result", tool_use_id: "t2", content: "hi", is_error: false },
          { type: "tool_result", tool_use_id: "t3", content: "gone", is_error: true },
          { type: "tool_result", tool_use_id: "never-opened", content: "x" },
        ],
      },
    },
    startEvent({ input_tokens: 50 }),
    { type: "result", result: "final answer", is_error: false, usage: { input_tokens: 150, output_tokens: 20, cache_read_input_tokens: 7 } },
  ];
  const run = runWorkload(script, { SKILLHUB_SKILL_DIR: "/skills-root-placeholder" });
  assert.equal(run.status, 0, run.stderr);
  assert.deepEqual(
    run.events.map((e) => [e.seq, e.type, e.status]),
    [
      [1, "agent_output", "ok"],
      [2, "skill_activation", "ok"],
      [3, "script_log", "ok"],
      [4, "tool_call", "ok"],
      [5, "resource_read", "error"],
      [6, "tool_call", "error"],
      [7, "agent_output", "ok"],
      [8, "usage", "ok"],
    ],
  );
  const [intermediate, activation, log, bash, resource, read, final, usage] = run.events.map((e) => e.payload);
  assert.deepEqual(intermediate, { kind: "intermediate", text: "thinking out loud", truncated: false });
  assert.deepEqual(activation, { skill_name: "tidy-notes", skill_version_id: "sv-1", decision: "activated", reason: "why" });
  assert.deepEqual(log, { script_path: null, stream: "stdout", message: "hi", truncated: false, dropped_bytes: null });
  assert.deepEqual(Object.keys(bash), ["tool_name", "invocation_id", "arguments", "result_summary", "outcome", "duration_ms", "truncated"]);
  assert.equal(bash.outcome, "succeeded");
  assert.deepEqual(resource, { resource_path: "tidy/SKILL.md", outcome: "not_found", bytes_read: null, truncated: false });
  assert.equal(read.outcome, "failed");
  assert.deepEqual(final, { kind: "final", text: "final answer", truncated: false });
  assert.deepEqual(Object.keys(usage), [
    "scope", "model", "input_tokens", "output_tokens", "cache_read_input_tokens", "cache_write_input_tokens",
    "cost_usd", "cost_source", "token_source", "duration_ms",
  ]);
  assert.deepEqual(
    { ...usage, duration_ms: 0 },
    {
      scope: "run_total", model: "test-model", input_tokens: 150, output_tokens: 20,
      cache_read_input_tokens: 7, cache_write_input_tokens: null, cost_usd: null, cost_source: null,
      token_source: "result", duration_ms: 0,
    },
  );
  assert.deepEqual(run.result, { status: "succeeded", agent_output: "final answer", message_types: ["assistant", "user", "result"] });
});

test("a turn that crosses its output ceiling across two responses stops there and exits with the budget code", () => {
  const script = [
    startEvent({ input_tokens: 5, output_tokens: 1 }),
    deltaEvent({ output_tokens: 6 }),
    startEvent({ output_tokens: 5 }),
    { type: "assistant", message: { content: [{ type: "text", text: "never seen" }] } },
    { type: "result", result: "never seen", usage: {} },
  ];
  const run = runWorkload(script, { SKILLHUB_MAX_OUTPUT_TOKENS: "10" });
  assert.equal(run.status, 9, run.stderr);
  const message =
    "run stopped at its output token ceiling: 11 of 10 tokens (PDM-005 5.2a; the limit shown in the pre-run permission summary)";
  assert.deepEqual(run.events.map((e) => e.type), ["error", "usage"]);
  assert.deepEqual(run.events[0].payload, { category: "execution", code: "token_budget_exceeded", message, retryable: false });
  assert.equal(run.events[1].payload.token_source, "accumulated");
  assert.equal(run.events[1].payload.input_tokens, 5);
  assert.equal(run.events[1].payload.output_tokens, 11);
  assert.deepEqual(run.result, { status: "failed", error: message, agent_output: "", message_types: [] });
});

test("a turn exactly at its output ceiling is not stopped", () => {
  const script = [
    startEvent({ output_tokens: 1 }),
    deltaEvent({ output_tokens: 10 }),
    { type: "result", result: "done", usage: { output_tokens: 10 } },
  ];
  const run = runWorkload(script, { SKILLHUB_MAX_OUTPUT_TOKENS: "10" });
  assert.equal(run.status, 0, run.stderr);
  assert.equal(run.result.status, "succeeded");
});

test("a result the agent marks as an error is reported with error status and the run still completes", () => {
  const run = runWorkload([{ type: "result", result: "gave up", is_error: true, usage: { input_tokens: 1, output_tokens: 2 } }]);
  assert.equal(run.status, 0, run.stderr);
  assert.deepEqual(
    run.events.map((e) => [e.type, e.status]),
    [["agent_output", "error"], ["usage", "ok"]],
  );
  assert.equal(run.result.status, "succeeded");
});

test("cached input counts toward the input ceiling", () => {
  const run = runWorkload(
    [startEvent({ input_tokens: 60, cache_read_input_tokens: 41 }), { type: "result", result: "never", usage: {} }],
    { SKILLHUB_MAX_INPUT_TOKENS: "100" },
  );
  assert.equal(run.status, 9, run.stderr);
  assert.match(run.result.error, /its input token ceiling: 101 of 100 tokens/);
});

test("a turn whose stream fails reports accumulated usage, then the failure", () => {
  const run = runWorkload([
    startEvent({ input_tokens: 3 }),
    deltaEvent({ output_tokens: 4 }),
    { throw: "stream broke" },
  ]);
  assert.equal(run.status, 1, run.stderr);
  assert.deepEqual(run.events.map((e) => e.type), ["usage", "error"]);
  assert.equal(run.events[0].payload.token_source, "accumulated");
  assert.equal(run.events[0].payload.input_tokens, 3);
  assert.equal(run.events[0].payload.output_tokens, 4);
  assert.deepEqual(run.events[1].payload, { category: "execution", code: "agent_turn_failed", message: "stream broke", retryable: false });
  assert.deepEqual(run.result, { status: "failed", error: "stream broke" });
});
