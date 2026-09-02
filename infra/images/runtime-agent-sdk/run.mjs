// Sandbox entrypoint: unpack the skill, run one agent turn, write the result.
//
// Everything it needs arrives in the environment (SBX-008). The provider passes
// paths and short-lived URLs and never opens what is behind them, so this file
// is the first thing that touches untrusted bytes — and it does so inside the
// sandbox, which is the whole point (iron rule 1).
//
// The Skill-load conditions are all here, because missing any one of them
// silently loads no skills at all:
//   1. cwd points at the directory holding .claude/skills/
//   2. settingSources is *omitted*. This is the opposite of what the PDM-003
//      spike measured on claude-agent-sdk 0.2.137, and it was re-measured on the
//      0.3.233 the image pins: `settingSources: ["project"]` discovers no
//      project skill at all there, while omitting the option does. Excluding
//      "user" was the spike's way of keeping a stray ~/.claude out of a run;
//      HOME is the run's own per-run tmpfs (/work), so there is no other home to
//      leak from and nothing is given up by omitting it.
//   3. skills: "all" — 0.3.x moved skill enablement out of allowedTools, where
//      passing "Skill" is now deprecated, into this option
//   4. the tool list still names the tools the workload may use
//   5. includePartialMessages: true — the only place a per-response token count
//      appears (see the token accounting section). Its absence fails silently
//      too: every count reads zero.
//
// A change to any of these must be re-measured against the pinned SDK, not
// reasoned about: every one of them fails silently.
//
// It is also the only place that sees a per-response token count, which makes it
// two other things: the Run Trace's usage reporter (TRACE-004) and the point
// where the PDM-005 token ceiling is enforced. Both are structured around the
// same fact - the SDK's `result` message is not guaranteed to arrive, so nothing
// that must happen once per run may hang off it.
//
// It is also the Run Trace producer (TRACE-002~004). Events are appended, one
// JSON object per line, to $SKILLHUB_OUTDIR/trace/events.jsonl. They are not
// posted anywhere from in here: the only address this container can reach is the
// model gateway (SBX-007), so sandboxd reads the file out and pushes it. The
// same asymmetry is why the run's inputs arrive as files rather than as URLs.
import { appendFileSync, existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";
import { inflateRawSync } from "node:zlib";
// Loaded dynamically, only inside the isMain-guarded entrypoint block below,
// rather than as a static top-level import: a static import is resolved
// before any of this module's own code runs, which would make extractPackage
// impossible to unit-test without the SDK installed in the test environment
// too. The container image installs it (Dockerfile); nothing else here needs
// it, and this changes nothing about what runs when SKILLHUB_* is the
// entrypoint.

const workDir = process.env.SKILLHUB_WORKDIR ?? "/work";
const outDir = process.env.SKILLHUB_OUTDIR ?? "/out";
const skillDir = process.env.SKILLHUB_SKILL_DIR ?? join(workDir, ".claude", "skills");
const inputDir = process.env.SKILLHUB_INPUT_DIR ?? join(workDir, ".skillhub");
const artifactDir = process.env.SKILLHUB_ARTIFACT_DIR ?? join(outDir, "artifacts");
const prompt = process.env.SKILLHUB_USER_PROMPT;

// --- trace emission ---------------------------------------------------------

const traceDir = join(outDir, "trace");
const tracePath = join(traceDir, "events.jsonl");
const runId = process.env.SKILLHUB_RUN_ID ?? "";
const attempt = Number(process.env.SKILLHUB_ATTEMPT ?? "1") || 1;

// seq is gapless from 1 and scoped to (run_id, attempt, emitted_by) — this
// process is the only `sandbox` producer for this attempt, so a hole in the
// stored sequence means an event was lost in transit and nothing else
// (contracts/events/README.md §7).
let seq = 0;
let traceReady = false;

// The schema's maxLength values, so a runaway tool result is cut here rather
// than rejected whole by the platform. Cutting sets the payload's `truncated`
// flag where the type has one, so the UI never shows a clipped string as
// complete.
const LIMITS = { message: 16000, text: 64000, result: 8000, reason: 2000, error: 4000 };

function clip(value, limit) {
  const s = typeof value === "string" ? value : JSON.stringify(value ?? null);
  if (s.length <= limit) return { value: s, truncated: false };
  return { value: s.slice(0, limit), truncated: true };
}

function emit(type, payload, status = "ok") {
  if (!runId) return; // nothing to attribute the event to
  if (!traceReady) {
    mkdirSync(traceDir, { recursive: true });
    traceReady = true;
  }
  seq += 1;
  const event = {
    // 1.1 adds usage.token_source, which this producer writes.
    schema_version: "1.1",
    event_id: randomUUID(),
    run_id: runId,
    attempt,
    seq,
    occurred_at: new Date().toISOString(),
    emitted_by: "sandbox",
    type,
    status,
    // Masking is the control plane's job and happens before storage
    // (TRACE-005, iron rule 11). Claiming `true` here would be this container
    // vouching for itself, which is exactly what the trust boundary forbids.
    masked: false,
    masked_fields: [],
    payload,
  };
  try {
    // One append per event, not one write at the end: sandboxd copies this file
    // while the run is still going, and a buffered writer would mean the trace
    // only ever appears after the workload has finished.
    appendFileSync(tracePath, JSON.stringify(event) + "\n");
  } catch {
    // A full /out must not take the run down with it. The gap shows up as a
    // hole in seq, which is precisely how a lost event is meant to surface.
  }
}

// EXIT_TOKEN_BUDGET is how this process tells the node *why* it stopped, since
// the provider reads the exit code and not result.json. It must stay in step
// with exitTokenBudget in apps/sandbox/internal/sandbox/manager.go, which
// turns it into a RunError message a user can read; any other non-zero code is
// an ordinary workload failure.
const EXIT_TOKEN_BUDGET = 9;

function finish(status, extra = {}, exitCode = status === "succeeded" ? 0 : 1) {
  // The provider reads state from the container, not from this file; the file
  // is for the artifact manifest and the agent's own output.
  writeFileSync(join(outDir, "result.json"), JSON.stringify({ status, ...extra }, null, 2));
  waitToBeCollected();
  process.exit(exitCode);
}

// Atomics.wait blocks the calling call stack without spawning anything, unlike
// the "/bin/sleep" this used to shell out to — which does not exist on the
// Windows host this file must also run on now (04 丙-82) and would throw
// uncaught there. Both busy-wait loops below need genuinely synchronous
// blocking (see each one's own comment for why an event-based wait will not
// do), and Atomics.wait on a throwaway SharedArrayBuffer is the stdlib's way
// to get that: the calling frame does not return until the timeout elapses,
// no callback and no event-loop turn involved.
function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

// waitToBeCollected is the second half of the collection handshake, and the run
// is not finished without it: /out is a tmpfs, so the tail of the trace and
// every artifact vanish the instant this process exits. Announcing completion
// and waiting lets the execution node take them out first.
//
// Bounded, and a timeout is not an error: the node may have no reason to collect
// (no ingestion URL, no write grant) and a run must not hang because nobody came.
function waitToBeCollected() {
  const collected = join(outDir, ".collected");
  try {
    writeFileSync(join(outDir, ".workload-done"), "done\n");
  } catch {
    return; // nowhere to announce it; nothing will be collected either
  }
  const deadline = Date.now() + 60_000;
  while (!existsSync(collected) && Date.now() < deadline) {
    sleepSync(200);
  }
}

// fail records the error in the trace before ending the run, so a failure the
// user can see in the timeline and the exit code always agree (TRACE-004).
function fail(category, code, message) {
  const clipped = clip(message, LIMITS.error);
  emit("error", { category, code, message: clipped.value, retryable: false }, "error");
  finish("failed", { error: clipped.value });
}

// --- package extraction (no `unzip` — see extractPackage below) -------------
//
// This used to be the one external call in this file that was also a security
// boundary: `unzip -q -o`'s own refusal of absolute paths and `../` entries
// was what bounded a hostile skill package (iron rule 1 — this is the first
// code in the whole system that opens one). `unzip` does not exist on the
// Windows host this file must now also run on (04 丙-82), so that refusal has
// to be reimplemented by hand, entry by entry, using nothing but the
// standard library — and it has to be the *same* reimplementation on both
// platforms, not two hand-tuned approximations of it, because a check that
// passes on one platform and not the other is exactly the kind of gap a
// crafted package would be built to find.

// Bounds on the *input* direction. The output direction has had these since
// SBX-008 (artifactMaxEntries = 1000 and the per-file/per-run byte ceilings in
// apps/sandbox/internal/sandbox/artifacts.go); the input direction had none,
// and this is the code that opens the first untrusted bytes in the whole system
// (iron rule 1).
//
// It matters more here than it did under `unzip`, which had no compression-ratio
// guard either: in a container the blast radius was a cgroup memory ceiling and
// an OOM the driver reports as an execution failure, but clean mode runs this on
// a host process where Linux enforces nothing and Windows' Job Object kills the
// whole job — a run with no trace, no artifacts and no stated reason. Taking
// over a security boundary is the moment to bound it, not a later one.
//
// MAX_PACKAGE_ENTRIES mirrors artifactMaxEntries for the reason its comment
// gives: an empty file costs no bytes and still costs a filesystem entry, so a
// byte ceiling alone is not a bound. 1000 is the same number on both sides so
// that a package a run could not have produced is also one it cannot consume.
//
// MAX_PACKAGE_TOTAL_BYTES is 256 MiB: comfortably above any real skill package
// (the largest in the M2 catalog is under 2 MiB) and comfortably below the
// smallest memory ceiling any deployment gives this process, so the refusal
// arrives as a message rather than as an OOM kill.
//
// The total is checked against what the central directory *declares*, before a
// single byte is inflated or written — a running total of what came out would
// only notice after a quarter of a gigabyte was already on disk. Declaring less
// than the truth is not a way around it, because the two checks compose: a
// deflated entry cannot exceed its own declared size (maxOutputLength in
// readEntryData) and a stored entry's two sizes must be equal (checked there
// too), so the declared total is an upper bound on what actually lands.
const MAX_PACKAGE_ENTRIES = 1000;
const MAX_PACKAGE_TOTAL_BYTES = 256 * 1024 * 1024;

const EOCD_SIGNATURE = 0x06054b50;
const CENTRAL_DIR_SIGNATURE = 0x02014b50;
const LOCAL_FILE_SIGNATURE = 0x04034b50;
const ZIP64_EOCD_LOCATOR_SIGNATURE = 0x07064b50;

// The end-of-central-directory record carries a variable-length comment (0 to
// 65535 bytes), so its offset cannot be read directly and is found by
// scanning backward for the signature instead.
function findEndOfCentralDirectory(buf) {
  const minPos = Math.max(0, buf.length - 22 - 65535);
  for (let i = buf.length - 22; i >= minPos; i -= 1) {
    if (buf.readUInt32LE(i) === EOCD_SIGNATURE) return i;
  }
  throw new Error("not a zip file: end of central directory record not found");
}

// A path segment is checked against both "/" and "\": POSIX only treats "/"
// as a separator, so a "..\\x" entry looks like one harmless filename there —
// but Node's own path.join treats "\\" as a separator on Windows, so the same
// bytes become a real parent-directory escape the moment this file runs on
// the Windows target this task exists for. Both platforms run this same
// check, so both refuse the same entries regardless of which one is doing the
// unzipping today.
function isUnsafeEntryName(name) {
  if (name === "") return true;
  if (name.startsWith("/") || name.startsWith("\\")) return true; // posix-absolute or UNC
  if (/^[a-zA-Z]:/.test(name)) return true; // Windows drive-absolute, e.g. "C:\..." or "C:foo"
  return name.split(/[/\\]+/).some((segment) => segment === "..");
}

// Unix file-type bits live in the top 16 bits of external_attr. A symlink
// pointing outside the staging directory is the second escape route past the
// path check above (unzip would create it as a symlink, faithfully, and
// whatever reads through it later reads through it wherever it points); a
// device, fifo, or socket entry has no business inside a skill package
// either. A package built with no unix mode bits at all — external_attr's top
// 16 bits all zero, the ordinary case for a deflate-only zip made by a tool
// that never set them — is not rejected by this check.
const S_IFMT = 0xf000;
const S_IFREG = 0x8000;
const S_IFDIR = 0x4000;

function isUnsafeFileType(externalAttr) {
  const fileType = (externalAttr >>> 16) & S_IFMT;
  return fileType !== 0 && fileType !== S_IFREG && fileType !== S_IFDIR;
}

// Reads and validates every central directory entry. Anything this parser
// does not implement — Zip64 (large-archive) records, encrypted entries, a
// compression method other than store or deflate — is refused loudly here
// rather than misread: a parser that silently skips bytes it does not
// understand is a parser that can be made to disagree with itself about where
// an entry's data actually starts.
function readCentralDirectory(buf) {
  const eocdOffset = findEndOfCentralDirectory(buf);
  const totalEntries = buf.readUInt16LE(eocdOffset + 10);
  const cdSize = buf.readUInt32LE(eocdOffset + 12);
  const cdOffset = buf.readUInt32LE(eocdOffset + 16);

  if (totalEntries === 0xffff || cdSize === 0xffffffff || cdOffset === 0xffffffff) {
    throw new Error("unsupported zip feature: zip64 archive");
  }
  if (totalEntries > MAX_PACKAGE_ENTRIES) {
    throw new Error(`refusing oversized skill package: ${totalEntries} entries exceeds the ${MAX_PACKAGE_ENTRIES} entry limit`);
  }
  if (eocdOffset >= 4 && buf.readUInt32LE(eocdOffset - 4) === ZIP64_EOCD_LOCATOR_SIGNATURE) {
    throw new Error("unsupported zip feature: zip64 archive");
  }

  const entries = [];
  let declaredBytes = 0;
  let pos = cdOffset;
  for (let i = 0; i < totalEntries; i += 1) {
    if (buf.readUInt32LE(pos) !== CENTRAL_DIR_SIGNATURE) {
      throw new Error("malformed zip: central directory entry signature mismatch");
    }
    const generalFlag = buf.readUInt16LE(pos + 8);
    const method = buf.readUInt16LE(pos + 10);
    const compressedSize = buf.readUInt32LE(pos + 20);
    const uncompressedSize = buf.readUInt32LE(pos + 24);
    const nameLen = buf.readUInt16LE(pos + 28);
    const extraLen = buf.readUInt16LE(pos + 30);
    const commentLen = buf.readUInt16LE(pos + 32);
    const externalAttr = buf.readUInt32LE(pos + 38);
    const localHeaderOffset = buf.readUInt32LE(pos + 42);
    const name = buf.toString("utf8", pos + 46, pos + 46 + nameLen);

    if (compressedSize === 0xffffffff || uncompressedSize === 0xffffffff || localHeaderOffset === 0xffffffff) {
      throw new Error(`unsupported zip feature: zip64 entry (${name})`);
    }
    // Bit 0 of the general-purpose flag is the encryption bit (traditional or
    // strong encryption); this parser holds no keys and must not treat cipher
    // bytes as if they were the plaintext they are not.
    if (generalFlag & 0x1) {
      throw new Error(`unsupported zip feature: encrypted entry (${name})`);
    }
    if (method !== 0 && method !== 8) {
      throw new Error(`unsupported zip feature: compression method ${method} (${name})`);
    }
    if (isUnsafeEntryName(name)) {
      throw new Error(`refusing unsafe zip entry path: ${name}`);
    }
    if (isUnsafeFileType(externalAttr)) {
      throw new Error(`refusing non-regular zip entry: ${name}`);
    }

    entries.push({ name, method, compressedSize, uncompressedSize, localHeaderOffset });
    declaredBytes += uncompressedSize;
    if (declaredBytes > MAX_PACKAGE_TOTAL_BYTES) {
      throw new Error(
        `refusing oversized skill package: declared ${declaredBytes} bytes exceeds the ${MAX_PACKAGE_TOTAL_BYTES} byte limit`,
      );
    }
    pos += 46 + nameLen + extraLen + commentLen;
  }
  return entries;
}

// The local file header repeats (and can disagree in length with) the name
// and extra field from the central directory, so the entry's actual data
// offset can only be computed by reading the local header, not assumed from
// the central directory alone.
function readEntryData(buf, entry) {
  const lh = entry.localHeaderOffset;
  if (buf.readUInt32LE(lh) !== LOCAL_FILE_SIGNATURE) {
    throw new Error(`malformed zip: local file header signature mismatch (${entry.name})`);
  }
  const nameLen = buf.readUInt16LE(lh + 26);
  const extraLen = buf.readUInt16LE(lh + 28);
  const dataStart = lh + 30 + nameLen + extraLen;
  const compressed = buf.subarray(dataStart, dataStart + entry.compressedSize);
  if (entry.method !== 8) {
    // "Stored" means the two sizes are the same number. Checking it is what
    // makes the declared total above an honest bound for this branch as well:
    // without it an entry could declare one byte and carry a hundred megabytes.
    if (entry.compressedSize !== entry.uncompressedSize) {
      throw new Error(
        `malformed zip: stored entry declares ${entry.uncompressedSize} bytes but carries ${entry.compressedSize} (${entry.name})`,
      );
    }
    return Buffer.from(compressed);
  }
  // The entry's own declared uncompressed size is the bound. An archive that
  // says 100 bytes and inflates to 10 MB is lying about itself, and this is
  // where the lie stops being free — the central directory's size field was
  // read already and until now was only checked for the zip64 sentinel.
  // maxOutputLength must be at least 1, so a legitimately empty entry gets 1.
  try {
    return inflateRawSync(compressed, { maxOutputLength: Math.max(1, entry.uncompressedSize) });
  } catch (err) {
    if (err?.code === "ERR_BUFFER_TOO_LARGE") {
      throw new Error(
        `refusing zip entry that inflates past its declared size of ${entry.uncompressedSize} bytes: ${entry.name}`,
      );
    }
    throw err;
  }
}

// extractPackage is the whole of what `unzip -q -o <archivePath> -d <destDir>`
// did: every entry's tree position is preserved — `unzip -j`'s flattening is
// exactly what would break the directory layout the Agent Skills spec depends
// on — and a file already at the destination is silently replaced, matching
// `-o`. What it adds is the entry-by-entry refusal above, running as the same
// code on every platform this file executes on instead of leaning on a binary
// that exists on only one of them.
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
  }
}

// --- what the agent is told about where output goes -------------------------
//
// Until 2026-09-02 the agent was told nothing, and `/out` was a convention that
// existed only in the collector. What that cost, measured on 2026-08-31 (04
// 丙-106): given a task that had to produce a file, the agent ran
// `Get-ChildItem /out` (failed: `Cannot find path 'C:\out'`), then wrote to
// `/out/announcement.md` (succeeded), then said it had created the file — while
// `GET /runs/{id}/artifacts` came back empty and the host had gained a stray
// `C:\outnnouncement.md`.
//
// The Windows host is only where it became visible. Both drivers collect
// **<outDir>/artifacts** and nothing else (localdrv/transfer.go walks
// artifactDir; dockerdrv runs `tar -C /out artifacts`), so a file written to
// `/out` directly, or to the working directory, was never going to be handed
// back on any platform — a container just made the write succeed quietly.
//
// So this is not a clean-mode patch: it is the machine behind a convention that
// never had one. The agent gets the absolute path of the directory that is
// actually collected, on whichever platform it is running.
//
// **This is a change to every run, not only the clean test mode**, and it is
// the only steering this harness does beyond the user's own prompt. Kept to
// file placement on purpose: the sentence about not changing anything else is
// there because a system prompt is the one input that silently competes with
// the user's, and 02:TEST-* judges runs on what the user asked for.
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

// agentOptions is every option the SDK turn is given, in one place a test can
// read. It exists because of the header's own warning: each of these fails
// **silently** — the wrong settingSources loads no skills, a missing
// includePartialMessages reports every token count as zero — and until now
// there was nothing asserting any of them was still there.
//
// systemPrompt is passed as a **string**, and that is a measured choice rather
// than a stylistic one. In 0.3.233 the SDK maps the option like this:
// `if (s === undefined) p = ""` — so omitting it, which is what this file did
// until now, sends an explicitly empty system prompt, not Claude Code's preset.
// A string replaces that empty one and changes nothing else. The
// `{ type: "preset", preset: "claude_code", append }` form would instead switch
// the run to the **whole** Claude Code system prompt, which is a different
// product's behaviour arriving as a side effect of naming a directory.
export function agentOptions(dir) {
  return {
    cwd: workDir,
    // settingSources deliberately omitted — see the header.
    skills: "all",
    allowedTools: ["Skill", "Read", "Write", "Edit", "Glob", "Grep", "Bash"],
    model: process.env.SKILLHUB_MODEL,
    permissionMode: "bypassPermissions",
    systemPrompt: outputContract(dir),
    // The only source of per-response token counts — see the accounting
    // section. Without it a turn that produces no result reports nothing.
    includePartialMessages: true,
  };
}

// True only when this module is the process entrypoint (`node run.mjs`), not
// when it is imported — which is what lets run.test.mjs import extractPackage
// without also running the sandbox workload it decorates below.
const isMain = process.argv[1] ? pathToFileURL(process.argv[1]).href === import.meta.url : false;

if (isMain) {
if (!prompt) {
  fail("execution", "missing_prompt", "SKILLHUB_USER_PROMPT is required");
}

// --- inputs -----------------------------------------------------------------
//
// This container cannot reach object storage: its only route out is the model
// gateway (SBX-007). The execution node fetches the run's inputs with the
// short-lived grants it was dispatched with and writes them in here, then drops
// the ready marker last. Waiting for that marker is the whole handshake - the
// container starts before its inputs are in place, and guessing would race.
const readyPath = join(inputDir, "ready");
const inputDeadline = Date.now() + 120_000;
while (!existsSync(readyPath)) {
  if (Date.now() > inputDeadline) {
    fail("provision", "inputs_not_delivered", "the run's inputs were never delivered to the sandbox");
  }
  // Busy-wait on a tmpfs path: there is no event to subscribe to across the
  // exec boundary, and the wait is normally a few hundred milliseconds.
  sleepSync(200);
}

// Unpack the skill package, which ingest stored as a zip. The hash is verified
// by the control plane before the grant is minted; re-checking it here would
// need the archive on disk twice, and a sandbox vouching for its own inputs
// proves nothing anyway.
//
// This is the first thing in the whole system that opens the package, and it is
// deliberately the least privileged place there is (iron rule 1). extractPackage
// (defined above) is what bounds it now — see its own comment block for why it
// exists instead of a shell-out to `unzip`.
//
// It unpacks into <skillDir>/<name>/, not into <skillDir>: the Agent SDK
// discovers one directory per skill, so a package emptied straight into the
// skills root would be discovered as no skill at all (PDM-003 spike §10). The
// name comes from the package's own SKILL.md frontmatter, because the frozen
// RunRequest does not carry one — and reading it here is exactly where reading
// untrusted content belongs.
const skillArchive = join(inputDir, "skill.zip");
if (existsSync(skillArchive)) {
  const staging = join(inputDir, "package");
  mkdirSync(staging, { recursive: true });
  extractPackage(skillArchive, staging);

  let name = "skill";
  try {
    const frontmatter = readFileSync(join(staging, "SKILL.md"), "utf8").split(/^---\s*$/m)[1] ?? "";
    const declared = /^name:\s*(.+)$/m.exec(frontmatter)?.[1]?.trim();
    // Only a spec-shaped name is used as a path segment; anything else keeps the
    // fallback rather than becoming a directory name of the package's choosing.
    if (declared && /^[a-z0-9]+(-[a-z0-9]+)*$/.test(declared)) name = declared;
  } catch {
    // No readable SKILL.md: the skill will simply not be discovered, and the
    // trace will show no activation, which is the honest outcome.
  }
  const target = join(skillDir, name);
  mkdirSync(skillDir, { recursive: true });
  renameSync(staging, target);
}
// The agent writes here and the node collects it after the turn (SBX-008).
mkdirSync(artifactDir, { recursive: true });

// --- agent turn -------------------------------------------------------------

// Tool calls are traced as one completed event each, carrying their own
// duration (TRACE-003, contracts/events/README.md §3): the tool_use block opens
// an entry here and the matching tool_result closes it.
const pending = new Map();

// A path the agent read that lives under the skill package is a resource read
// (TRACE-002); anything else is an ordinary file tool call and stays one.
function skillResourcePath(input) {
  const path = input?.file_path ?? input?.path ?? input?.notebook_path;
  if (typeof path !== "string" || !path.startsWith(skillDir)) return null;
  // Relative to the package root: absolute sandbox paths are not exported.
  return path.slice(skillDir.length).replace(/^[/\\]+/, "");
}

function openToolUse(block) {
  pending.set(block.id, { name: block.name, input: block.input, at: Date.now() });

  if (block.name === "Skill") {
    // TRACE-002. The SDK surfaces a skill being used as a `Skill` tool call;
    // a skill that was available and never invoked produces no block at all, so
    // `skipped` is not something this producer can observe and is left to
    // whoever can (the evaluator, EVAL-002). Recording a guess would be worse
    // than recording nothing.
    const name = block.input?.command ?? block.input?.skill ?? block.input?.name ?? "";
    emit("skill_activation", {
      skill_name: String(name).slice(0, 200),
      // The platform knows which version it mounted; the sandbox only ever sees
      // a directory name, so skill_version_id is filled in by nobody here and
      // the field is left out rather than invented.
      skill_version_id: process.env.SKILLHUB_SKILL_VERSION_ID ?? "",
      decision: "activated",
      reason: clip(block.input?.description ?? "", LIMITS.reason).value || null,
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
    emit("resource_read", {
      resource_path: resource.slice(0, 1024),
      outcome: failed ? "not_found" : "read",
      bytes_read: failed ? null : rendered.value.length,
      truncated: rendered.truncated,
    }, failed ? "error" : "ok");
  }

  // TRACE-003: script output. Bash is the only tool in the allow list that runs
  // a script, and its result is that script's combined output.
  if (open.name === "Bash") {
    const log = clip(block.content, LIMITS.message);
    emit("script_log", {
      script_path: null,
      stream: failed ? "stderr" : "stdout",
      message: log.value,
      truncated: log.truncated,
      dropped_bytes: null, // unknown, and the UI must show unknown rather than 0
    }, failed ? "error" : "ok");
  }

  emit("tool_call", {
    tool_name: String(open.name).slice(0, 200),
    invocation_id: String(block.tool_use_id).slice(0, 200),
    arguments: open.input ?? null,
    result_summary: rendered.value,
    outcome: failed ? "failed" : "succeeded",
    duration_ms: durationMs,
    truncated: rendered.truncated,
  }, failed ? "error" : "ok");
}

// gatewaySpend asks the model gateway what this run's own Virtual Key has spent
// (TRACE-004, ADR-017). The key can read itself and nothing else, so this needs
// no admin credential and discloses no other run.
//
// The gateway flushes spend asynchronously, so the first non-zero reading is an
// undercount: the last calls of the turn are still in flight when it is taken.
// This reads until the figure stops moving, then reports that. Any failure
// answers null, which the schema and the UI both read as "unreported" - a run
// must not fail because its accounting was slow.
//
// It is still a reading, not a ledger: a flush that lands after the last poll is
// missed, and the gateway's own per-key spend remains the number to reconcile
// against (ADR-017 - the gateway is a metering source, not the fact).
async function gatewaySpend() {
  const base = process.env.ANTHROPIC_BASE_URL;
  const key = process.env.ANTHROPIC_AUTH_TOKEN;
  if (!base || !key) return null;
  let last = null;
  // The turn's last call is charged after the result message arrives, so a
  // reading taken immediately is stable and wrong. Measured against the gateway's
  // own spend log: without this pause the figure was short by exactly the final
  // call, every time.
  await new Promise((resolve) => setTimeout(resolve, 2500));
  for (let attempt = 0; attempt < 6; attempt += 1) {
    let spend = null;
    try {
      const res = await fetch(`${base.replace(/\/$/, "")}/key/info`, {
        headers: { Authorization: `Bearer ${key}` },
      });
      if (res.ok) {
        const body = await res.json();
        if (typeof body?.info?.spend === "number") spend = body.info.spend;
      }
    } catch {
      // Unreachable gateway: nothing to report, and nothing to fail over.
    }
    if (spend !== null && spend > 0 && spend === last) return spend;
    if (spend !== null) last = spend;
    await new Promise((resolve) => setTimeout(resolve, 1500));
  }
  return last !== null && last > 0 ? last : null;
}

// --- token accounting and the PDM-005 ceiling --------------------------------
//
// Two jobs, one accumulator.
//
// 1. TRACE-004. usage used to be emitted only on the SDK's `result` message, so
//    a turn that ended any other way reported no usage at all - not zero, no
//    event. Measured on the 45-skill baseline: `add-iso3166` succeeded, produced
//    its artifact, traced 13 events with no gap, and cost the platform nothing
//    it could see, because that turn produced no result message. Summed over the
//    batch the trace said $3.0879 against the gateway's $3.3932. So every model
//    response is counted here as it streams, and a run-total usage event is
//    emitted exactly once however the turn ends (§token_source in the contract).
//
// 2. PDM-005 5.2a. The 300K/60K ceiling is shown to the user in the pre-run
//    permission summary and they confirm it, so something has to enforce it.
//    The counting has to happen where the numbers are, and the only place that
//    sees a per-response token count is right here: RunUsage carries none back
//    to the platform, and the gateway's brakes are a spend brake and a rate
//    brake, which prompt caching decoupled from token count by 7-8x.
//
// Honest about what kind of brake this is: it is a cooperative stop inside the
// sandbox, the same class as the wall-clock soft limit, and the hard brakes
// remain the Virtual Key's max_budget and the wall-clock kill outside. It stops
// a runaway agent loop, which is what the limit is for; it is not a defence
// against a workload attacking its own harness, which already holds the key.
// Where the numbers come from, measured on the pinned SDK against the gateway
// rather than assumed, because the obvious place is empty:
//
//   assistant message  msg.message.usage  -> all four fields ZERO, every time,
//                      and the same message.id arrives more than once, so summing
//                      it would be summing zeros twice.
//   result message     msg.usage          -> the real end-of-turn total, but only
//                      if a result arrives at all, which is the whole problem.
//   stream_event       event.message.usage on message_start, event.usage on
//                      message_delta -> the real per-response figures. Here the
//                      gateway puts everything on message_delta and zeros on
//                      message_start; the Anthropic API splits input at start and
//                      output at delta. Taking the per-field maximum over one
//                      response is right for both and needs no knowledge of which.
//
// That is why `includePartialMessages: true` is on the query: without it there is
// no per-response token count anywhere in the stream. Like the other options in
// the header, this was measured on the pinned SDK and must be re-measured, not
// reasoned about, on an upgrade - it fails by reporting zeros, silently.
const tokenCeiling = {
  input: Number(process.env.SKILLHUB_MAX_INPUT_TOKENS ?? "") || 0,
  output: Number(process.env.SKILLHUB_MAX_OUTPUT_TOKENS ?? "") || 0,
};

const zeroUsage = () => ({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0 });
// committed is every response that has finished streaming; current is the one in
// flight, replaced rather than added to, because its usage is restated in full at
// each step rather than sent as an increment.
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
  current.cacheRead = Math.max(current.cacheRead, u.cache_read_input_tokens ?? 0);
  current.cacheWrite = Math.max(current.cacheWrite, u.cache_creation_input_tokens ?? 0);
}

// totals is what has been counted so far, including the response still streaming:
// tokens already sent are already spent, so a ceiling check must see them.
function totals() {
  const t = { ...committed, responses };
  if (current) {
    for (const k of Object.keys(committed)) t[k] += current[k];
    t.responses += 1;
  }
  return t;
}

// Cache-hit tokens count against the ceiling: caching reduces cost, not context
// (PDM-005 5.2a-3). LiteLLM's /v1/messages route reports neither cache field, so
// today this is just input_tokens - which is also exactly the figure the run
// summaries already show, so the ceiling means what the summary said it meant.
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

// emitUsage sends the one run-total usage event, at most once. Cost is asked of
// the gateway either way: the gateway knows what was charged whether or not the
// SDK produced an end-of-turn total (iron rule 8), so cost_source stays
// `gateway` and token_source carries the separate question of where the token
// counts came from.
//
// The SDK's total_cost_usd is still deliberately unused: it is computed from a
// local price table for a model name the gateway resolves, so it would be an
// estimate wearing the gateway's name. `cost_usd: null` means the gateway did
// not answer, and the UI must render that as unreported rather than as zero.
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
  for await (const msg of query({ prompt, options: agentOptions(artifactDir) })) {
    // Token counting first: a stream_event carries nothing else this loop wants,
    // and it is the only message type that carries a usage figure that is not
    // zero. It is also not recorded in message_types, which would otherwise be
    // hundreds of identical entries in result.json.
    if (msg.type === "stream_event") {
      if (msg.event?.type === "message_start") {
        beginResponse();
        observeUsage(msg.event.message?.usage);
      } else if (msg.event?.type === "message_delta") {
        observeUsage(msg.event.usage);
      }
      // PDM-005 5.2a, checked here because this is the only point at which the
      // count can move. Breaking rather than killing the process closes the
      // SDK's iterator the ordinary way: no further model call is made, the
      // events already emitted stay in the timeline, and the collection
      // handshake below still runs, so artifacts and trace are not lost.
      breach = ceilingBreach();
      if (breach) break;
      continue;
    }

    messages.push(msg.type);
    const blocks = Array.isArray(msg.message?.content) ? msg.message.content : [];
    for (const block of blocks) {
      if (block.type === "tool_use") openToolUse(block);
      else if (block.type === "tool_result") closeToolUse(block);
      else if (block.type === "text" && msg.type === "assistant") {
        const text = clip(block.text ?? "", LIMITS.text);
        if (text.value.trim() !== "") {
          emit("agent_output", { kind: "intermediate", text: text.value, truncated: text.truncated });
        }
      }
    }

    if (msg.type === "result") {
      output = msg.result ?? "";
      const text = clip(output, LIMITS.text);
      emit("agent_output", { kind: "final", text: text.value, truncated: text.truncated },
        msg.is_error ? "error" : "ok");

      // TRACE-004. The SDK's end-of-turn total wins over the running sum when it
      // exists: it is the same quantity from a vantage point that also sees any
      // response still in flight when the stream closed.
      const usage = msg.usage ?? {};
      await emitUsage({
        input: usage.input_tokens ?? 0,
        output: usage.output_tokens ?? 0,
        cacheRead: usage.cache_read_input_tokens ?? null,
        cacheWrite: usage.cache_creation_input_tokens ?? null,
      }, "result");
    }
  }
} catch (err) {
  // A crashed turn is still a turn that spent money. Report what was counted
  // before the failure, then fail - in that order, because fail() exits.
  await emitUsage(totals(), "accumulated");
  fail("execution", "agent_turn_failed", String(err?.message ?? err));
}

if (breach) {
  const message =
    `run stopped at its ${breach.field} token ceiling: ${breach.used} of ${breach.limit} tokens ` +
    `(PDM-005 5.2a; the limit shown in the pre-run permission summary)`;
  emit("error", { category: "execution", code: "token_budget_exceeded", message, retryable: false }, "error");
  await emitUsage(totals(), "accumulated");
  finish("failed", { error: message, agent_output: output, message_types: messages }, EXIT_TOKEN_BUDGET);
}

// The turn ended without a result message: no crash, no ceiling, just an SDK
// that produced none (measured on add-iso3166, which succeeded and produced its
// artifact). Reporting the running sum is what keeps that run's cost from
// vanishing; emitUsage is a no-op if the result branch already reported.
await emitUsage(totals(), "accumulated");

finish("succeeded", { agent_output: output, message_types: messages });
} // isMain
