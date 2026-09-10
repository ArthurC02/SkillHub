import { PGlite } from "@electric-sql/pglite";
import { vector } from "@electric-sql/pglite/vector";
import { PGLiteSocketServer } from "@electric-sql/pglite-socket";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { loadMigrations, applyMigrations } from "./migrations.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
export const REPO_ROOT = join(__dirname, "..", "..", "..");
export const MIGRATIONS_DIR = join(REPO_ROOT, "db", "migrations");

export const DISALLOWED_MULTIPLEXER_MAX_CONNECTIONS = 2;

export async function startHarness(opts = {}) {
  const { port = 0, host = "127.0.0.1", maxConnections = 1 } = opts;

  const db = new PGlite({ extensions: { vector } });
  await db.waitReady;

  const migrations = loadMigrations(MIGRATIONS_DIR);
  const migrationResult = await applyMigrations(db, migrations);

  const server = new PGLiteSocketServer({ db, port, host, maxConnections });
  await server.start();
  const address = server.getServerConn();
  // sslmode=disable is required: a client that probes for TLS first (as pgx
  // does) gets a refusal and disconnects in a way that wedges this socket.
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
