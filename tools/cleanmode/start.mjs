// Clean test mode's launcher (02:PORT-005).
//
// It is a Node script rather than a shell script or a Task target because Node
// is already a hard requirement here - the database carrier is a Node process -
// and the machine this mode exists for is Windows. One script that runs on both
// platforms beats two that drift.
//
// It starts three processes and nothing else: the PGlite carrier, the API
// binary with SKILLHUB_CLEAN_MODE set (which also runs the workers in-process,
// ADR-060 decision 6), and sandboxd with the same flag. There is exactly one
// flag; this script does not invent a second way to configure any single axis.
//
// Every preflight failure names what is missing and how to get it. That is the
// acceptance criterion, and on the machine this is for it is also the only
// diagnostic anyone will get: "failed to start" on a locked-down workstation
// tells the person in front of it nothing they can act on.
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { childOverlay, readDotEnv, resolve } from "./env.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");

const API_PORT = Number(process.env.CLEAN_MODE_API_PORT ?? 8080);
const SANDBOX_PORT = Number(process.env.CLEAN_MODE_SANDBOX_PORT ?? 8081);
const PGLITE_PORT = Number(process.env.CLEAN_MODE_PGLITE_PORT ?? 5433);

// The repository's .env. Precedence, lowest first: .env, then the shell, then
// what this launcher owns — the launcher wins because the values it sets are
// ones it also acts on (it prints the URL it chose, so a .env that renamed
// API_ADDR would produce a launch whose own instructions are wrong). The rules
// and the reasons for each live in env.mjs, which is where they can be tested.
const dotEnv = readDotEnv(join(repoRoot, ".env"));

/** A deployment variable as this launch sees it. */
const deployment = (name) => resolve(dotEnv, process.env, name);

/** What .env contributes to the API child, and to nothing else. */
const dotEnvForApi = childOverlay(dotEnv, process.env);

// The account `devctl seed-clean` logs in as. It must equal seedDevLoginUser in
// tools/devctl/seed_clean.go; TestTheLauncherGrantsTheSeedImporterACatalogWorkspace
// there fails if the two ever drift apart.
const SEED_IMPORTER = "seed-importer";

const children = [];
let shuttingDown = false;

function fail(what, howToFix) {
  console.error(`\nclean mode cannot start: ${what}`);
  console.error(`  ${howToFix}`);
  process.exit(1);
}

function has(cmd) {
  const probe = spawn(cmd, ["version"], { shell: true, stdio: "ignore" });
  return new Promise((resolve) => {
    probe.on("error", () => resolve(false));
    probe.on("exit", (code) => resolve(code === 0));
  });
}

function portFree(port) {
  return new Promise((resolve) => {
    const s = createServer();
    s.once("error", () => resolve(false));
    s.once("listening", () => s.close(() => resolve(true)));
    s.listen(port, "127.0.0.1");
  });
}

// agentSdkVersion reads the pinned Agent SDK version out of the runtime image's
// Dockerfile, which ADR-023 決策 1 makes the only place that version is written
// down. Returns null rather than guessing: a wrong version in the hint is worse
// than no version, because it produces a clean mode that runs a different
// runtime than the image it claims to rehearse.
function agentSdkVersion(dockerfile) {
  try {
    const m = readFileSync(dockerfile, "utf8").match(
      /^ARG\s+CLAUDE_AGENT_SDK_VERSION\s*=\s*"?([^"\s]+)"?\s*$/m,
    );
    return m ? m[1] : null;
  } catch {
    return null;
  }
}

// --- deployment preconditions (02:PORT-005, 04 丙-102) -----------------------
//
// 02:PORT-005 asks every preflight failure to name what is missing and how to
// get it. Until now this preflight checked node, go, the carrier's modules, the
// harness runtime, the frontend build and three ports - and not one deployment
// setting. A launch with none of them set came up looking healthy and then
// failed one feature per click: every run refused by the gateway, packaging 503,
// cross-language search empty, and the trace that would have explained any of it
// switched off.
//
// Settings split three ways, and the split is the whole design:
//
//   1. THIS SCRIPT OWNS BOTH ENDS. The value only means anything inside this
//      launch and nobody cares what it is. Asking for it is how it ends up
//      missing. sandboxToken below has always been handled this way ("nothing
//      for an operator to configure and nothing to leave behind on disk"); the
//      trace secret, the trace URL and the profile directory are the same shape
//      and were being asked for anyway.
//   2. ONLY A PERSON KNOWS IT. It points at something outside this machine.
//      Reported, with the variable and where its value comes from.
//   3. IT IS A PROMISE, NOT A PARAMETER. A default here would be this script
//      deciding something on the owner's behalf. DOWNLOAD_ARTIFACT_RETENTION is
//      the one: GOV-RETENTION-001 leaves it unset ON PURPOSE because PDM-006's
//      90 days is not ratified, and the consent form quotes whatever it says.
//      Reported with WHY it has no default - never filled in.
//
// Only one combination is refused outright, because only one is incoherent
// rather than reduced: a gateway with no SKILLHUB_RUN_MODEL. The Agent SDK then
// asks for its own default model, the gateway does not serve that name, and
// every run dies on `400 Invalid model name`. Everything else produces a
// deployment that is smaller than the demo and honest about it - a run with no
// model exit is refused by the platform with a reason, packaging with no
// retention answers 503, search with no embedding service says it is degraded.
//
// The list this paragraph described was the launcher's own, and it is gone:
// 05 R-36 第二段 landed on 2026-09-01 and reportCapabilities() now asks the
// platform (GET /readyz) instead of keeping a second copy. What survives here
// is the settings SPLIT above — which category a value falls into is still this
// script's decision, because it is the one minting and passing them.

// ownedSettings are category 1: derived here, never asked for. An operator who
// set one anyway keeps their value - this fills gaps, it does not override.
function ownedSettings() {
  return {
    // Minted per launch like sandboxToken, and for the same reason: the API
    // both signs and verifies these tokens, so the value never leaves this
    // process tree and there is nothing to coordinate.
    SKILLHUB_TRACE_INGEST_SECRET: randomUUID(),
    // The address the sandbox posts trace events back to. This script already
    // computed it for APP_URL; not passing it here is what left every failed
    // run saying only "workload exited with code 1".
    SKILLHUB_TRACE_INGEST_URL: `http://127.0.0.1:${API_PORT}`,
    // The platform's default for this is repo-root relative while the API runs
    // with cwd apps/platform, so the default resolves to a directory that does
    // not exist and packaging reports itself unconfigured. This script knows
    // the repo root.
    PACKAGING_PROFILES_DIR: join(
      repoRoot,
      "contracts",
      "packaging",
      "profiles",
    ),
    // 05 R-37 (c) / ADR-061: where the operator writes the Skill Versions they
    // have deliberately released to run with no isolation boundary.
    //
    // Filled in here, and seeded with its own instructions below, because a
    // switch nobody can find is the same as no switch — and this one cannot be
    // named at launch: the version being released does not exist until somebody
    // has uploaded it, which on this machine happens after all three processes
    // are already up.
    //
    // Outside the repo (a demo must not leave a file in `git status`) and
    // outside the carrier (that database is in-memory and gone at shutdown, so
    // a release recorded in it would have to be re-typed every launch).
    SKILLHUB_CLEAN_MODE_RELEASES: RELEASES_FILE,
  };
}

// RELEASES_FILE is stable across launches on purpose: the ids in it are not,
// so a line left over from a previous demo simply never matches anything — the
// wrong direction to fail is the safe one here.
const RELEASES_FILE = join(tmpdir(), "skillhub-clean-mode-releases.txt");

// seedReleaseFile writes the empty list once, with the format and the risk in
// it. The header is not decoration: this file is the whole of the audit trail
// for a decision that turns a protection off (the log line at use is the other
// half, and it is in this launcher's own terminal), so the person editing it
// should be reading what they are accepting while they type the reason.
function seedReleaseFile() {
  if (existsSync(RELEASES_FILE)) return;
  writeFileSync(
    RELEASES_FILE,
    [
      "# Clean test mode — versions released to run WITHOUT ANY ISOLATION (05 R-37, ADR-061).",
      "#",
      "# One release per line:   <skill_version_id> <why you are allowing it>",
      "#",
      "# The reason is required. A line with an id and nothing after it is not a",
      "# release, because the reason is the only thing this switch actually records.",
      "#",
      "# What you are accepting: this mode runs workloads as a plain process on this",
      "# machine, as you. Releasing a version means somebody else's code runs with",
      "# your account's reach. Release only content you have read.",
      "#",
      "# Takes effect on the next run — no restart. Delete a line to withdraw it.",
      "",
    ].join("\n"),
    "utf8",
  );
}

function applyOwnedSettings() {
  const filled = [];
  for (const [name, value] of Object.entries(ownedSettings())) {
    // deployment(), not process.env: this block's own contract is "fills gaps,
    // it does not override", and a value an operator wrote in .env is one they
    // supplied. Reading only process.env would mint over it and the launch
    // would then run on a secret nobody chose.
    if (!deployment(name)) {
      process.env[name] = value;
      filled.push(name);
    }
  }
  return filled;
}

// gatewayModels reads the model names the gateway actually serves, so the
// refusal below can list them instead of telling somebody to go and look. Same
// rule as agentSdkVersion: read the one file that defines them, never a second
// copy that can drift.
function gatewayModels() {
  try {
    const text = readFileSync(
      join(repoRoot, "infra", "compose", "litellm-config.yaml"),
      "utf8",
    );
    return [...text.matchAll(/^\s*-\s*model_name:\s*(\S+)/gm)].map((m) => m[1]);
  } catch {
    return [];
  }
}

// The capability table used to live here, as a list of variables this script
// checked with `!process.env[n]`. It is now the platform's (05 R-36 第二段,
// apps/platform/cmd/api/capabilities.go) and this asks for it.
//
// R-36's hard condition is why the old list is gone rather than kept in sync:
// two lists of the same preconditions is the drift this repository keeps
// finding. And this script's copy was the one that could not do better than
// guess — it owns no process it could ask, so "the variable is set" was the
// most it could ever say.
//
// What that cost, measured 2026-09-01 (04 丙-118): LLM_SERVICE_URL pointing at a
// service that had been restarted without its credential still printed
// ✓ 跨語言意圖搜尋 here, while that service answered 503 on every capability
// endpoint. The platform's table probes it instead, with this deployment's own
// token, so the same state now prints ✗ with the reason.
const READINESS_LABEL = {
  ready: "✓ 量到了，可以用",
  // Printed differently from ✓ on purpose: this is the state every green tick
  // the old list printed was really in.
  unmeasured: "? 前提齊全，但沒有人量過它",
  unavailable: "✗ 缺前提",
  broken: "✗ 前提齊全，但量到它壞的",
};

async function reportCapabilities(filled) {
  console.log(
    `[launcher] 這次啟動自己補上的設定（不必也不該由人提供）：${filled.join("、") || "無"}`,
  );
  // Names, never values: .env holds secrets and this print is a terminal and a
  // log. Printed even when empty, because "nothing was picked up" is the answer
  // somebody debugging a setting that did nothing needs to see.
  const fromFile = Object.keys(dotEnvForApi).sort();
  console.log(
    `[launcher] 從 repo 的 .env 讀進來、交給 API 的變數（只列名字）：${fromFile.join("、") || "無"}`,
  );
  let body;
  try {
    const response = await fetch(`http://127.0.0.1:${API_PORT}/readyz`, {
      signal: AbortSignal.timeout(15000),
    });
    if (!response.ok) throw new Error(`GET /readyz -> ${response.status}`);
    body = await response.json();
  } catch (error) {
    // Not fatal and not silent. The processes are up; what failed is the report
    // about them, and saying so beats printing a table this script made up.
    console.log(
      `[launcher] 問不到平台的能力表（${error.message}）。三個行程都起來了，` +
        `但「這個部署現在有什麼」這一題現在沒有答案——直接開 http://127.0.0.1:${API_PORT}/readyz 再試一次。`,
    );
    return;
  }
  console.log(
    `[launcher] 這個部署現在有什麼、缺什麼（平台的答案，GET /readyz 是同一張表）：`,
  );
  for (const c of body.capabilities ?? []) {
    console.log(`[launcher]   ${READINESS_LABEL[c.readiness] ?? c.readiness} ${c.name}`);
    if (c.detail) console.log(`[launcher]       ${c.detail}`);
    if (c.missing?.length) console.log(`[launcher]       缺 ${c.missing.join("、")}`);
    if (c.readiness !== "ready" && c.without) {
      console.log(`[launcher]       沒有它會怎樣：${c.without}`);
    }
    if (c.readiness === "unavailable" && c.fix) {
      console.log(`[launcher]       怎麼補：${c.fix}`);
    }
  }
  if (!body.ready) {
    console.log(
      `[launcher] 上面不是每一列都量到可以用。這不一定是壞掉——一個比較小的部署也長這樣——` +
        `但「設定齊全」和「它會動」是兩件事，只有 ✓ 那一行是量出來的。`,
    );
  }
}

async function preflight() {
  const [major] = process.versions.node.split(".").map(Number);
  if (major < 20) {
    fail(
      `node ${process.versions.node} is too old`,
      "this mode needs Node 20 or newer",
    );
  }
  if (!(await has("go"))) {
    fail(
      "the go toolchain is not on PATH",
      "the API and the sandbox daemon are Go programs and this script builds them here rather than shipping a binary; install Go or run this on a machine that has it",
    );
  }
  const carrierDeps = join(repoRoot, "tools", "pglite", "node_modules");
  if (!existsSync(carrierDeps)) {
    fail(
      `the database carrier's dependencies are not installed (${carrierDeps} does not exist)`,
      "with a registry: `npm ci --prefix tools/pglite`. Without one: build the bundle on a machine that has a registry (`node tools/cleanmode/bundle.mjs <dir>`), copy that directory here, and run `npm ci --offline --cache <dir> --prefix tools/pglite`",
    );
  }
  // The harness's own runtime. This is the check whose absence made every other
  // check pointless: the mode started, accepted a run, dispatched it and failed
  // it twelve seconds later with `Cannot find package
  // '@anthropic-ai/claude-agent-sdk'` — a fact this process could have known
  // before it printed its first line.
  //
  // run.mjs is COPY'd into the runtime image, where the Dockerfile npm-installs
  // the SDK beside it at build time. Clean mode runs that same script from the
  // repo (ADR-060: the same workload, a different driver), and the repo carries
  // no package.json and no node_modules there, so the import cannot resolve on
  // any machine, ever. 02:PORT-005 asks every preflight failure to name what is
  // missing and how to get it; this one had no check at all, and the operator
  // met it as a failed Run instead.
  //
  // The version comes from the Dockerfile's ARG rather than from a constant
  // here. ADR-023 決策 1 makes that the version's single source of truth, and a
  // second copy in this file is exactly how a clean-mode run would silently
  // stop being a rehearsal of the image.
  const harnessDir = join(repoRoot, "infra", "images", "runtime-agent-sdk");
  const sdkDir = join(
    harnessDir,
    "node_modules",
    "@anthropic-ai",
    "claude-agent-sdk",
  );
  if (!existsSync(sdkDir)) {
    const version = agentSdkVersion(join(harnessDir, "Dockerfile"));
    fail(
      `the run harness's Agent SDK is not installed (${sdkDir} does not exist), so every Run would fail with "Cannot find package '@anthropic-ai/claude-agent-sdk'"`,
      version
        ? `run \`npm install --no-save --prefix infra/images/runtime-agent-sdk @anthropic-ai/claude-agent-sdk@${version}\` — the version is the one the runtime image pins (ARG CLAUDE_AGENT_SDK_VERSION), and a different one is a different runtime than the image being rehearsed`
        : "install @anthropic-ai/claude-agent-sdk under infra/images/runtime-agent-sdk at the version the Dockerfile's ARG CLAUDE_AGENT_SDK_VERSION pins",
    );
  }

  const dist = join(repoRoot, "apps", "web", "dist", "index.html");
  if (!existsSync(dist)) {
    fail(
      `the frontend is not built (${dist} does not exist)`,
      "run `npm --prefix apps/web run build`; clean mode serves this build itself so the disclosure reaches a visitor who has not logged in",
    );
  }
  for (const [name, port] of [
    ["the API", API_PORT],
    ["the sandbox daemon", SANDBOX_PORT],
    ["the database carrier", PGLITE_PORT],
  ]) {
    if (!(await portFree(port))) {
      fail(
        `port ${port} is already in use, and ${name} needs it`,
        `stop whatever holds it, or set ${name === "the API" ? "CLEAN_MODE_API_PORT" : name === "the sandbox daemon" ? "CLEAN_MODE_SANDBOX_PORT" : "CLEAN_MODE_PGLITE_PORT"}`,
      );
    }
  }

  // The only combination refused rather than reported: a model exit with no
  // model named. The SDK falls back to its own default, the gateway does not
  // serve that name, and every run dies on `400 Invalid model name passed in
  // model=...` after about a minute of looking like it is working (04 丙-102 ①).
  // Measured 2026-08-30; naming the model turned the same run green.
  if (deployment("SKILLHUB_MODEL_GATEWAY_URL") && !deployment("SKILLHUB_RUN_MODEL")) {
    const models = gatewayModels();
    fail(
      "a model gateway is configured but SKILLHUB_RUN_MODEL is not, so every run would be refused by the gateway with `400 Invalid model name` about a minute after it starts",
      models.length
        ? `set SKILLHUB_RUN_MODEL to one of the names that gateway config serves: ${models.join(", ")}`
        : "set SKILLHUB_RUN_MODEL to a model name your gateway serves (the run tier is the mini one, PDM-003 v5)",
    );
  }
}

// grantCatalogWorkspace writes the three rows Service.signup would have written
// on the seed importer's first dev login, with workspaces.is_catalog already
// true — so that the skills `devctl seed-clean` uploads land somewhere
// GET /api/skills/search can see them.
//
// Why this is needed: the public catalog search joins `workspaces.is_catalog`
// (db/queries/search.sql), and nothing in the public API sets that flag — it is
// set by SQL when a catalog is built (tools/content/seed_testcases.py's
// docstring says the same). 04 丙-84 ① recorded what happens without it: fifty
// skills in the database and `total: 0` on the screen the demo is about.
//
// Why HERE and not in the seeder, which is the natural home for it: the carrier
// serves exactly one client. maxConnections=1 is ADR-060 decision 2 (opening the
// multiplexer makes pg_try_advisory_lock lie), Node's net.Server enforces it by
// dropping the second socket outright, and cmd/api pins pgxpool to MaxConns=1 to
// match. Once the API starts it holds that one connection for the rest of the
// run, so `devctl seed-clean` cannot open a second one — this window, after the
// carrier reports ready and before the API is spawned, is the only moment any
// SQL can run at all.
//
// Why it is not the second data layer 02:PORT-008 forbids: it defines no schema
// and implements none of db/gen's query methods. It is one statement, it names
// the rows it writes, and if it fails the launch fails with it.
async function grantCatalogWorkspace(dsn) {
  // `pg` lives in the carrier's node_modules, which preflight has already
  // checked for; resolve it from there rather than from this directory.
  const requirePglite = createRequire(
    join(repoRoot, "tools", "pglite", "package.json"),
  );
  const { Client } = requirePglite("pg");
  const client = new Client({ connectionString: dsn });
  await client.connect();
  try {
    // The field values are exactly what devLogin builds for this name
    // (internal/creator/workspace/http.go), so LoginOrSignup finds this identity
    // and reuses the account instead of creating a private one beside it.
    const { rows } = await client.query(
      `WITH u AS (
         INSERT INTO users (email, display_name) VALUES ($1 || '@dev.local', $1)
         RETURNING id
       ), w AS (
         INSERT INTO workspaces (owner_user_id, name, is_catalog)
         SELECT id, $1, true FROM u
         RETURNING id
       ), i AS (
         INSERT INTO user_identities (user_id, provider, provider_user_id)
         SELECT id, 'dev', $1 FROM u
         RETURNING user_id
       )
       SELECT (SELECT id FROM w) AS workspace_id, (SELECT user_id FROM i) AS user_id`,
      [SEED_IMPORTER],
    );
    return { workspaceId: rows[0].workspace_id, userId: rows[0].user_id };
  } finally {
    // end() sends the wire-protocol Terminate. A socket dropped mid-protocol
    // instead leaves the carrier refusing every later connection (the known
    // limitation in tools/pglite/bin/serve.mjs), which would take the API down
    // with it before it ever started.
    await client.end();
  }
}

// quoteForShell exists because the machine this mode targets is Windows, where
// the node binary lives under "C:\Program Files" and spawn with shell:true
// concatenates rather than escapes - the first attempt to launch died on
// 'C:\Program' is not recognized. The shell is still needed so `go` resolves
// through PATH without hard-coding an extension.
function quoteForShell(value) {
  return /\s/.test(value) ? `"${value}"` : value;
}

function start(label, cmd, args, opts = {}) {
  const child = spawn(quoteForShell(cmd), args.map(quoteForShell), {
    cwd: repoRoot,
    shell: true,
    // detached on POSIX so shutdown has a process group to signal; on Windows
    // the tree is reached with taskkill /T instead (see killTree).
    detached: process.platform !== "win32",
    env: { ...process.env, ...(opts.env ?? {}) },
    stdio: ["ignore", "pipe", "pipe"],
  });
  children.push({ label, child });
  const echo = (stream, prefix) =>
    stream.on("data", (b) => {
      for (const line of String(b).split(/\r?\n/)) {
        if (line.trim()) console.log(`[${prefix}] ${line}`);
      }
    });
  echo(child.stdout, label);
  echo(child.stderr, label);
  child.on("exit", (code) => {
    if (shuttingDown) return;
    console.error(
      `\n[${label}] exited with code ${code}; shutting the rest down`,
    );
    shutdown(code ?? 1);
  });
  return child;
}

// waitFor resolves on a line the child prints, so the next process starts when
// the previous one is actually ready rather than after a sleep that is wrong on
// a slower machine. The carrier is the only one with a real handshake, and it
// is also the only one the others cannot start without.
function waitFor(child, pattern, timeoutMs, whatWasWaitedFor) {
  return new Promise((resolve, reject) => {
    let seen = "";
    const timer = setTimeout(
      () =>
        reject(
          new Error(
            `${whatWasWaitedFor} within ${timeoutMs}ms; last output was: ${seen.slice(-300) || "(nothing)"}`,
          ),
        ),
      timeoutMs,
    );
    const onData = (b) => {
      seen += String(b);
      const match = seen.match(pattern);
      if (match) {
        clearTimeout(timer);
        child.stdout.off("data", onData);
        resolve(match);
      }
    };
    child.stdout.on("data", onData);
  });
}

// killTree ends the whole tree, not just the child this script holds a handle
// to. On Windows `shell: true` means that handle is a cmd.exe wrapper, and
// killing it leaves the node or go process it launched running - measured here
// the hard way: a previous run left its database carrier holding port 15433,
// and the next launch refused to start because of it. That is the same leak the
// clean-mode driver exists to prevent (report-local-driver.md section 2); a
// launcher that leaks its own children has no business starting a driver that
// does not.
function killTree({ label, child }) {
  if (child.pid === undefined) return;
  try {
    if (process.platform === "win32") {
      spawn("taskkill", ["/PID", String(child.pid), "/T", "/F"], {
        stdio: "ignore",
      });
    } else {
      // Negative pid signals the group; the children were started detached so
      // there is a group to signal.
      process.kill(-child.pid, "SIGTERM");
    }
  } catch {
    // Already gone. Nothing to do, and nothing worth saying during shutdown.
  }
}

function shutdown(code = 0) {
  if (shuttingDown) return;
  shuttingDown = true;
  for (const entry of children.reverse()) killTree(entry);
  setTimeout(() => process.exit(code), 1500);
}

process.on("SIGINT", () => shutdown(0));
process.on("SIGTERM", () => shutdown(0));

const filledSettings = applyOwnedSettings();
seedReleaseFile();
await preflight();

const carrier = start("pglite", process.execPath, [
  join(repoRoot, "tools", "pglite", "bin", "serve.mjs"),
  `--port=${PGLITE_PORT}`,
]);
let dsn;
try {
  const match = await waitFor(
    carrier,
    /PGLITE_READY (\S+)/,
    120_000,
    "the database carrier never reported ready",
  );
  dsn = match[1];
} catch (err) {
  console.error(`\nclean mode cannot start: ${err.message}`);
  shutdown(1);
  throw err;
}
console.log(`[launcher] database carrier ready on ${PGLITE_PORT}`);

// Fail-closed: a launch that skipped this would come up looking healthy and
// then seed fifty skills nobody can find, which is the failure 04 丙-84 ① is.
let seedImporter;
try {
  seedImporter = await grantCatalogWorkspace(dsn);
} catch (err) {
  console.error(
    `\nclean mode cannot start: the demo importer's catalog workspace could not be created: ${err.message}`,
  );
  console.error(
    "  if `pg` is missing, run `npm ci --prefix tools/pglite` (it is a devDependency there and `--omit=dev` skips it)",
  );
  shutdown(1);
  throw err;
}
console.log(
  `[launcher] demo importer "${SEED_IMPORTER}" owns workspace ${seedImporter.workspaceId}, workspaces.is_catalog = true`,
);
console.log(
  `[launcher]   this is what makes \`devctl seed-clean\`'s uploads visible to GET /api/skills/search (04 丙-84 ①)`,
);

// The operator roster, for the same reason the workspace flag above is set here
// and not left to somebody: OPERATOR_USER_IDS is read by cmd/api at start-up, and
// the id it needs does not exist until the statement above has run. The carrier
// is in-memory (tools/pglite/lib/harness.mjs constructs PGlite with no dataDir),
// so it cannot be looked up in one launch and supplied to the next either — the
// database that held it is gone. Without this, no account on a clean-mode
// deployment can ever reach an operator route, and 02:PACK-001's last step stays
// unreachable: every seeded skill is `redistribution = unknown`, packaging
// answers 422 for all fifty, and the only endpoint that can change that is
// behind RequireOperator (04 丙-105).
//
// Category 1 like the rest: this script creates the account AND spawns the
// process that reads the roster, so there is nothing here for an operator to
// configure. It fills a gap and never overrides — a launch that named its own
// roster keeps it.
//
// Safe only because of what clean mode already is: DEV_LOGIN is on, so anybody
// can already sign in as anybody (the launcher says so, loudly). This grants no
// authority that mode did not already hand out. It would be wrong in any
// deployment where DEV_LOGIN is off, and this file only ever runs with it on.
// deployment(), not process.env: a roster set in .env is one somebody wrote
// down, and overwriting it with this launch's seed importer would silently
// unmake their choice.
if (!deployment("OPERATOR_USER_IDS")) {
  process.env.OPERATOR_USER_IDS = seedImporter.userId;
  console.log(
    `[launcher]   and is this launch's operator, so 可散布性 can be established at all (04 丙-105)`,
  );
}

// DEV_LOGIN is set because clean mode has no GitHub OAuth to reach, and
// COOKIE_INSECURE because it is plain http on loopback. Both are what
// .env.example already documents for a local run; neither is a second switch
// for this mode.
// The bearer the platform uses to call the sandbox daemon is generated per
// launch rather than read from anywhere. It is a shared secret between two
// processes this script owns for the length of one run, so there is nothing for
// an operator to configure and nothing to leave behind on disk - which also
// means a demo cannot accidentally ship with a token someone else knows.
const sandboxToken = randomUUID();

// DATABASE_URL is deliberately not in `shared`: sandboxd holds no database
// connection and needs none (apps/sandbox/cmd/sandboxd/main.go), and the
// driver's workload env is an allowlist precisely so a DSN never reaches a
// skill. Handing it to sandboxd anyway would put the control plane's key one
// hop from the workload for no reason.
const shared = {
  SKILLHUB_CLEAN_MODE: "1",
  DEV_LOGIN: "1",
  COOKIE_INSECURE: "1",
  APP_URL: `http://127.0.0.1:${API_PORT}`,
  API_ADDR: `127.0.0.1:${API_PORT}`,
};

// dotEnvForApi first, so everything after it wins: the shell already won by
// being filtered out of that object, and what follows is what this launcher
// owns and acts on. DATABASE_URL is the sharpest case — .env names the compose
// Postgres and this launch runs on the carrier it just started, so the line
// below must be the one that survives.
start("api", "go", ["-C", "apps/platform", "run", "./cmd/api"], {
  env: {
    ...dotEnvForApi,
    ...shared,
    DATABASE_URL: dsn,
    SKILLHUB_SANDBOX_PROVIDERS: `self_hosted=http://127.0.0.1:${SANDBOX_PORT}`,
    SKILLHUB_SANDBOX_TOKEN_SELF_HOSTED: sandboxToken,
  },
});
start("sandboxd", "go", ["-C", "apps/sandbox", "run", "./cmd/sandboxd"], {
  env: {
    ...shared,
    SKILLHUB_SANDBOX_ADDR: `127.0.0.1:${SANDBOX_PORT}`,
    SKILLHUB_SANDBOX_TOKEN: sandboxToken,
  },
});

console.log(`\n[launcher] clean test mode is starting.`);
console.log(`[launcher]   open http://127.0.0.1:${API_PORT}/`);
console.log(
  `[launcher]   the page says what this mode is not: no isolation, no signature checks, one connection.`,
);
console.log(
  `[launcher]   only curated material runs here (02:PORT-010). To run something else,`,
);
console.log(
  `[launcher]   add \`<skill_version_id> <why>\` to ${RELEASES_FILE} — the refusal names the id.`,
);
console.log(`[launcher] ctrl-c stops all three.\n`);

// Last, so it is the block still on screen when somebody starts clicking, and
// after the API is up because it is the API that answers it now.
await reportCapabilities(filledSettings);
