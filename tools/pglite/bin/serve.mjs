#!/usr/bin/env node
// Starts the 02:PORT-001 PGlite carrier and keeps it running until killed.
//
// Usage:
//   node bin/serve.mjs [--port=5433] [--host=127.0.0.1]
//
// Prints a line starting with "PGLITE_READY " followed by the connection
// string once all db/migrations/*.sql have applied, then blocks. Ctrl-C
// (SIGINT) or SIGTERM closes the socket server and the database cleanly.
//
// maxConnections is deliberately not a CLI flag: ADR-060 decision 2 fixes it
// at 1 for this carrier. Anything else belongs to the mutation test, not to
// an operator's command line.
//
// KNOWN LIMITATION (matches report-inmemory-postgres.md's earlier finding):
// an abruptly terminated client -- a TCP connection dropped mid-protocol
// rather than closed with a normal wire-protocol Terminate message -- can
// leave the socket server refusing every later connection (ECONNRESET on
// the client side) even though this process is still alive. Reproduced
// locally: destroying a raw socket mid-handshake left every subsequent
// `pg` client connection failing. There is no recovery path from inside
// this script for that state; restart the harness process. Because
// maxConnections=1, a single misbehaving client is enough to take the
// whole carrier down -- callers that spawn this as a test-session fixture
// should treat any connection failure as "restart the process", not retry
// in a loop against the same instance.

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
