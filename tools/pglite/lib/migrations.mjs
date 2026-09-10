import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

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
