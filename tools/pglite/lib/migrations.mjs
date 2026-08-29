// Applies db/migrations/*.sql, unmodified, in filename order.
//
// 02:PORT-001 requires the clean test mode to run the exact same migrations
// as production, byte for byte. This file must never rewrite SQL to make it
// "PGlite-friendly" -- if a migration fails, that is a real result to report,
// not a bug in this loader to work around.

import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

/**
 * @param {string} migrationsDir absolute path to db/migrations
 * @returns {{name: string, sql: string}[]} sorted by filename
 */
export function loadMigrations(migrationsDir) {
  const files = readdirSync(migrationsDir)
    .filter((f) => f.endsWith(".sql"))
    .sort();
  if (files.length === 0) {
    throw new Error(`no .sql files found in ${migrationsDir}`);
  }
  return files.map((name) => ({
    name,
    sql: readFileSync(join(migrationsDir, name), "utf8"),
  }));
}

/**
 * Applies every migration in order against an open PGlite instance.
 * Stops at the first failure -- a later migration succeeding against a
 * broken schema proves nothing (see report-inmemory-postgres.md SQLite run).
 *
 * @param {import("@electric-sql/pglite").PGlite} db
 * @param {{name: string, sql: string}[]} migrations
 * @returns {Promise<{applied: string[], failed: {name: string, error: string} | null}>}
 */
export async function applyMigrations(db, migrations) {
  const applied = [];
  for (const migration of migrations) {
    try {
      await db.exec(migration.sql);
      applied.push(migration.name);
    } catch (err) {
      return { applied, failed: { name: migration.name, error: String(err?.message ?? err) } };
    }
  }
  return { applied, failed: null };
}
