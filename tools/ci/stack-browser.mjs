// Drive a real browser against a real backend: nginx serving the built SPA,
// proxying to platform-api, on a real Postgres. Started by tools/ci/stack-smoke.sh.
//
// Why this exists (04 丙-221). Every spec in apps/web/e2e calls stubPlatform, so
// the browser tier has never spoken to the API; the Go tiers never render a
// page. The seam between them was checked once, by hand, on 2026-09-06 -- and
// that single pass found two bugs neither tier can see: Go's encoding/json
// writes a nil slice as `null` and the client did `.length` on it, and a <p>
// with a <form> inside it, which only appears with real data.
//
// What it covers and what it does not. Public routes, then the workspace
// routes behind a session, on an empty database. The session is taken by
// POSTing /auth/dev/login rather than through the login form: that form is
// injected by apps/platform/cmd/api when *it* serves the bundle, and here
// nginx serves it, so window.__SKILLHUB_DEV_LOGIN__ is absent by
// construction. Seeded content is the rest of 丙-221 -- these pages are
// checked in their empty state, which is the state the nil-slice bug lives in.
//
// An empty database is not a weak fixture for the bug class this targets: an
// empty result set is exactly the nil slice that serialises to `null`.
import { chromium } from "playwright";

const base = process.env.BASE_URL;
if (!base) {
	console.error("BASE_URL is required");
	process.exit(1);
}

// Public, unauthenticated, and each one reads the API: the home page browses
// the catalogue, /policy reads the retention policy the deployment reports.
const routes = ["/", "/policy"];
// Behind a session. These are where a logged-in person spends the visit, and
// where every list-shaped response the client indexes into is rendered.
const signedIn = ["/workspace/skills", "/workspace/runs", "/workspace/downloads", "/workspace/account"];

const browser = await chromium.launch();
let failed = false;

// One context for the anonymous pass, one for the signed-in pass. Sharing it
// would make the first 401 the reason a later page rendered differently.
const anon = await browser.newContext();
const member = await browser.newContext();
const res = await member.request.post(base + "/auth/dev/login", { data: { user: "smoke" } });
if (res.status() !== 204) {
	console.error(`FAIL /auth/dev/login answered ${res.status()}; DEV_LOGIN=1 and COOKIE_INSECURE=1 are required`);
	await browser.close();
	process.exit(1);
}

for (const [context, route] of [
	...routes.map((r) => [anon, r]),
	...signedIn.map((r) => [member, r]),
]) {
	const page = await context.newPage();
	const problems = [];
	// A React crash surfaces as an uncaught exception, not as a bad status, so
	// both channels are recorded. console.warn is not collected: React's dev
	// warnings are not what this is looking for, and the bundle is a production
	// build anyway.
	page.on("pageerror", (err) => problems.push(`uncaught: ${err.message}`));
	page.on("console", (msg) => {
		if (msg.type() !== "error") return;
		// "Failed to load resource" is the browser restating a status it already
		// gave us, and it names no URL -- which is exactly the report that is
		// useless to whoever reads the failure. The response listener below says
		// which request it was.
		if (/Failed to load resource/.test(msg.text())) return;
		problems.push(`console.error: ${msg.text()}`);
	});
	page.on("response", (res) => {
		if (res.status() < 400) return;
		const path = new URL(res.url()).pathname;
		// A logged-out visitor asking `GET /me` is answered 401 by design, and the
		// anonymous pass is deliberately in that state.
		if (res.status() === 401 && path === "/me") return;
		problems.push(`HTTP ${res.status()} ${path}`);
	});

	const response = await page.goto(base + route, { waitUntil: "networkidle" });
	const status = response ? response.status() : 0;
	if (status !== 200) problems.push(`HTTP ${status}`);

	// The nav is rendered by the router, so its presence means React mounted and
	// the route resolved -- an empty #root with a 200 is the shape a crash takes.
	const mounted = await page.locator(".app-nav a").first().count();
	if (mounted === 0) problems.push("no .app-nav link: the app did not mount");

	if (problems.length > 0) {
		failed = true;
		console.error(`FAIL ${route}`);
		for (const p of problems) console.error(`       ${p}`);
	} else {
		console.log(`ok   ${route} rendered against the real API`);
	}
	await page.close();
}

await browser.close();
process.exit(failed ? 1 : 0);
