import { writeFileSync } from "node:fs";
import { parseArgs } from "node:util";

const { values: opts } = parseArgs({
  options: {
    base: { type: "string" },
    rps: { type: "string", default: "100" },
    seconds: { type: "string", default: "60" },
    cookie: { type: "string" },
    search: { type: "boolean", default: false },
    out: { type: "string" },
    "max-5xx-ratio": { type: "string", default: "0.005" },
    "max-p95-ms": { type: "string", default: "500" },
    "max-in-flight": { type: "string", default: "2000" },
  },
});
if (!opts.base) {
  console.error(
    "usage: node tools/ci/load.mjs --base URL [--rps 100] [--seconds 60] [--cookie sh_session=...] [--search] [--out result.json]",
  );
  process.exit(2);
}

const base = opts.base.replace(/\/$/, "");
const rps = Number(opts.rps);
const seconds = Number(opts.seconds);
const maxInFlight = Number(opts["max-in-flight"]);
const headers = {
  Accept: "application/json",
  ...(opts.cookie ? { Cookie: opts.cookie } : {}),
};
const searchWords = ["csv", "pdf", "表格", "翻譯", "excel", "summary"];

const catalogue = await fetch(`${base}/api/skills/catalog?limit=50`, {
  headers,
}).then((r) => (r.ok ? r.json() : { results: [] }));
const skillIDs = (catalogue.results ?? [])
  .map((s) => s.skill_id)
  .filter(Boolean);
const pick = (list) => list[Math.floor(Math.random() * list.length)];

const routes = [
  { name: "catalog", weight: 40, path: () => "/api/skills/catalog" },
  skillIDs.length && {
    name: "skill-detail",
    weight: 30,
    path: () => `/api/skills/${pick(skillIDs)}`,
  },
  skillIDs.length && {
    name: "skill-files",
    weight: 10,
    path: () => `/api/skills/${pick(skillIDs)}/files`,
  },
  opts.search && {
    name: "search",
    weight: 20,
    budgeted: true,
    path: () => `/api/skills/search?q=${encodeURIComponent(pick(searchWords))}`,
  },
  opts.cookie && { name: "me", weight: 10, path: () => "/me" },
].filter(Boolean);
const totalWeight = routes.reduce((sum, r) => sum + r.weight, 0);

function chooseRoute() {
  let n = Math.random() * totalWeight;
  for (const route of routes) {
    n -= route.weight;
    if (n < 0) return route;
  }
  return routes[0];
}

const results = new Map(
  routes.map((r) => [r.name, { latencies: [], statuses: {}, failures: 0 }]),
);
let inFlight = 0;
let dropped = 0;

async function hit(route) {
  const tally = results.get(route.name);
  inFlight += 1;
  const started = performance.now();
  try {
    const response = await fetch(base + route.path(), { headers });
    await response.arrayBuffer();
    tally.latencies.push(performance.now() - started);
    tally.statuses[response.status] =
      (tally.statuses[response.status] ?? 0) + 1;
  } catch {
    tally.failures += 1;
  } finally {
    inFlight -= 1;
  }
}

const tickMs = 50;
const perTick = (rps * tickMs) / 1000;
let owed = 0;
const pending = [];
const deadline = performance.now() + seconds * 1000;
await new Promise((resolve) => {
  const timer = setInterval(() => {
    if (performance.now() >= deadline) {
      clearInterval(timer);
      resolve();
      return;
    }
    owed += perTick;
    for (; owed >= 1; owed -= 1) {
      if (inFlight >= maxInFlight) {
        dropped += 1;
        continue;
      }
      pending.push(hit(chooseRoute()));
    }
  }, tickMs);
});
await Promise.all(pending);

function percentile(sorted, p) {
  if (!sorted.length) return null;
  return Math.round(
    sorted[Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length))],
  );
}

const summary = { base, rps, seconds, dropped, routes: {} };
let requests = dropped;
let serverErrors = dropped;
let worstReadP95 = 0;
for (const route of routes) {
  const tally = results.get(route.name);
  const sorted = [...tally.latencies].sort((a, b) => a - b);
  const count = sorted.length + tally.failures;
  const fiveXX = Object.entries(tally.statuses)
    .filter(([status]) => Number(status) >= 500)
    .reduce((sum, [, n]) => sum + n, 0);
  const p95 = percentile(sorted, 95);
  summary.routes[route.name] = {
    requests: count,
    p50_ms: percentile(sorted, 50),
    p95_ms: p95,
    p99_ms: percentile(sorted, 99),
    statuses: tally.statuses,
    connection_failures: tally.failures,
    refused_429: tally.statuses[429] ?? 0,
  };
  requests += count;
  serverErrors += fiveXX + tally.failures;
  if (!route.budgeted && p95 !== null)
    worstReadP95 = Math.max(worstReadP95, p95);
}
summary.requests = requests;
summary.server_error_ratio = requests ? serverErrors / requests : 0;
summary.worst_read_p95_ms = worstReadP95;

const verdicts = [];
if (summary.server_error_ratio > Number(opts["max-5xx-ratio"])) {
  verdicts.push(
    `5xx, dropped and failed connections are ${(summary.server_error_ratio * 100).toFixed(2)}% of requests`,
  );
}
if (worstReadP95 > Number(opts["max-p95-ms"])) {
  verdicts.push(`the slowest read route has p95 ${worstReadP95} ms`);
}
summary.passed = verdicts.length === 0;
summary.failures = verdicts;

const report = JSON.stringify(summary, null, 2);
if (opts.out) writeFileSync(opts.out, report);
console.log(report);
for (const verdict of verdicts) console.log(`FAIL ${verdict}`);
process.exit(summary.passed ? 0 : 1);
