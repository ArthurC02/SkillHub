// The repeatable PGlite carrier for 02:PORT-001.
//
// Shape is fixed by ADR-060 decision 2, not a tuning knob:
//   - single process
//   - pool_max_conns=1 on the Go side (out of scope here; this harness just
//     has to make maxConnections=1 the default so nobody can quietly widen it)
//   - PGlite socket multiplexer OFF (maxConnections=1): opening it lets two
//     independent TCP clients share one Postgres session, which makes
//     pg_try_advisory_lock lie (report-inmemory-postgres.md §5.2). Passing a
//     larger maxConnections here is only ever done by the mutation test.
//   - River (not wired by this harness) must run poll-only against this pool.

import { PGlite } from "@electric-sql/pglite";
import { vector } from "@electric-sql/pglite/vector";
import { PGLiteSocketServer } from "@electric-sql/pglite-socket";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { loadMigrations, applyMigrations } from "./migrations.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
export const REPO_ROOT = join(__dirname, "..", "..", "..");
export const MIGRATIONS_DIR = join(REPO_ROOT, "db", "migrations");

// The forbidden configuration (ADR-060 decision 2 / 02:PORT-001 judge
// criterion 1-2). Exported only so the mutation test can name what it is
// deliberately turning on, not so callers can casually pass a bigger number.
export const DISALLOWED_MULTIPLEXER_MAX_CONNECTIONS = 2;

/**
 * @param {object} [opts]
 * @param {number} [opts.port] 0 lets the OS assign a free port (default)
 * @param {string} [opts.host]
 * @param {number} [opts.maxConnections] must stay 1 outside of the mutation
 *   test -- see DISALLOWED_MULTIPLEXER_MAX_CONNECTIONS above.
 * @returns {Promise<{
 *   db: import("@electric-sql/pglite").PGlite,
 *   server: PGLiteSocketServer,
 *   connectionString: string,
 *   migrationResult: {applied: string[], failed: {name: string, error: string} | null},
 *   stop: () => Promise<void>,
 * }>}
 */
export async function startHarness(opts = {}) {
  const { port = 0, host = "127.0.0.1", maxConnections = 1 } = opts;

  const db = new PGlite({ extensions: { vector } });
  await db.waitReady;

  const migrations = loadMigrations(MIGRATIONS_DIR);
  const migrationResult = await applyMigrations(db, migrations);

  const server = new PGLiteSocketServer({ db, port, host, maxConnections });
  await server.start();
  const address = server.getServerConn(); // "host:port" once bound
  // sslmode=disable is part of the address, not a caller's choice: this socket
  // speaks no TLS, and a client that probes for it first - pgx does - sends an
  // SSLRequest, gets a refusal it does not expect, and the disconnect that
  // follows wedges the carrier for every later connection (see the recovery
  // note in bin/serve.mjs). Handing out a DSN that cannot trigger that is
  // cheaper than documenting it.
  const connectionString = `postgres://postgres@${address}/postgres?sslmode=disable`;

  let stopped = false;
  const stop = async () => {
    if (stopped) return;
    stopped = true;
    await server.stop();
    await db.close();
  };

  return { db, server, connectionString, migrationResult, stop };
}
