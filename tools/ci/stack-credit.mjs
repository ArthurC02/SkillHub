import { chromium } from "playwright";
import { zipOneFile } from "./stack-seed.mjs";

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
const startKey = (page) =>
  page
    .getByRole("button", { name: "開始創作" })
    .evaluate(
      (key) => {
        const why = key.getAttribute("aria-describedby");
        const reason = (why && document.getElementById(why)?.textContent) || "";
        return {
          disabled: key.disabled,
          reason,
          timesSaid: reason
            ? document.body.innerText.split(reason).length - 1
            : 0,
          placeholder:
            document.querySelector('textarea[aria-label="想完成的任務"]')
              ?.placeholder ?? null,
        };
      },
      undefined,
      { timeout: 5000 },
    )
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
  page.on("console", (message) => {
    if (message.type() === "error") problems.push(`console: ${message.text()}`);
  });

  const importName = `browser-upload-${crypto.randomUUID()}`;
  await page.goto(base + "/workspace/import", { waitUntil: "networkidle" });
  await page
    .locator('input[name="skill-import-source"]')
    .nth(1)
    .check({ force: true });
  await page.locator("#skill-import-file").setInputFiles({
    name: `${importName}.zip`,
    mimeType: "application/zip",
    buffer: zipOneFile(
      "SKILL.md",
      `---\nname: ${importName}\ndescription: Browser-uploaded skill used to verify the platform integration.\nlicense: MIT\n---\n\n# Task\n\nReply with the requested format.\n`,
    ),
  });
  const importResponse = await Promise.all([
    page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname.includes("/skills/import/upload") &&
        response.request().method() === "POST",
    ),
    page
      .locator("#skill-import-file")
      .locator("xpath=ancestor::form")
      .locator('button[type="submit"]')
      .click(),
  ]).then(([response]) => response);
  const imported = await importResponse.json().catch(() => ({}));
  const importLink = page.locator('a[href^="/skills/"]').last();
  const versions =
    typeof imported.skill_id === "string"
      ? await (
          await member.request.get(
            `${base}/skills/${imported.skill_id}/versions`,
          )
        ).json()
      : {};
  check(
    "the browser uploads a zip and the imported version reads back",
    importResponse.status() === 201 &&
      imported.duplicate === false &&
      typeof imported.skill_id === "string" &&
      typeof imported.version_id === "string" &&
      (await importLink.getAttribute("href")) ===
        `/skills/${imported.skill_id}` &&
      versions.versions?.length === 1 &&
      versions.versions[0]?.version_id === imported.version_id &&
      versions.versions[0]?.version_number === imported.version_number,
    JSON.stringify({
      status: importResponse.status(),
      imported,
      versions,
    }).slice(0, 500),
  );

  const testCaseName = "Browser-created test case";
  const testCasePrompt = "Return the requested answer in a concise form.";
  await page.goto(base + "/lab/test-cases", { waitUntil: "networkidle" });
  if (typeof imported.skill_id === "string") {
    await page.locator("#tc-skill").selectOption(imported.skill_id);
  }
  await page.locator("#tc-name").fill(testCaseName);
  await page.locator("#tc-prompt").fill(testCasePrompt);
  const testCaseResponse = await Promise.all([
    page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === "/test-cases" &&
        response.request().method() === "POST",
    ),
    page
      .locator("#tc-name")
      .locator("xpath=ancestor::form")
      .locator('button[type="submit"]')
      .click(),
  ]).then(([response]) => response);
  const createdTestCase = await testCaseResponse.json().catch(() => ({}));
  const testCaseReadBack =
    typeof createdTestCase.test_case_id === "string"
      ? await (
          await member.request.get(
            `${base}/test-cases/${createdTestCase.test_case_id}`,
          )
        ).json()
      : {};
  check(
    "the browser creates a test case and its draft reads back",
    testCaseResponse.status() === 201 &&
      typeof imported.skill_id === "string" &&
      createdTestCase.skill_id === imported.skill_id &&
      page.url().endsWith(`/lab/test-cases/${createdTestCase.test_case_id}`) &&
      testCaseReadBack.test_case_id === createdTestCase.test_case_id &&
      testCaseReadBack.name === testCaseName &&
      testCaseReadBack.user_prompt === testCasePrompt,
    JSON.stringify({
      status: testCaseResponse.status(),
      createdTestCase,
      testCaseReadBack,
    }).slice(0, 500),
  );

  await page.goto(base + "/workspace/creations", { waitUntil: "networkidle" });
  check(
    "the creation page states the shortfall",
    (await page.getByText(/目前 0 點/).count()) > 0,
  );
  const blocked = await startKey(page);
  check(
    "the start key is disabled and says why once, outside the placeholder",
    blocked?.disabled === true &&
      /目前 0 點/.test(blocked.reason) &&
      blocked.timesSaid === 1 &&
      typeof blocked.placeholder === "string" &&
      !/餘額|點/.test(blocked.placeholder),
    JSON.stringify(blocked),
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

  const budget = page.locator('input[name="creation-budget"][value="500"]');
  await budget.click({ force: true });
  check("the browser selects a creation budget", await budget.isChecked());
  await page.getByLabel("想完成的任務").fill("用真瀏覽器送出這次創作");
  const start = page.locator("button.composer-send");
  check("the selected budget enables creation", await start.isEnabled());
  const createdRequests = [];
  const createdResponses = [];
  const isCreationStart = (message, method) =>
    new URL(message.url()).pathname.includes("/creation-sessions") &&
    method === "POST";
  page.on("request", (request) => {
    if (isCreationStart(request, request.method()))
      createdRequests.push(request.url());
  });
  page.on("response", (response) => {
    if (isCreationStart(response, response.request().method()))
      createdResponses.push(response);
  });
  await start.evaluate((button) => button.click());
  await page.waitForTimeout(500);
  const submitState = await page.locator(".composer").evaluate((composer) => ({
    message: composer.querySelector("textarea")?.value,
    disabled: composer.querySelector("button.composer-send")?.disabled,
    failure: document.querySelector(".notice-danger")?.textContent,
  }));
  const createdResponse = createdResponses.at(-1);
  const created = createdResponse
    ? await createdResponse.json().catch(() => ({}))
    : {};
  const readBack =
    typeof created.id === "string"
      ? await (
          await member.request.get(`${base}/creation-sessions/${created.id}`)
        ).json()
      : {};
  check(
    "the browser starts a creation session and its message reads back",
    createdRequests.length === 1 &&
      createdResponse?.status() === 200 &&
      readBack.id === created.id &&
      readBack.snapshot?.messages?.some(
        (message) =>
          message.role === "user" &&
          message.content === "用真瀏覽器送出這次創作",
      ),
    `${JSON.stringify({ createdRequests, status: createdResponse?.status(), readBack, submitState }).slice(0, 500)}`,
  );
  check("no uncaught page errors", problems.length === 0, problems.join(" / "));
} catch (err) {
  check("the credit pass ran to the end", false, err.message);
}

await browser.close();
console.log(lines.join("\n"));
process.exit(failed ? 1 : 0);
