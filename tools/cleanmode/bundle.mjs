// Builds the offline dependency bundle clean mode needs on a machine with no
// route to a package registry (02:PORT-005: "所需相依必須能離線帶上機器").
//
// Two halves, because clean mode has two toolchains: the database carrier is
// Node, and cmd/api and sandboxd are Go programs the launcher builds on the
// spot. Both are shipped as a *cache*, not as a copy of what is installed here:
// a cache restores through the lockfile, so what lands on the far machine is
// what go.sum and package-lock.json say, not whatever one developer's tree
// happened to contain.
//
// Run this where a registry is reachable, copy the directory across, and run the
// two commands this prints. It verifies both halves against its own output
// before saying it is ready, because a bundle that is missing something should
// fail here and not on the machine that cannot fix it.
//
// Size is not a detail: the Go half is ~150 MB, and 246 MB of that once
// extracted is the Go toolchain itself. go.mod asks for a newer Go than a
// machine may have installed, and Go fetches that as a module - so an offline
// machine with the wrong Go version fails before it compiles a line. That is
// measured, not assumed: against an empty bundle the build dies on "toolchain
// not available" before it reaches any dependency.
//
// None of this answers whether that machine is permitted to *run* what Go
// produces. Go writes an unsigned executable into the temp directory and
// executes it, which is environment-probe Q2. This bundle removes the network
// requirement; it does not remove the allowlist question.
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const carrier = join(repoRoot, "tools", "pglite");
const goModules = ["apps/platform", "apps/sandbox", "tools/devctl"];

const out = resolve(process.argv[2] ?? join(repoRoot, ".cleanmode-bundle"));
const npmCache = join(out, "npm");
const goCache = join(out, "go");

// A file:// proxy URL needs a drive letter on Windows - "file:///tmp/x" is
// rejected with "file URL missing drive letter", and Windows is the platform
// this mode exists for.
const fileURL = (p) => "file:///" + resolve(p).replace(/\\/g, "/").replace(/^\/+/, "");

function run(label, cmd, args, opts = {}) {
  const r = spawnSync(cmd, args, { stdio: "inherit", shell: true, ...opts });
  if (r.status !== 0) {
    console.error(`\n${label} failed`);
    process.exit(r.status ?? 1);
  }
}

function tryRun(cmd, args, opts = {}) {
  return spawnSync(cmd, args, { stdio: "inherit", shell: true, ...opts }).status === 0;
}

if (!existsSync(join(carrier, "package-lock.json"))) {
  console.error(`no lockfile at ${join(carrier, "package-lock.json")}; nothing to bundle`);
  process.exit(1);
}

// A stale cache hides a dependency added since it was built, and that failure
// lands on the machine that cannot fix it.
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

console.log(`=== node half -> ${npmCache}`);
run("npm cache population", "npm", ["ci", "--cache", npmCache, "--no-audit", "--no-fund"], { cwd: carrier });

console.log(`\n=== go half -> ${goCache}`);
// The local module cache is tried first so a rebuild costs nothing, then the
// public proxy for anything it holds only in extracted form - measured: two of
// apps/sandbox's dependencies had no zip locally.
const localMod = spawnSync("go", ["env", "GOMODCACHE"], { encoding: "utf8", shell: true }).stdout.trim();
const goProxy = `${fileURL(join(localMod, "cache", "download"))},https://proxy.golang.org,direct`;
for (const m of goModules) {
  run(`go mod download (${m})`, "go", ["-C", m, "mod", "download"], {
    cwd: repoRoot,
    env: { ...process.env, GOMODCACHE: goCache, GOPROXY: goProxy, GOFLAGS: "-mod=mod" },
  });
}

// Only cache/download is a proxy; go mod download also extracts every module
// beside it, and carrying both makes the bundle four times the size for
// nothing (634 MB against 149 MB, measured). Trimming before the verification
// is deliberate: what gets checked has to be what gets carried.
for (const entry of readdirSync(goCache)) {
  if (entry !== "cache") rmSync(join(goCache, entry), { recursive: true, force: true });
}
for (const entry of readdirSync(join(goCache, "cache"))) {
  if (entry !== "download") rmSync(join(goCache, "cache", entry), { recursive: true, force: true });
}

console.log("\n=== verifying both halves restore with no network");

rmSync(join(carrier, "node_modules"), { recursive: true, force: true });
if (!tryRun("npm", ["ci", "--offline", "--cache", npmCache, "--no-audit", "--no-fund"], { cwd: carrier })) {
  console.error("\nthe node half does not restore offline; do not carry this anywhere");
  process.exit(1);
}

// An empty module cache and a proxy with no fallback: anything the bundle is
// missing fails here rather than there.
const scratch = join(out, ".verify-modcache");
rmSync(scratch, { recursive: true, force: true });
mkdirSync(scratch, { recursive: true });
for (const m of goModules) {
  if (
    !tryRun("go", ["-C", m, "build", "./..."], {
      cwd: repoRoot,
      env: { ...process.env, GOMODCACHE: scratch, GOPROXY: fileURL(join(goCache, "cache", "download")), GOFLAGS: "-mod=mod" },
    })
  ) {
    console.error(`\nthe go half does not build ${m} offline; do not carry this anywhere`);
    process.exit(1);
  }
}
rmSync(scratch, { recursive: true, force: true });

console.log(`
bundle ready: ${out}

On the machine with no registry, from the repository root:

  1. node half
     npm ci --offline --cache "<copied>/npm" --prefix tools/pglite

  2. go half - this fills that machine's own module cache, so nothing has to
     stay set afterwards and the launcher needs to know nothing about it
     for m in apps/platform apps/sandbox tools/devctl; do
       GOPROXY="file:///<copied>/go/cache/download" go -C "$m" mod download
     done

Then: node tools/cleanmode/start.mjs

What this does not answer: whether that machine will execute what Go produces.
Go writes an unsigned executable into the temp directory and runs it - see
docs/plans/mvp/m6/environment-probe.md Q2.`);
