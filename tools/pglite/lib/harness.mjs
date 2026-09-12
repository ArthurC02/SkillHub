import { PGlite } from "@electric-sql/pglite";
import { vector } from "@electric-sql/pglite-pgvector";
import { PGLiteSocketServer } from "@electric-sql/pglite-socket";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { loadMigrations, applyMigrations } from "./migrations.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
export const REPO_ROOT = join(__dirname, "..", "..", "..");
export const MIGRATIONS_DIR = join(REPO_ROOT, "db", "migrations");

export const DISALLOWED_MULTIPLEXER_MAX_CONNECTIONS = 2;

// When a client's socket errors (a reset), pglite-socket 0.2.11 strips the socket's
// listeners before "close" fires, so the server keeps counting a handler with no
// socket and refuses every later connection. Drop those before it counts.
function forgetHandlersWithoutSocket(server) {
  const accept = server.handleConnection.bind(server);
  server.handleConnection = (socket) => {
    for (const handler of server.handlers) {
      if (!handler.isAttached) server.handlers.delete(handler);
    }
    return accept(socket);
  };
}

export async function startHarness(opts = {}) {
  const {
    port = 0,
    host = "127.0.0.1",
    maxConnections = 1,
    forgetDetachedHandlers = true,
  } = opts;

  const db = new PGlite({ extensions: { vector } });
  await db.waitReady;

  const migrations = loadMigrations(MIGRATIONS_DIR);
  const migrationResult = await applyMigrations(db, migrations);

  const server = new PGLiteSocketServer({ db, port, host, maxConnections });
  if (forgetDetachedHandlers) forgetHandlersWithoutSocket(server);
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
