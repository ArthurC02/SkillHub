// Drive a real browser against a real backend: nginx serving the built SPA,
// proxying to platform-api, on a real Postgres. Started by tools/ci/stack-smoke.sh.
//
// Why this exists (04 丙-221). Every spec in apps/web/e2e calls stubPlatform, so
// the browser tier has never spoken to the API; the Go tiers never render a
// page. The seam between them was checked once, by hand, on 2026-09-06 -- and
// that single pass found two bugs neither tier can see: Go's encoding/json
// writes a nil slice as `null` and the client did `.length` on it, and a <p>
// with a <form> inside it, which only appears with real data. Its first
// automated run found a third (04 丙-223).
//
// The route list is READ FROM e2e/routes.ts, not copied. That file is already
// the one table the screenshot pass and the phone-width assertion share, and a
// second hand-maintained copy here would be a list that silently stops matching
// the router -- the failure this repository keeps finding in its own documents.
//
// What it covers and what it does not. Every route the router declares, twice:
// once behind a session taken from POST /auth/dev/login (the dev-login form is
// injected by apps/platform/cmd/api when *it* serves the bundle; here nginx
// serves it, so the form is absent by construction) and once signed out. One
// Skill is imported first through the product's own upload route, so the pages
// that list and detail a skill are driven with something on them; the ids that
// nothing seeded still render their not-found state, which is a real scenario
// and the one an empty result set produces, so it is asserted rather than
// skipped. A Test Case is seeded too (it costs nothing to create); a Run is
// not, because a Run has to reach a model — see stack-seed.mjs.
import { chromium } from "playwright";
import { readFileSync } from "node:fs";
import { seedSkill } from "./stack-seed.mjs";

const base = process.env.BASE_URL;
if (!base) {
  console.error("BASE_URL is required");
  process.exit(1);
}
const repo = process.env.REPO || "/work";

// The ids are template substitutions in routes.ts, exported by the fixture
// module the stub tier serves. Both files are read as text on purpose: this
// runs on plain node with no TypeScript and no bundler, and a regex over two
// `export const NAME = "uuid"` lines is a smaller dependency than either.
const fixtures = Object.fromEntries(
  [
    ...readFileSync(
      `${repo}/apps/web/src/fixtures/platform.ts`,
      "utf8",
    ).matchAll(/export const (\w+)\s*=\s*"([^"]+)"/g),
  ].map((m) => [m[1], m[2]]),
);
const routes = [
  ...readFileSync(`${repo}/apps/web/e2e/routes.ts`, "utf8").matchAll(
    /\["([\w-]+)",\s*[`"]([^`"]+)[`"]\]/g,
  ),
].map(([, name, url]) => ({
  name,
  url: url.replace(/\$\{(\w+)\}/g, (whole, key) => fixtures[key] ?? whole),
}));
// Kept before the seed overwrites them, so the substitution below can tell the
// seeded fixture ids apart from every other uuid in the table.
const ORIGINAL = {
  SKILL: fixtures.SKILL,
  VERSION: fixtures.VERSION,
  TEST_CASE: fixtures.TEST_CASE,
};

if (routes.length < 15) {
  // The regex above is the kind of thing that starts matching nothing after a
  // reformat, and a pass over zero routes is green.
  console.error(
    `only ${routes.length} routes parsed from e2e/routes.ts; the reader is broken`,
  );
  process.exit(1);
}

// Every route is visited twice, and there is deliberately no table saying which
// ones need a session. A prefix rule was tried first and was wrong on the first
// route that breaks the pattern: `/skills/{id}/package` is not under /workspace,
// /lab or /runs, and it calls /packaging/targets, which answers 401 to a
// stranger. Such a table would be a third copy of what the router already
// knows, kept by hand.
//
// So: signed in, every route must come back clean. Signed out, every route must
// still RENDER — being refused is the product working there, but an uncaught
// exception, a 5xx or an unmounted app is not.
// When a 404 is the answer and when it is a defect. Three cases, and the first
// run with a seed in it needed all three:
//
//   · The route names no id at all. Nothing can be "not found" here, so a 404
//     is always a defect — this is what caught 丙-223 on /workspace/downloads.
//   · The caller is signed out. The seed is imported into its owner's private
//     workspace, not the public catalogue (that needs workspaces.is_catalog),
//     so a stranger asking for it gets 404 and that is CORE-006 working.
//   · The caller is signed in. Then a route whose ids were ALL seeded must
//     resolve, or the seed is proving nothing. A route that also carries an id
//     nothing created may 404: /compare names two skills and only one exists,
//     and /lab/run carries a test_case that was never made — a Run needs a
//     model, so seeding one is out of reach here.
// Two regexes and not one, deliberately: `.test()` on a /g regex advances
// lastIndex and the next call starts from there, which is a bug this repository
// has already paid for once today. matchAll does not have that problem; the
// membership test uses a separate, non-global pattern.
const UUID_G = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g;
const HAS_UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/;

const allowed404 = (routeUrl, signedIn) => {
  if (!HAS_UUID.test(routeUrl)) return false;
  if (!signedIn) return true;
  const known = new Set(seeded ? Object.values(seeded) : []);
  return [...routeUrl.matchAll(UUID_G)]
    .map((m) => m[0])
    .some((id) => !known.has(id));
};

const browser = await chromium.launch();
let failed = false;

// One context for the anonymous pass, one for the signed-in pass. Sharing it
// would make the first 401 the reason a later page rendered differently.
const anon = await browser.newContext();
const member = await browser.newContext();
const login = await member.request.post(base + "/auth/dev/login", {
  data: { user: "smoke" },
});
if (login.status() !== 204) {
  console.error(
    `FAIL /auth/dev/login answered ${login.status()}; DEV_LOGIN=1 and COOKIE_INSECURE=1 are required`,
  );
  await browser.close();
  process.exit(1);
}

// One real Skill, imported through the product's own upload route with the
// session above. Its ids replace the fixture ids, so skill-detail, skill-files
// and packaging are driven against a row that exists instead of a 404 -- the
// states where a list has something in it are the ones an empty database
// cannot show, and they are where layout work actually lands.
let seeded = null;
try {
  seeded = await seedSkill(member.request, base);
  for (const [key, id] of Object.entries(seeded)) fixtures[key] = id;
  // Keyed by the id the table carried, so adding a seeded fixture is one entry
  // in ORIGINAL and nothing here -- the chain of ternaries this replaced would
  // have needed a branch, and a missed branch reads as "that page has no data".
  const swap = new Map(
    Object.entries(ORIGINAL).map(([key, was]) => [was, seeded[key]]),
  );
  for (const route of routes) {
    route.url = route.url.replace(
      /[0-9a-f-]{36}/g,
      (was) => swap.get(was) ?? was,
    );
  }
  console.log(
    `seeded skill ${seeded.SKILL} version ${seeded.VERSION} test case ${seeded.TEST_CASE}`,
  );
} catch (err) {
  // Not a skip. A seed that failed silently would turn every not-found page
  // green for the wrong reason.
  console.error(`FAIL seeding: ${err.message}`);
  await browser.close();
  process.exit(1);
}

for (const { signedIn, context } of [
  { signedIn: true, context: member },
  { signedIn: false, context: anon },
])
  for (const route of routes) {
    const page = await context.newPage();
    const problems = [];
    // A React crash surfaces as an uncaught exception, not as a bad status, so
    // both channels are recorded.
    page.on("pageerror", (err) => problems.push(`uncaught: ${err.message}`));
    page.on("console", (msg) => {
      if (msg.type() !== "error") return;
      // "Failed to load resource" is the browser restating a status, and it
      // names no URL -- exactly the report that is useless to whoever reads the
      // failure. The response listener below says which request it was.
      if (/Failed to load resource/.test(msg.text())) return;
      problems.push(`console.error: ${msg.text()}`);
    });
    page.on("response", (res) => {
      const status = res.status();
      if (status < 400) return;
      const path = new URL(res.url()).pathname;
      // Signed out, being refused is the answer, not a defect; that pass is
      // checking that the page survives it.
      if (!signedIn && (status === 401 || status === 403)) return;
      if (status === 404 && allowed404(route.url, signedIn)) return;
      problems.push(`HTTP ${status} ${path}`);
    });

    const response = await page.goto(base + route.url, {
      waitUntil: "networkidle",
    });
    const status = response ? response.status() : 0;
    if (status !== 200)
      problems.push(`the document itself answered HTTP ${status}`);

    // The nav is rendered by the router, so its presence means React mounted and
    // the route resolved -- an empty #root with a 200 is the shape a crash takes.
    if ((await page.locator(".app-nav a").first().count()) === 0) {
      problems.push("no .app-nav link: the app did not mount");
    }

    if (problems.length > 0) {
      failed = true;
      console.error(
        `FAIL ${route.name} (${signedIn ? "signed in" : "signed out"})  ${route.url}`,
      );
      for (const p of problems) console.error(`       ${p}`);
    } else {
      console.log(
        `ok   ${route.name} (${signedIn ? "signed in" : "signed out"})`,
      );
    }
    await page.close();
  }

await browser.close();
console.log(
  `${routes.length} route(s) driven against the real API, signed in and signed out`,
);
process.exit(failed ? 1 : 0);
