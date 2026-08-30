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
import { existsSync } from "node:fs";
import { createRequire } from "node:module";
import { createServer } from "node:net";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");

const API_PORT = Number(process.env.CLEAN_MODE_API_PORT ?? 8080);
const SANDBOX_PORT = Number(process.env.CLEAN_MODE_SANDBOX_PORT ?? 8081);
const PGLITE_PORT = Number(process.env.CLEAN_MODE_PGLITE_PORT ?? 5433);

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

async function preflight() {
  const [major] = process.versions.node.split(".").map(Number);
  if (major < 20) {
    fail(`node ${process.versions.node} is too old`, "this mode needs Node 20 or newer");
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
  const requirePglite = createRequire(join(repoRoot, "tools", "pglite", "package.json"));
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
       SELECT (SELECT id FROM w) AS workspace_id`,
      [SEED_IMPORTER],
    );
    return rows[0].workspace_id;
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
    console.error(`\n[${label}] exited with code ${code}; shutting the rest down`);
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
      () => reject(new Error(`${whatWasWaitedFor} within ${timeoutMs}ms; last output was: ${seen.slice(-300) || "(nothing)"}`)),
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
      spawn("taskkill", ["/PID", String(child.pid), "/T", "/F"], { stdio: "ignore" });
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

await preflight();

const carrier = start("pglite", process.execPath, [join(repoRoot, "tools", "pglite", "bin", "serve.mjs"), `--port=${PGLITE_PORT}`]);
let dsn;
try {
  const match = await waitFor(carrier, /PGLITE_READY (\S+)/, 120_000, "the database carrier never reported ready");
  dsn = match[1];
} catch (err) {
  console.error(`\nclean mode cannot start: ${err.message}`);
  shutdown(1);
  throw err;
}
console.log(`[launcher] database carrier ready on ${PGLITE_PORT}`);

// Fail-closed: a launch that skipped this would come up looking healthy and
// then seed fifty skills nobody can find, which is the failure 04 丙-84 ① is.
let catalogWorkspaceId;
try {
  catalogWorkspaceId = await grantCatalogWorkspace(dsn);
} catch (err) {
  console.error(`\nclean mode cannot start: the demo importer's catalog workspace could not be created: ${err.message}`);
  console.error("  if `pg` is missing, run `npm ci --prefix tools/pglite` (it is a devDependency there and `--omit=dev` skips it)");
  shutdown(1);
  throw err;
}
console.log(`[launcher] demo importer "${SEED_IMPORTER}" owns workspace ${catalogWorkspaceId}, workspaces.is_catalog = true`);
console.log(`[launcher]   this is what makes \`devctl seed-clean\`'s uploads visible to GET /api/skills/search (04 丙-84 ①)`);

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

start("api", "go", ["-C", "apps/platform", "run", "./cmd/api"], {
  env: {
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
console.log(`[launcher]   the page says what this mode is not: no isolation, no signature checks, one connection.`);
console.log(`[launcher] ctrl-c stops all three.\n`);
