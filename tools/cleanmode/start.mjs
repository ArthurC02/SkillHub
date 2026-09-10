import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { childOverlay, readDotEnv, releasePath, resolve } from "./env.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");

const API_PORT = Number(process.env.CLEAN_MODE_API_PORT ?? 8080);
const SANDBOX_PORT = Number(process.env.CLEAN_MODE_SANDBOX_PORT ?? 8081);
const PGLITE_PORT = Number(process.env.CLEAN_MODE_PGLITE_PORT ?? 5433);

const dotEnv = readDotEnv(join(repoRoot, ".env"));

const deployment = (name) => resolve(dotEnv, process.env, name);

const dotEnvForApi = childOverlay(dotEnv, process.env);

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

function ownedSettings() {
  return {
    SKILLHUB_TRACE_INGEST_SECRET: randomUUID(),
    SKILLHUB_TRACE_INGEST_URL: `http://127.0.0.1:${API_PORT}`,
    PACKAGING_PROFILES_DIR: join(
      repoRoot,
      "contracts",
      "packaging",
      "profiles",
    ),
    SKILLHUB_CLEAN_MODE_RELEASES: RELEASES_FILE,
  };
}

const RELEASES_FILE = join(tmpdir(), "skillhub-clean-mode-releases.txt");

function releasesFile() {
  return releasePath(dotEnv, process.env, RELEASES_FILE);
}

function seedReleaseFile() {
  const path = releasesFile();
  if (existsSync(path)) return;
  writeFileSync(
    path,
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
    if (!deployment(name)) {
      process.env[name] = value;
      filled.push(name);
    }
  }
  return filled;
}

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

const READINESS_LABEL = {
  ready: "✓ 量到了，可以用",
  unmeasured: "? 前提齊全，但沒有人量過它",
  unavailable: "✗ 缺前提",
  broken: "✗ 前提齊全，但量到它壞的",
};

async function reportCapabilities(filled) {
  console.log(
    `[launcher] 這次啟動自己補上的設定（不必也不該由人提供）：${filled.join("、") || "無"}`,
  );
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

async function grantCatalogWorkspace(dsn) {
  const requirePglite = createRequire(
    join(repoRoot, "tools", "pglite", "package.json"),
  );
  const { Client } = requirePglite("pg");
  const client = new Client({ connectionString: dsn });
  await client.connect();
  try {
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
    await client.end();
  }
}

// spawn with shell:true on Windows concatenates rather than escapes, so a
// path with spaces (e.g. under "C:\Program Files") must be quoted here.
function quoteForShell(value) {
  return /\s/.test(value) ? `"${value}"` : value;
}

function start(label, cmd, args, opts = {}) {
  const child = spawn(quoteForShell(cmd), args.map(quoteForShell), {
    cwd: repoRoot,
    shell: true,
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

// On Windows the handle here is a cmd.exe wrapper (spawn used shell:true), so
// killing it alone leaves the real child running; taskkill /T reaches the
// whole tree. Elsewhere the negative pid signals the detached process group.
function killTree({ label, child }) {
  if (child.pid === undefined) return;
  try {
    if (process.platform === "win32") {
      spawn("taskkill", ["/PID", String(child.pid), "/T", "/F"], {
        stdio: "ignore",
      });
    } else {
      process.kill(-child.pid, "SIGTERM");
    }
  } catch {
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

if (!deployment("OPERATOR_USER_IDS")) {
  process.env.OPERATOR_USER_IDS = seedImporter.userId;
  console.log(
    `[launcher]   and is this launch's operator, so 可散布性 can be established at all (04 丙-105)`,
  );
}

const sandboxToken = randomUUID();

const shared = {
  SKILLHUB_CLEAN_MODE: "1",
  DEV_LOGIN: "1",
  COOKIE_INSECURE: "1",
  APP_URL: `http://127.0.0.1:${API_PORT}`,
  API_ADDR: `127.0.0.1:${API_PORT}`,
};

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
  `[launcher]   add \`<skill_version_id> <why>\` to ${releasesFile()} — the refusal names the id.`,
);
console.log(`[launcher] ctrl-c stops all three.\n`);

await reportCapabilities(filledSettings);
