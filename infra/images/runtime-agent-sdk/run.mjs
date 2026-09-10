import {
  appendFileSync,
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { randomUUID } from "node:crypto";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";
import { crc32, inflateRawSync } from "node:zlib";

const workDir = process.env.SKILLHUB_WORKDIR ?? "/work";
const outDir = process.env.SKILLHUB_OUTDIR ?? "/out";
const skillDir =
  process.env.SKILLHUB_SKILL_DIR ?? join(workDir, ".claude", "skills");
const inputDir = process.env.SKILLHUB_INPUT_DIR ?? join(workDir, ".skillhub");
const artifactDir =
  process.env.SKILLHUB_ARTIFACT_DIR ?? join(outDir, "artifacts");
const prompt = process.env.SKILLHUB_USER_PROMPT;

const traceDir = join(outDir, "trace");
const tracePath = join(traceDir, "events.jsonl");
const runId = process.env.SKILLHUB_RUN_ID ?? "";
const attempt = Number(process.env.SKILLHUB_ATTEMPT ?? "1") || 1;

let seq = 0;
let traceReady = false;

const LIMITS = {
  message: 16000,
  text: 64000,
  result: 8000,
  reason: 2000,
  error: 4000,
};

function clip(value, limit) {
  const s = typeof value === "string" ? value : JSON.stringify(value ?? null);
  if (s.length <= limit) return { value: s, truncated: false };
  return { value: s.slice(0, limit), truncated: true };
}

function emit(type, payload, status = "ok") {
  if (!runId) return;
  if (!traceReady) {
    mkdirSync(traceDir, { recursive: true });
    traceReady = true;
  }
  seq += 1;
  const event = {
    schema_version: "1.1",
    event_id: randomUUID(),
    run_id: runId,
    attempt,
    seq,
    occurred_at: new Date().toISOString(),
    emitted_by: "sandbox",
    type,
    status,
    masked: false,
    masked_fields: [],
    payload,
  };
  try {
    appendFileSync(tracePath, JSON.stringify(event) + "\n");
  } catch {
  }
}

const EXIT_TOKEN_BUDGET = 9;

function finish(status, extra = {}, exitCode = status === "succeeded" ? 0 : 1) {
  writeFileSync(
    join(outDir, "result.json"),
    JSON.stringify({ status, ...extra }, null, 2),
  );
  waitToBeCollected();
  process.exit(exitCode);
}

// Blocks the current call stack synchronously via a throwaway buffer, with no
// process spawned and no event-loop turn — the busy-wait loops below need
// that.
function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

function waitToBeCollected() {
  const collected = join(outDir, ".collected");
  try {
    writeFileSync(join(outDir, ".workload-done"), "done\n");
  } catch {
    return;
  }
  const deadline = Date.now() + 60_000;
  while (!existsSync(collected) && Date.now() < deadline) {
    sleepSync(200);
  }
}

function fail(category, code, message) {
  const clipped = clip(message, LIMITS.error);
  emit(
    "error",
    { category, code, message: clipped.value, retryable: false },
    "error",
  );
  finish("failed", { error: clipped.value });
}

const MAX_PACKAGE_ENTRIES = 2000; // one-number: maxSkillPackageEntries
const MAX_PACKAGE_TOTAL_BYTES = 100 * 1024 * 1024;
const MAX_PACKAGE_ENTRY_BYTES = 10 * 1024 * 1024;
const MAX_PACKAGE_DEPTH = 10;

const EOCD_SIGNATURE = 0x06054b50;
const CENTRAL_DIR_SIGNATURE = 0x02014b50;
const LOCAL_FILE_SIGNATURE = 0x04034b50;
const ZIP64_EOCD_LOCATOR_SIGNATURE = 0x07064b50;

// The end-of-central-directory record has a variable-length comment (0-65535
// bytes), so its offset can't be read directly and must be found by scanning
// backward for the signature.
function findEndOfCentralDirectory(buf) {
  const minPos = Math.max(0, buf.length - 22 - 65535);
  for (let i = buf.length - 22; i >= minPos; i -= 1) {
    if (
      buf.readUInt32LE(i) === EOCD_SIGNATURE &&
      i + 22 + buf.readUInt16LE(i + 20) === buf.length
    )
      return i;
  }
  throw new Error("not a zip file: end of central directory record not found");
}

// Checks both separators: a name like "..\\x" looks harmless to POSIX (only
// "/" separates), but Node's path.join treats "\\" as a separator too, so the
// same entry becomes a real parent-directory escape once joined.
function isUnsafeEntryName(name) {
  if (name === "") return true;
  if (name.includes("\\")) return true;
  if (name.startsWith("/") || name.startsWith("\\")) return true;
  if (/^[a-zA-Z]:/.test(name)) return true;
  return name.split(/[/\\]+/).some((segment) => segment === "..");
}

function portableEntryKey(name) {
  const trimmed = name.endsWith("/") ? name.slice(0, -1) : name;
  const parts = trimmed.split("/");
  if (
    trimmed === "" ||
    parts.some(
      (part) =>
        part === "" ||
        part === "." ||
        Buffer.byteLength(part, "utf8") > 255 ||
        /[<>:"|?*\x00-\x1f]/u.test(part) ||
        /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/iu.test(part) ||
        part.replace(/[ .]+$/u, "") !== part,
    )
  ) {
    throw new Error(`refusing non-canonical zip entry name: ${name}`);
  }
  return parts.map((part) => part.toLowerCase()).join("/");
}

export async function gatewaySpend({
  base = process.env.ANTHROPIC_BASE_URL,
  key = process.env.ANTHROPIC_AUTH_TOKEN,
  initialDelayMs = 2500,
  retryDelayMs = 1500,
  requestTimeoutMs = 2000,
  attempts = 6,
} = {}) {
  if (!base || !key) return null;
  // The gateway flushes spend asynchronously, so this polls until the reading
  // stops moving rather than trusting the first (likely undercounted) value.
  let last = null;
  await new Promise((resolve) => setTimeout(resolve, initialDelayMs));
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    let spend = null;
    try {
      const res = await fetch(`${base.replace(/\/$/, "")}/key/info`, {
        headers: { Authorization: `Bearer ${key}` },
        signal: AbortSignal.timeout(requestTimeoutMs),
      });
      if (res.ok) {
        const body = await res.json();
        if (typeof body?.info?.spend === "number") spend = body.info.spend;
      }
    } catch {
    }
    if (spend !== null && spend > 0 && spend === last) return spend;
    if (spend !== null) last = spend;
    if (attempt + 1 < attempts) {
      await new Promise((resolve) => setTimeout(resolve, retryDelayMs));
    }
  }
  return last !== null && last > 0 ? last : null;
}

function hasZip64Extra(extra) {
  let pos = 0;
  while (pos < extra.length) {
    if (pos + 4 > extra.length) {
      throw new Error("malformed zip: truncated extra field");
    }
    const id = extra.readUInt16LE(pos);
    const size = extra.readUInt16LE(pos + 2);
    if (pos + 4 + size > extra.length) {
      throw new Error("malformed zip: truncated extra field");
    }
    if (id === 0x0001) return true;
    pos += 4 + size;
  }
  return false;
}

const S_IFMT = 0xf000;
const S_IFREG = 0x8000;
const S_IFDIR = 0x4000;

// Unix file-type bits live in the top 16 bits of external_attr; anything that
// is not a plain file or directory (symlink, device, fifo, socket) is refused.
function isUnsafeFileType(unixMode) {
  const fileType = unixMode & S_IFMT;
  return fileType !== 0 && fileType !== S_IFREG && fileType !== S_IFDIR;
}

function readCentralDirectory(buf) {
  const eocdOffset = findEndOfCentralDirectory(buf);
  const diskNumber = buf.readUInt16LE(eocdOffset + 4);
  const centralDisk = buf.readUInt16LE(eocdOffset + 6);
  const diskEntries = buf.readUInt16LE(eocdOffset + 8);
  const totalEntries = buf.readUInt16LE(eocdOffset + 10);
  const cdSize = buf.readUInt32LE(eocdOffset + 12);
  const cdOffset = buf.readUInt32LE(eocdOffset + 16);

  if (
    diskNumber !== 0 ||
    centralDisk !== 0 ||
    diskEntries !== totalEntries ||
    totalEntries === 0xffff ||
    cdSize === 0xffffffff ||
    cdOffset === 0xffffffff
  ) {
    throw new Error("unsupported zip feature: zip64 archive");
  }
  if (totalEntries > MAX_PACKAGE_ENTRIES) {
    throw new Error(
      `refusing oversized skill package: ${totalEntries} entries exceeds the ${MAX_PACKAGE_ENTRIES} entry limit`,
    );
  }
  if (totalEntries > 0 && buf.readUInt32LE(0) !== LOCAL_FILE_SIGNATURE) {
    throw new Error("unsupported prefixed zip archive");
  }
  if (
    eocdOffset >= 20 &&
    buf.readUInt32LE(eocdOffset - 20) === ZIP64_EOCD_LOCATOR_SIGNATURE
  ) {
    throw new Error("unsupported zip feature: zip64 archive");
  }

  const entries = [];
  const seenNames = new Map();
  const requiredDirs = new Set();
  let declaredBytes = 0;
  let pos = cdOffset;
  for (let i = 0; i < totalEntries; i += 1) {
    if (pos + 46 > eocdOffset) {
      throw new Error("malformed zip: truncated central directory entry");
    }
    if (buf.readUInt32LE(pos) !== CENTRAL_DIR_SIGNATURE) {
      throw new Error(
        "malformed zip: central directory entry signature mismatch",
      );
    }
    const creatorSystem = buf.readUInt16LE(pos + 4) >>> 8;
    const generalFlag = buf.readUInt16LE(pos + 8);
    const method = buf.readUInt16LE(pos + 10);
    const crc = buf.readUInt32LE(pos + 16);
    const compressedSize = buf.readUInt32LE(pos + 20);
    const uncompressedSize = buf.readUInt32LE(pos + 24);
    const nameLen = buf.readUInt16LE(pos + 28);
    const extraLen = buf.readUInt16LE(pos + 30);
    const commentLen = buf.readUInt16LE(pos + 32);
    const externalAttr = buf.readUInt32LE(pos + 38);
    // Only a Unix or macOS creator system uses this word as a mode; other
    // tools put unrelated data there, which would otherwise look like a mode.
    const unixMode = creatorSystem === 3 || creatorSystem === 19
      ? externalAttr >>> 16
      : 0;
    const localHeaderOffset = buf.readUInt32LE(pos + 42);
    const nameBytes = buf.subarray(pos + 46, pos + 46 + nameLen);
    const extraStart = pos + 46 + nameLen;
    const extraEnd = extraStart + extraLen;
    if (extraEnd + commentLen > eocdOffset) {
      throw new Error("malformed zip: central directory entry exceeds its bounds");
    }
    const extra = buf.subarray(extraStart, extraEnd);
    let name;
    try {
      name = new TextDecoder("utf-8", { fatal: true }).decode(nameBytes);
    } catch {
      throw new Error("refusing zip entry with an invalid UTF-8 name");
    }
    if (name.length === 0 || name.includes("\0")) {
      throw new Error("refusing zip entry with an invalid name");
    }
    if (hasZip64Extra(extra)) {
      throw new Error(`unsupported zip feature: zip64 entry (${name})`);
    }

    if (
      compressedSize === 0xffffffff ||
      uncompressedSize === 0xffffffff ||
      localHeaderOffset === 0xffffffff
    ) {
      throw new Error(`unsupported zip feature: zip64 entry (${name})`);
    }
    // Bit 0 of the general-purpose flag marks encryption; this parser holds
    // no keys, so it must refuse ciphertext rather than read it as plaintext.
    if (generalFlag & 0x1) {
      throw new Error(`unsupported zip feature: encrypted entry (${name})`);
    }
    if (method !== 0 && method !== 8) {
      throw new Error(
        `unsupported zip feature: compression method ${method} (${name})`,
      );
    }
    if (isUnsafeEntryName(name)) {
      throw new Error(`refusing unsafe zip entry path: ${name}`);
    }
    const nameKey = portableEntryKey(name);
    if (seenNames.has(nameKey)) {
      throw new Error(`refusing duplicate portable zip entry name: ${name}`);
    }
    const nameIsDir = name.endsWith("/");
    const parts = nameKey.split("/");
    for (let part = 1; part < parts.length; part += 1) {
      const ancestor = parts.slice(0, part).join("/");
      if (seenNames.has(ancestor) && !seenNames.get(ancestor)) {
        throw new Error(`refusing zip file ancestor conflict: ${name}`);
      }
      requiredDirs.add(ancestor);
    }
    if (!nameIsDir && requiredDirs.has(nameKey)) {
      throw new Error(`refusing zip file that conflicts with a descendant: ${name}`);
    }
    seenNames.set(nameKey, nameIsDir);
    if (isUnsafeFileType(unixMode)) {
      throw new Error(`refusing non-regular zip entry: ${name}`);
    }
    const fileType = unixMode & S_IFMT;
    if ((fileType === S_IFDIR) !== nameIsDir && fileType !== 0) {
      throw new Error(
        `refusing zip entry whose type disagrees with its name: ${name}`,
      );
    }
    if (uncompressedSize > MAX_PACKAGE_ENTRY_BYTES) {
      throw new Error(
        `refusing oversized zip entry: ${name} declares ${uncompressedSize} bytes`,
      );
    }
    const depth = name.replace(/\/+$/, "").split("/").length - 1;
    if (depth > MAX_PACKAGE_DEPTH) {
      throw new Error(
        `refusing zip entry nested ${depth} directories deep: ${name}`,
      );
    }

    entries.push({
      name,
      method,
      crc,
      compressedSize,
      uncompressedSize,
      localHeaderOffset,
      mode: unixMode & 0o777,
    });
    declaredBytes += uncompressedSize;
    if (declaredBytes > MAX_PACKAGE_TOTAL_BYTES) {
      throw new Error(
        `refusing oversized skill package: declared ${declaredBytes} bytes exceeds the ${MAX_PACKAGE_TOTAL_BYTES} byte limit`,
      );
    }
    pos += 46 + nameLen + extraLen + commentLen;
  }
  if (cdOffset + cdSize !== eocdOffset) {
    throw new Error("unsupported prefixed or malformed zip archive");
  }
  if (pos !== cdOffset + cdSize) {
    throw new Error("malformed zip: central directory entry count mismatch");
  }
  return entries;
}

export function packageRoot(entries) {
  if (entries.some((entry) => entry.name === "SKILL.md")) return "";
  const roots = new Set(
    entries
      .filter((entry) => entry.name !== "")
      .map((entry) => entry.name.split("/")[0]),
  );
  if (roots.size !== 1) return "";
  const [root] = roots;
  return entries.some((entry) => entry.name === `${root}/SKILL.md`)
    ? `${root}/`
    : "";
}

function readEntryData(buf, entry) {
  const lh = entry.localHeaderOffset;
  if (buf.readUInt32LE(lh) !== LOCAL_FILE_SIGNATURE) {
    throw new Error(
      `malformed zip: local file header signature mismatch (${entry.name})`,
    );
  }
  const nameLen = buf.readUInt16LE(lh + 26);
  const extraLen = buf.readUInt16LE(lh + 28);
  const dataStart = lh + 30 + nameLen + extraLen;
  const compressed = buf.subarray(dataStart, dataStart + entry.compressedSize);
  if (compressed.length !== entry.compressedSize) {
    throw new Error(`malformed zip: truncated compressed data (${entry.name})`);
  }
  let data;
  if (entry.method !== 8) {
    if (entry.compressedSize !== entry.uncompressedSize) {
      throw new Error(
        `malformed zip: stored entry declares ${entry.uncompressedSize} bytes but carries ${entry.compressedSize} (${entry.name})`,
      );
    }
    data = Buffer.from(compressed);
  } else {
    try {
      data = inflateRawSync(compressed, {
        maxOutputLength: Math.max(1, entry.uncompressedSize),
      });
    } catch (err) {
      if (err?.code === "ERR_BUFFER_TOO_LARGE") {
        throw new Error(
          `refusing zip entry that inflates past its declared size of ${entry.uncompressedSize} bytes: ${entry.name}`,
        );
      }
      throw err;
    }
  }
  if (data.length !== entry.uncompressedSize) {
    throw new Error(
      `malformed zip: uncompressed size mismatch (${entry.name})`,
    );
  }
  if (crc32(data) >>> 0 !== entry.crc) {
    throw new Error(`malformed zip: CRC32 mismatch (${entry.name})`);
  }
  return data;
}

export function extractPackage(archivePath, destDir) {
  const buf = readFileSync(archivePath);
  const entries = readCentralDirectory(buf);
  for (const entry of entries) {
    const destPath = join(destDir, entry.name);
    if (entry.name.endsWith("/")) {
      mkdirSync(destPath, { recursive: true });
      continue;
    }
    mkdirSync(dirname(destPath), { recursive: true });
    writeFileSync(destPath, readEntryData(buf, entry));
    if (entry.mode !== 0) chmodSync(destPath, entry.mode);
  }
  return packageRoot(entries);
}

export function provisionPackage(archivePath, destDir, onFailure = fail) {
  try {
    return extractPackage(archivePath, destDir);
  } catch (error) {
    onFailure("provision", "invalid_package", `skill package extraction failed: ${error}`);
    return undefined;
  }
}

export function outputContract(dir) {
  return [
    "Files you create are delivered back to the person who started this run only if they are inside this directory:",
    "",
    `  ${dir}`,
    "",
    "Anything written anywhere else, including your working directory, is discarded when this run ends and nobody is told. If the task calls for producing a file, write it there, using that absolute path.",
    "",
    "This concerns where files go and nothing else. It does not change how you answer, what language you answer in, or how you approach the task.",
  ].join("\n");
}

// settingSources is intentionally left unset — passing it discovers no
// project skills on the pinned SDK. includePartialMessages is the only
// setting that makes per-response token usage appear on the stream at all.
export function agentOptions(dir) {
  return {
    cwd: workDir,
    skills: "all",
    allowedTools: ["Skill", "Read", "Write", "Edit", "Glob", "Grep", "Bash"],
    model: process.env.SKILLHUB_MODEL,
    permissionMode: "bypassPermissions",
    systemPrompt: outputContract(dir),
    includePartialMessages: true,
  };
}

const isMain = process.argv[1]
  ? pathToFileURL(process.argv[1]).href === import.meta.url
  : false;

if (isMain) {
  if (!prompt) {
    fail("execution", "missing_prompt", "SKILLHUB_USER_PROMPT is required");
  }

  const readyPath = join(inputDir, "ready");
  const inputDeadline = Date.now() + 120_000;
  while (!existsSync(readyPath)) {
    if (Date.now() > inputDeadline) {
      fail(
        "provision",
        "inputs_not_delivered",
        "the run's inputs were never delivered to the sandbox",
      );
    }
    sleepSync(200);
  }

  const skillArchive = join(inputDir, "skill.zip");
  if (existsSync(skillArchive)) {
    const staging = join(inputDir, "package");
    mkdirSync(staging, { recursive: true });
    const root = provisionPackage(skillArchive, staging);
    const packageDir = root ? join(staging, root.slice(0, -1)) : staging;

    let name = "skill";
    try {
      const frontmatter =
        readFileSync(join(packageDir, "SKILL.md"), "utf8").split(
          /^---\s*$/m,
        )[1] ?? "";
      const declared = /^name:\s*(.+)$/m.exec(frontmatter)?.[1]?.trim();
      if (declared && /^[a-z0-9]+(-[a-z0-9]+)*$/.test(declared))
        name = declared;
    } catch {
    }
    const target = join(skillDir, name);
    mkdirSync(skillDir, { recursive: true });
    renameSync(packageDir, target);
    if (root) rmSync(staging, { recursive: true, force: true });
  }
  mkdirSync(artifactDir, { recursive: true });

  const pending = new Map();

  function skillResourcePath(input) {
    const path = input?.file_path ?? input?.path ?? input?.notebook_path;
    if (typeof path !== "string" || !path.startsWith(skillDir)) return null;
    return path.slice(skillDir.length).replace(/^[/\\]+/, "");
  }

  function openToolUse(block) {
    pending.set(block.id, {
      name: block.name,
      input: block.input,
      at: Date.now(),
    });

    if (block.name === "Skill") {
      const name =
        block.input?.command ?? block.input?.skill ?? block.input?.name ?? "";
      emit("skill_activation", {
        skill_name: String(name).slice(0, 200),
        skill_version_id: process.env.SKILLHUB_SKILL_VERSION_ID ?? "",
        decision: "activated",
        reason:
          clip(block.input?.description ?? "", LIMITS.reason).value || null,
      });
    }
  }

  function closeToolUse(block) {
    const open = pending.get(block.tool_use_id);
    if (!open) return;
    pending.delete(block.tool_use_id);

    const durationMs = Date.now() - open.at;
    const failed = block.is_error === true;
    const rendered = clip(block.content, LIMITS.result);

    const resource = skillResourcePath(open.input);
    if (resource) {
      emit(
        "resource_read",
        {
          resource_path: resource.slice(0, 1024),
          outcome: failed ? "not_found" : "read",
          bytes_read: failed ? null : rendered.value.length,
          truncated: rendered.truncated,
        },
        failed ? "error" : "ok",
      );
    }

    if (open.name === "Bash") {
      const log = clip(block.content, LIMITS.message);
      emit(
        "script_log",
        {
          script_path: null,
          stream: failed ? "stderr" : "stdout",
          message: log.value,
          truncated: log.truncated,
          dropped_bytes: null,
        },
        failed ? "error" : "ok",
      );
    }

    emit(
      "tool_call",
      {
        tool_name: String(open.name).slice(0, 200),
        invocation_id: String(block.tool_use_id).slice(0, 200),
        arguments: open.input ?? null,
        result_summary: rendered.value,
        outcome: failed ? "failed" : "succeeded",
        duration_ms: durationMs,
        truncated: rendered.truncated,
      },
      failed ? "error" : "ok",
    );
  }

  const tokenCeiling = {
    input: Number(process.env.SKILLHUB_MAX_INPUT_TOKENS ?? "") || 0,
    output: Number(process.env.SKILLHUB_MAX_OUTPUT_TOKENS ?? "") || 0,
  };

  const zeroUsage = () => ({
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheWrite: 0,
  });
  // committed sums every response that finished streaming; current holds the
  // one still in flight, replaced rather than accumulated since its usage is
  // restated in full at each step.
  let committed = zeroUsage();
  let current = null;
  let responses = 0;

  function beginResponse() {
    if (current) {
      for (const k of Object.keys(committed)) committed[k] += current[k];
      responses += 1;
    }
    current = zeroUsage();
  }

  function observeUsage(u) {
    if (!u) return;
    if (!current) current = zeroUsage();
    current.input = Math.max(current.input, u.input_tokens ?? 0);
    current.output = Math.max(current.output, u.output_tokens ?? 0);
    current.cacheRead = Math.max(
      current.cacheRead,
      u.cache_read_input_tokens ?? 0,
    );
    current.cacheWrite = Math.max(
      current.cacheWrite,
      u.cache_creation_input_tokens ?? 0,
    );
  }

  function totals() {
    const t = { ...committed, responses };
    if (current) {
      for (const k of Object.keys(committed)) t[k] += current[k];
      t.responses += 1;
    }
    return t;
  }

  function ceilingBreach() {
    const t = totals();
    const input = t.input + t.cacheRead + t.cacheWrite;
    if (tokenCeiling.input && input > tokenCeiling.input) {
      return { field: "input", used: input, limit: tokenCeiling.input };
    }
    if (tokenCeiling.output && t.output > tokenCeiling.output) {
      return { field: "output", used: t.output, limit: tokenCeiling.output };
    }
    return null;
  }

  let usageEmitted = false;
  async function emitUsage(counts, tokenSource) {
    if (usageEmitted) return;
    usageEmitted = true;
    const cost = await gatewaySpend();
    emit("usage", {
      scope: "run_total",
      model: process.env.SKILLHUB_MODEL ?? "",
      input_tokens: counts.input,
      output_tokens: counts.output,
      cache_read_input_tokens: counts.cacheRead || null,
      cache_write_input_tokens: counts.cacheWrite || null,
      cost_usd: cost,
      cost_source: cost === null ? null : "gateway",
      token_source: tokenSource,
      duration_ms: Date.now() - startedAt,
    });
  }

  const messages = [];
  let output = "";
  let breach = null;
  const startedAt = Date.now();
  try {
    const { query } = await import("@anthropic-ai/claude-agent-sdk");
    for await (const msg of query({
      prompt,
      options: agentOptions(artifactDir),
    })) {
      if (msg.type === "stream_event") {
        if (msg.event?.type === "message_start") {
          beginResponse();
          observeUsage(msg.event.message?.usage);
        } else if (msg.event?.type === "message_delta") {
          observeUsage(msg.event.usage);
        }
        breach = ceilingBreach();
        if (breach) break;
        continue;
      }

      messages.push(msg.type);
      const blocks = Array.isArray(msg.message?.content)
        ? msg.message.content
        : [];
      for (const block of blocks) {
        if (block.type === "tool_use") openToolUse(block);
        else if (block.type === "tool_result") closeToolUse(block);
        else if (block.type === "text" && msg.type === "assistant") {
          const text = clip(block.text ?? "", LIMITS.text);
          if (text.value.trim() !== "") {
            emit("agent_output", {
              kind: "intermediate",
              text: text.value,
              truncated: text.truncated,
            });
          }
        }
      }

      if (msg.type === "result") {
        output = msg.result ?? "";
        const text = clip(output, LIMITS.text);
        emit(
          "agent_output",
          { kind: "final", text: text.value, truncated: text.truncated },
          msg.is_error ? "error" : "ok",
        );

        const usage = msg.usage ?? {};
        await emitUsage(
          {
            input: usage.input_tokens ?? 0,
            output: usage.output_tokens ?? 0,
            cacheRead: usage.cache_read_input_tokens ?? null,
            cacheWrite: usage.cache_creation_input_tokens ?? null,
          },
          "result",
        );
      }
    }
  } catch (err) {
    await emitUsage(totals(), "accumulated");
    fail("execution", "agent_turn_failed", String(err?.message ?? err));
  }

  if (breach) {
    const message =
      `run stopped at its ${breach.field} token ceiling: ${breach.used} of ${breach.limit} tokens ` +
      `(PDM-005 5.2a; the limit shown in the pre-run permission summary)`;
    emit(
      "error",
      {
        category: "execution",
        code: "token_budget_exceeded",
        message,
        retryable: false,
      },
      "error",
    );
    await emitUsage(totals(), "accumulated");
    finish(
      "failed",
      { error: message, agent_output: output, message_types: messages },
      EXIT_TOKEN_BUDGET,
    );
  }

  await emitUsage(totals(), "accumulated");

  finish("succeeded", { agent_output: output, message_types: messages });
}
