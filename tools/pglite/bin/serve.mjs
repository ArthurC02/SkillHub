#!/usr/bin/env node

import { startHarness } from "../lib/harness.mjs";

function parseArgs(argv) {
  const opts = {};
  for (const arg of argv) {
    const [key, value] = arg.replace(/^--/, "").split("=");
    if (key === "port") opts.port = Number(value);
    if (key === "host") opts.host = value;
  }
  return opts;
}

const { port, host } = parseArgs(process.argv.slice(2));
const harness = await startHarness({ port, host });

if (harness.migrationResult.failed) {
  console.error(
    `migration failed: ${harness.migrationResult.failed.name}: ${harness.migrationResult.failed.error}`,
  );
  console.error(`applied ${harness.migrationResult.applied.length} before failure`);
  await harness.stop();
  process.exit(1);
}

console.log(`applied ${harness.migrationResult.applied.length}/${harness.migrationResult.applied.length} migrations`);
console.log(`PGLITE_READY ${harness.connectionString}`);

let shuttingDown = false;
async function shutdown(signal) {
  if (shuttingDown) return;
  shuttingDown = true;
  console.log(`\n${signal} received, closing PGlite harness`);
  await harness.stop();
  process.exit(0);
}
process.on("SIGINT", () => shutdown("SIGINT"));
process.on("SIGTERM", () => shutdown("SIGTERM"));
