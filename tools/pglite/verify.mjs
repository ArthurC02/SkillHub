#!/usr/bin/env node

import pg from "pg";
import { startHarness, DISALLOWED_MULTIPLEXER_MAX_CONNECTIONS } from "./lib/harness.mjs";

const { Client } = pg;

const mutateArg = process.argv.find((a) => a.startsWith("--mutate="));
const mutate = mutateArg ? mutateArg.split("=")[1] : null;

const results = [];
function report(name, required, pass, detail) {
  results.push({ name, required, pass, detail });
  console.log(`${pass ? "PASS" : "FAIL"} [${required ? "required" : "informational"}] ${name} -- ${detail}`);
}

const maxConnections = mutate === "multiplexer" ? DISALLOWED_MULTIPLEXER_MAX_CONNECTIONS : 1;
if (mutate === "multiplexer") {
  console.log(`>>> MUTATION multiplexer: maxConnections forced to ${maxConnections} (disallowed by ADR-060 decision 2)`);
}

if (mutate === "no-prune") {
  console.log(">>> MUTATION no-prune: a handler whose socket errored stays counted against maxConnections");
}

const harness = await startHarness({ maxConnections, forgetDetachedHandlers: mutate !== "no-prune" });

{
  const { applied, failed } = harness.migrationResult;
  if (failed) {
    report(
      "all db/migrations/*.sql apply cleanly",
      true,
      false,
      `stopped at ${failed.name} after ${applied.length} applied: ${failed.error}`,
    );
  } else {
    report("all db/migrations/*.sql apply cleanly", true, true, `${applied.length}/${applied.length} applied`);
  }
}

const client = new Client({ connectionString: harness.connectionString });
await client.connect();

async function checkImmutability() {
  let targetId = null;
  if (mutate !== "missing-row") {
    const seed = await client.query(`
      WITH u AS (
        INSERT INTO users (email, display_name) VALUES ('port001@example.test', 'PORT-001 seed')
        RETURNING id
      ), w AS (
        INSERT INTO workspaces (owner_user_id, name) SELECT id, 'port-001-ws' FROM u
        RETURNING id
      ), sk AS (
        INSERT INTO skills (workspace_id, name) SELECT id, 'port-001-skill' FROM w
        RETURNING id, workspace_id
      )
      INSERT INTO skill_versions (workspace_id, skill_id, version_number, content_hash, package_object_key)
      SELECT workspace_id, id, 1, 'deadbeef', 'objects/port-001-seed'
      FROM sk
      RETURNING id;
    `);
    targetId = seed.rows[0].id;
  }

  const countRes = await client.query(
    mutate === "missing-row"
      ? `SELECT count(*)::int AS n FROM skill_versions WHERE id = '00000000-0000-0000-0000-000000000000'`
      : `SELECT count(*)::int AS n FROM skill_versions WHERE id = $1`,
    mutate === "missing-row" ? [] : [targetId],
  );
  const rowCount = countRes.rows[0].n;
  // Confirms the target row exists: an UPDATE/DELETE against zero rows
  // "succeeds" without the trigger ever firing, which would be a false pass.
  console.log(`    target row count for skill_versions immutability check = ${rowCount}`);

  for (const [label, sql] of [
    ["UPDATE skill_versions is rejected with ADR-003", `UPDATE skill_versions SET content_hash = 'x' WHERE id = $1`],
    ["DELETE skill_versions is rejected with ADR-003", `DELETE FROM skill_versions WHERE id = $1`],
  ]) {
    if (rowCount === 0) {
      report(label, true, false, `target row count is 0 -- this run cannot prove anything (mutate=${mutate})`);
      continue;
    }
    try {
      await client.query(sql, [targetId]);
      report(label, true, false, "statement succeeded; the immutability trigger did not fire");
    } catch (err) {
      const msg = String(err.message ?? err);
      report(label, true, msg.includes("ADR-003"), msg);
    }
  }
}
await checkImmutability();

{
  const res = await client.query(`
    SELECT c.relkind, p.partstrat
    FROM pg_class c
    JOIN pg_partitioned_table p ON p.partrelid = c.oid
    WHERE c.relname = 'trace_events'
  `);
  const row = res.rows[0];
  const ok = row && row.relkind === "p" && row.partstrat === "r";
  report(
    "trace_events is a RANGE partitioned table",
    true,
    !!ok,
    row ? `relkind=${row.relkind} partstrat=${row.partstrat}` : "no pg_partitioned_table row found",
  );
}

{
  const res = await client.query(`SELECT ('[1,0,0]'::vector <=> '[0,1,0]'::vector) AS d`);
  const d = res.rows[0]?.d;
  report("pgvector <=> operator computes a value", true, d !== undefined && d !== null, `d = ${d}`);
}

{
  const res = await client.query(`
    SELECT attgenerated
    FROM pg_attribute
    WHERE attrelid = 'search_documents'::regclass AND attname = 'tsv'
  `);
  const row = res.rows[0];
  const ok = row && row.attgenerated === "s";
  report(
    "search_documents.tsv is a generated tsvector column",
    true,
    !!ok,
    row ? `attgenerated=${JSON.stringify(row.attgenerated)}` : "column not found",
  );
}

await client.end();

// Under maxConnections=1, the socket server itself drops a second connection
// attempt, so "the second client never got a session" is as valid a pass as
// "it got a session but not the lock" — only holding both is the failure.
async function checkAdvisoryLockExclusion() {
  const lockId = 424242;
  const a = new Client({ connectionString: harness.connectionString });
  await a.connect();
  const aPid = (await a.query("SELECT pg_backend_pid() AS pid")).rows[0].pid;
  const aGot = (await a.query("SELECT pg_try_advisory_lock($1) AS got", [lockId])).rows[0].got;
  if (!aGot) {
    report("two independent connections cannot both hold the same advisory lock", true, false, "connection A failed to acquire the lock at all -- cannot test exclusion");
    await a.end();
    return;
  }

  const b = new Client({ connectionString: harness.connectionString });
  let bConnected = true;
  let bPid = null;
  let bGot = null;
  let bError = null;
  try {
    await b.connect();
    bPid = (await b.query("SELECT pg_backend_pid() AS pid")).rows[0].pid;
    bGot = (await b.query("SELECT pg_try_advisory_lock($1) AS got", [lockId])).rows[0].got;
  } catch (err) {
    bConnected = false;
    bError = String(err.message ?? err);
  }

  if (bConnected && bPid === aPid && bGot === true) {
    report(
      "two independent connections cannot both hold the same advisory lock",
      true,
      false,
      `A and B share backend pid ${aPid} and both hold the lock -- mutual exclusion is fake (this is the multiplexer defect)`,
    );
  } else if (bConnected && bGot === true) {
    report(
      "two independent connections cannot both hold the same advisory lock",
      true,
      false,
      `B connected as a distinct backend (pid ${bPid} != A's ${aPid}) but still acquired the lock`,
    );
  } else if (bConnected) {
    report(
      "two independent connections cannot both hold the same advisory lock",
      true,
      true,
      `B connected as backend pid ${bPid} (A=${aPid}) and pg_try_advisory_lock returned ${bGot} -- real session isolation held`,
    );
  } else {
    report(
      "two independent connections cannot both hold the same advisory lock",
      true,
      true,
      `B's connection attempt was rejected by the maxConnections=${maxConnections} cap (${bError}) -- it never got a session, so it never got the lock`,
    );
  }

  if (bConnected) await b.end().catch(() => {});
  await a.query("SELECT pg_advisory_unlock($1)", [lockId]).catch(() => {});
  await a.end();
}
await checkAdvisoryLockExclusion();

// A reset is what a client sends when it abandons a connection mid-query. The
// next client retries the way a pool does; a wedged carrier refuses every try.
async function checkResetClientDoesNotWedge() {
  const name = "a client reset mid-session does not wedge the carrier";
  const victim = new Client({ connectionString: harness.connectionString });
  victim.on("error", () => {});
  await victim.connect();
  await victim.query("SELECT 1");
  victim.connection.stream.resetAndDestroy();

  let lastError = null;
  for (let attempt = 1; attempt <= 20; attempt++) {
    const next = new Client({ connectionString: harness.connectionString });
    next.on("error", () => {});
    try {
      await next.connect();
      const one = (await next.query("SELECT 1 AS one")).rows[0].one;
      await next.end();
      report(name, true, one === 1, `a new client connected on attempt ${attempt} and read ${one}`);
      return;
    } catch (err) {
      lastError = String(err.message ?? err);
      await next.end().catch(() => {});
      await new Promise((r) => setTimeout(r, 100));
    }
  }
  report(name, true, false, `20 attempts after the reset were all refused; last: ${lastError}`);
}
await checkResetClientDoesNotWedge();

await harness.stop();

const requiredFailures = results.filter((r) => r.required && !r.pass);
console.log(`\n${results.length} checks run, ${requiredFailures.length} required failure(s)`);
process.exit(requiredFailures.length > 0 ? 1 : 0);
