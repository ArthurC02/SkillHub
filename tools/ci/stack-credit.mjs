import { chromium } from "playwright";

const base = process.env.BASE_URL;
if (!base) {
  console.error("BASE_URL is required");
  process.exit(1);
}

const lines = [];
let failed = false;
const check = (name, ok, detail = "") => {
  lines.push(
    `${ok ? "ok  " : "FAIL"} ${name}${ok || !detail ? "" : `  | ${detail}`}`,
  );
  if (!ok) failed = true;
};

const browser = await chromium.launch();
const signIn = async (user) => {
  const context = await browser.newContext();
  const res = await context.request.post(base + "/auth/dev/login", {
    data: { user },
  });
  if (res.status() !== 204) {
    throw new Error(`dev login for ${user} answered ${res.status()}`);
  }
  return context;
};
const startSession = async (context) => {
  const res = await context.request.post(base + "/creation-sessions", {
    data: { id: crypto.randomUUID(), message: "", budget_credits: 500 },
  });
  return { status: res.status(), body: (await res.text()).slice(0, 200) };
};
const balanceOf = async (context) =>
  (await context.request.get(base + "/me/credits")).json();
const placeholder = (page) =>
  page
    .locator("textarea")
    .first()
    .getAttribute("placeholder", { timeout: 5000 })
    .catch(() => null);

try {
  const member = await signIn("smoke-credit-member");
  const me = await (await member.request.get(base + "/me")).json();

  let credits = await balanceOf(member);
  check(
    "a new account starts at 0 credits and cannot start",
    credits.balance_credits === 0 && credits.can_start === false,
    JSON.stringify(credits),
  );

  let started = await startSession(member);
  check(
    "a creation start at 0 credits is refused before any model call",
    started.status === 422 && started.body.includes("點數不足"),
    `${started.status} ${started.body}`,
  );

  const page = await member.newPage();
  const problems = [];
  page.on("pageerror", (err) => problems.push(`uncaught: ${err.message}`));
  await page.goto(base + "/workspace/creations", { waitUntil: "networkidle" });
  check(
    "the creation page states the shortfall",
    (await page.getByText(/目前 0 點/).count()) > 0,
  );
  const blocked = await placeholder(page);
  check(
    "the composer says why it cannot start",
    blocked === "餘額不足，暫時不能開始",
    String(blocked),
  );

  const selfGrant = await member.request.post(
    `${base}/admin/credits/${me.workspace_id}/grants`,
    { data: { amount_credits: 13000, reason: "self" } },
  );
  check(
    "a member cannot grant itself credits",
    selfGrant.status() === 404,
    String(selfGrant.status()),
  );

  const operator = await signIn("smoke-operator");
  const grant = await operator.request.post(
    `${base}/admin/credits/${me.workspace_id}/grants`,
    { data: { amount_credits: 13000, reason: "beta reward" } },
  );
  const granted = await grant.json().catch(() => ({}));
  check(
    "the operator grants the 13,000 beta reward",
    grant.status() === 200 && granted.balance_credits === 13000,
    `${grant.status()} ${JSON.stringify(granted)}`,
  );

  credits = await balanceOf(member);
  check(
    "the member's balance follows and it can start",
    credits.balance_credits === 13000 && credits.can_start === true,
    JSON.stringify(credits),
  );

  await page.reload({ waitUntil: "networkidle" });
  check(
    "the creation page shows the new balance",
    (await page.getByText(/餘額 13,?000 點/).count()) > 0,
  );

  started = await startSession(member);
  check(
    "with the grant, a session starts",
    started.status === 200,
    `${started.status} ${started.body}`,
  );
  check("no uncaught page errors", problems.length === 0, problems.join(" / "));
} catch (err) {
  check("the credit pass ran to the end", false, err.message);
}

await browser.close();
console.log(lines.join("\n"));
process.exit(failed ? 1 : 0);
