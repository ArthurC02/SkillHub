import { chromium } from "playwright";
import { readFileSync } from "node:fs";
import { seedSkill } from "./stack-seed.mjs";

const base = process.env.BASE_URL;
if (!base) {
  console.error("BASE_URL is required");
  process.exit(1);
}
const repo = process.env.REPO || "/work";

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
// Captured before seeding overwrites fixtures, so the swap below can tell
// seeded ids apart from any other uuid in the route table.
const ORIGINAL = {
  SKILL: fixtures.SKILL,
  VERSION: fixtures.VERSION,
  TEST_CASE: fixtures.TEST_CASE,
};

if (routes.length < 15) {
  console.error(
    `only ${routes.length} routes parsed from e2e/routes.ts; the reader is broken`,
  );
  process.exit(1);
}

// Two regexes, not one: a /g regex's .test() advances lastIndex across calls,
// so reusing it as a membership check would silently skip matches. matchAll
// uses the global one; HAS_UUID (non-global) is only ever tested once per call.
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

// Separate browser contexts so a 401 in one pass can't affect the other.
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

let seeded = null;
try {
  seeded = await seedSkill(member.request, base);
  for (const [key, id] of Object.entries(seeded)) fixtures[key] = id;
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
    page.on("pageerror", (err) => problems.push(`uncaught: ${err.message}`));
    page.on("console", (msg) => {
      if (msg.type() !== "error") return;
      if (/Failed to load resource/.test(msg.text())) return;
      problems.push(`console.error: ${msg.text()}`);
    });
    page.on("response", (res) => {
      const status = res.status();
      if (status < 400) return;
      const path = new URL(res.url()).pathname;
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
