// Builds the offline dependency bundle clean mode needs on a machine with no
// route to the npm registry (02:PORT-005: "所需相依必須能離線帶上機器").
//
// The bundle is an npm cache directory, not a copy of node_modules: a cache
// restores through `npm ci --offline`, which still honours the lockfile, so what
// lands on the far machine is what the lockfile says and not whatever happened
// to be in one developer's tree.
//
// Run this on a machine that can reach the registry, copy the directory across,
// and on the far side run the command this prints. Measured at 9.1 MB.
//
// It does not bundle the Go half. cmd/api and sandboxd are Go programs that the
// launcher builds on the spot, so an offline machine needs the Go module cache
// too - and that question is downstream of environment-probe Q2, which asks
// whether Go can run there at all. Solving the offline problem for a toolchain
// that is not allowed to execute would be work spent on the wrong side of a
// wall (see m6/environment-probe.md).
import { spawnSync } from "node:child_process";
import { existsSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const carrier = join(here, "..", "pglite");

const out = resolve(process.argv[2] ?? join(here, "..", "..", ".cleanmode-bundle"));

if (!existsSync(join(carrier, "package-lock.json"))) {
  console.error(`no lockfile at ${join(carrier, "package-lock.json")}; nothing to bundle`);
  process.exit(1);
}

// A stale cache would hide a dependency that was added since it was built, and
// the failure would land on the machine that cannot fix it.
rmSync(out, { recursive: true, force: true });

console.log(`populating ${out} from ${carrier}/package-lock.json ...`);
const populate = spawnSync("npm", ["ci", "--cache", out, "--no-audit", "--no-fund"], {
  cwd: carrier,
  stdio: "inherit",
  shell: true,
});
if (populate.status !== 0) {
  console.error("\nbundling failed; the machine building the bundle needs the registry");
  process.exit(populate.status ?? 1);
}

// Prove the bundle is sufficient here rather than discovering it is not on the
// machine with no network. --offline makes npm refuse to reach the registry, so
// a bundle missing anything fails now, with ENOTCACHED, instead of there.
console.log("\nverifying the bundle restores with no network ...");
rmSync(join(carrier, "node_modules"), { recursive: true, force: true });
const restore = spawnSync("npm", ["ci", "--offline", "--cache", out, "--no-audit", "--no-fund"], {
  cwd: carrier,
  stdio: "inherit",
  shell: true,
});
if (restore.status !== 0) {
  console.error("\nthe bundle does not restore offline; do not carry it anywhere");
  process.exit(restore.status ?? 1);
}

console.log(`
bundle ready: ${out}

On the machine with no registry:
  1. copy that directory across
  2. npm ci --offline --cache <copied-directory> --prefix tools/pglite

The Go toolchain is a separate question and deliberately not bundled here -
see the note at the top of this file and environment-probe.md Q2.`);
