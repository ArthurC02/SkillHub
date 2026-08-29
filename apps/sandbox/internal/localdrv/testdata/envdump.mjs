// Writes the workload process's own environment out as an artifact, then
// speaks the same collection handshake workload.mjs does.
//
// This fixture exists because env() is only half of the question. Asserting on
// the []string env() returns would prove what this driver *intended* to pass;
// what 04 稽核 D2 was actually about is what a `node run.mjs` process can read
// back with `env` — Bash is in the harness's allowedTools, so anything in this
// dump is one tool call away from a trace event. So the assertion is made on
// the child's real environment, through cmd.Env, not on the slice.
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const outDir = process.env.SKILLHUB_OUTDIR;

const artifactDir = join(outDir, "artifacts");
mkdirSync(artifactDir, { recursive: true });
writeFileSync(join(artifactDir, "env.json"), JSON.stringify(process.env, null, 1));

writeFileSync(join(outDir, ".workload-done"), "done\n");
process.exit(0);
