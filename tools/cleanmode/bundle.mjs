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
// rejected with "file URL missing drive letter".
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

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

console.log(`=== node half -> ${npmCache}`);
run("npm cache population", "npm", ["ci", "--cache", npmCache, "--no-audit", "--no-fund"], { cwd: carrier });

console.log(`\n=== go half -> ${goCache}`);
const localMod = spawnSync("go", ["env", "GOMODCACHE"], { encoding: "utf8", shell: true }).stdout.trim();
const goProxy = `${fileURL(join(localMod, "cache", "download"))},https://proxy.golang.org,direct`;
for (const m of goModules) {
  run(`go mod download (${m})`, "go", ["-C", m, "mod", "download"], {
    cwd: repoRoot,
    env: { ...process.env, GOMODCACHE: goCache, GOPROXY: goProxy, GOFLAGS: "-mod=mod" },
  });
}

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
