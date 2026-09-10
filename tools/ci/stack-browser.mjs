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
// serves it, so the form is absent by construction) and once signed out. The
// database is empty, so routes carrying a fixture id render their not-found
// state -- a real scenario, and the one an empty result set produces, so it is
// asserted rather than skipped. Seeded content is the rest of 丙-221.
import { chromium } from "playwright";
import { readFileSync } from "node:fs";

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
	[...readFileSync(`${repo}/apps/web/src/fixtures/platform.ts`, "utf8").matchAll(
		/export const (\w+)\s*=\s*"([^"]+)"/g,
	)].map((m) => [m[1], m[2]]),
);
const routes = [
	...readFileSync(`${repo}/apps/web/e2e/routes.ts`, "utf8").matchAll(
		/\["([\w-]+)",\s*[`"]([^`"]+)[`"]\]/g,
	),
].map(([, name, url]) => ({
	name,
	url: url.replace(/\$\{(\w+)\}/g, (whole, key) => fixtures[key] ?? whole),
}));
if (routes.length < 15) {
	// The regex above is the kind of thing that starts matching nothing after a
	// reformat, and a pass over zero routes is green.
	console.error(`only ${routes.length} routes parsed from e2e/routes.ts; the reader is broken`);
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
// A route that names a fixture id is pointed at a row an empty database does
// not have, so its 404 is the answer and not a defect. A route with no id in it
// has no such excuse -- which is what caught 丙-223, a 404 on /workspace/downloads.
const mayBeMissing = (url) => /[0-9a-f]{8}-[0-9a-f]{4}-/.test(url);

const browser = await chromium.launch();
let failed = false;

// One context for the anonymous pass, one for the signed-in pass. Sharing it
// would make the first 401 the reason a later page rendered differently.
const anon = await browser.newContext();
const member = await browser.newContext();
const login = await member.request.post(base + "/auth/dev/login", { data: { user: "smoke" } });
if (login.status() !== 204) {
	console.error(
		`FAIL /auth/dev/login answered ${login.status()}; DEV_LOGIN=1 and COOKIE_INSECURE=1 are required`,
	);
	await browser.close();
	process.exit(1);
}

for (const { signedIn, context } of [
	{ signedIn: true, context: member },
	{ signedIn: false, context: anon },
]) for (const route of routes) {
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
		if (status === 404 && mayBeMissing(route.url)) return;
		problems.push(`HTTP ${status} ${path}`);
	});

	const response = await page.goto(base + route.url, { waitUntil: "networkidle" });
	const status = response ? response.status() : 0;
	if (status !== 200) problems.push(`the document itself answered HTTP ${status}`);

	// The nav is rendered by the router, so its presence means React mounted and
	// the route resolved -- an empty #root with a 200 is the shape a crash takes.
	if ((await page.locator(".app-nav a").first().count()) === 0) {
		problems.push("no .app-nav link: the app did not mount");
	}

	if (problems.length > 0) {
		failed = true;
		console.error(`FAIL ${route.name} (${signedIn ? "signed in" : "signed out"})  ${route.url}`);
		for (const p of problems) console.error(`       ${p}`);
	} else {
		console.log(`ok   ${route.name} (${signedIn ? "signed in" : "signed out"})`);
	}
	await page.close();
}

await browser.close();
console.log(`${routes.length} route(s) driven against the real API, signed in and signed out`);
process.exit(failed ? 1 : 0);
