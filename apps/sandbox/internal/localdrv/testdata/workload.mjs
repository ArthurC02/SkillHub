// Minimal stand-in for run.mjs's *collection* contract (not its agent logic):
// write one trace event, write one artifact, signal DonePath, wait for
// CollectedPath, exit 0. This package's tests exercise the marker-file
// protocol dockerdrv's driver and this one both implement without needing the
// Claude Agent SDK, network access or an API key — none of which localdrv's
// own tests should depend on.
//
// sleepSync uses Atomics.wait rather than run.mjs's own
// execFileSync("/bin/sleep", ...): that absolute POSIX path does not exist on
// Windows, which is exactly the kind of host-portability gap this test fixture
// is written to not reproduce (see the report to the launching agent for why
// that matters for run.mjs itself, out of this package's scope to fix).
import { existsSync, mkdirSync, appendFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

const outDir = process.env.SKILLHUB_OUTDIR;

const traceDir = join(outDir, "trace");
mkdirSync(traceDir, { recursive: true });
appendFileSync(
  join(traceDir, "events.jsonl"),
  JSON.stringify({ type: "agent_output", run_id: process.env.SKILLHUB_RUN_ID }) + "\n",
);

const artifactDir = join(outDir, "artifacts");
mkdirSync(artifactDir, { recursive: true });
writeFileSync(join(artifactDir, "result.txt"), "hello from the workload\n");

writeFileSync(join(outDir, ".workload-done"), "done\n");

const collected = join(outDir, ".collected");
const deadline = Date.now() + 5000;
while (!existsSync(collected) && Date.now() < deadline) {
  sleepSync(50);
}

process.exit(0);
