import { afterAll, beforeAll, expect, test } from "vitest";
import { BATCH, SKILL, SKILL_VERSIONS, VERSION } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});

afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const ADOPTED = "44444444-4444-4444-8444-444444444444";
const HELD = "33333333-3333-4333-8333-333333333333";
const CANDIDATES = `/skills/${SKILL}/candidate-versions`;

const listed = (version_id: string, version_number: number, held: boolean, promoted: boolean) => ({
  version_id,
  version_number,
  content_hash: `sha256:${version_number}`,
  created_at: `2026-11-0${version_number}T00:00:00Z`,
  held_for_evaluation: held,
  promoted,
});

const WITH_CANDIDATES = {
  versions: [
    listed(ADOPTED, 4, true, true),
    listed(HELD, 3, true, false),
    ...SKILL_VERSIONS.versions,
  ],
};

const ROUND = [
  { version_id: ADOPTED, reflection_id: "r1", batch_id: "b1", round: 1, candidate: 0 },
  { version_id: HELD, reflection_id: "r1", batch_id: "b1", round: 1, candidate: 1 },
];

function withVersions(
  page: ReturnType<typeof app>,
  versions: unknown,
  candidates?: Parameters<ReturnType<typeof app>["route"]>[1],
) {
  page.route(`**/skills/${SKILL}/versions`, () => ({ json: versions }));
  page.route(`**${CANDIDATES}`, candidates ?? (() => ({ json: { candidate_versions: ROUND } })));
}

async function openHistory(page: ReturnType<typeof app>) {
  await page.getByText("每一版與它跟上一版的差異").click();
}

function versionItem(page: ReturnType<typeof app>, number: number) {
  return page
    .getByRole("region", { name: "版本", exact: true })
    .getByRole("listitem")
    .filter({ hasText: new RegExp(`^v${number}\\s`) });
}

test("the history names the line's newest, labels a held candidate by its round, and compares with the line version before it", async () => {
  const page = app();
  withVersions(page, WITH_CANDIDATES);
  await page.goto(`/skills/${SKILL}`);
  const history = page.getByRole("region", { name: "版本", exact: true });
  await page.eventually(() => expect(history.text()).toContain("（第 1 輪候選 2，保留中）"));
  expect(history.text()).toContain("共 4 版，最新 v4");
  await openHistory(page);
  expect(versionItem(page, 4).text()).not.toContain("保留中");

  await versionItem(page, 4).getByRole("button", { name: "與上一版比較" }).click();
  await page.eventually(() =>
    expect(
      page.requests.some(
        (r) =>
          r.url.pathname === `/skills/${SKILL}/diff` &&
          r.url.searchParams.get("from") === VERSION &&
          r.url.searchParams.get("to") === ADOPTED,
      ),
    ).toBe(true),
  );
});

test("a held candidate the round read cannot name is still marked held, and is not the newest", async () => {
  const page = app();
  withVersions(
    page,
    { versions: [listed(HELD, 3, true, false), ...SKILL_VERSIONS.versions] },
    () => ({
      status: 503,
      json: { error: "暫時無法讀取" },
    }),
  );
  await page.goto(`/skills/${SKILL}`);
  const history = page.getByRole("region", { name: "版本", exact: true });
  await page.eventually(() => expect(history.text()).toContain("共 3 版"));
  await openHistory(page);
  await page.eventually(() => expect(versionItem(page, 3).text()).toContain("（保留中）"));
  expect(history.text()).toContain("共 3 版，最新 v2");
});

test("trying the skill opens its test cases on the line's newest, not on a held candidate", async () => {
  const page = app();
  withVersions(page, { versions: [listed(HELD, 3, true, false), ...SKILL_VERSIONS.versions] });
  await page.goto(`/skills/${SKILL}`);
  const trial = page.getByRole("link", { name: "此小工具的測試題" });
  await page.eventually(() => expect(trial.count()).toBe(1));
  expect(new URL(trial.attribute("href") ?? "", "http://x").searchParams.get("version")).toBe(
    VERSION,
  );
});

test("a list without a held version does not ask which round built it", async () => {
  const page = app();
  withVersions(page, SKILL_VERSIONS);
  await page.goto(`/skills/${SKILL}`);
  const history = page.getByRole("region", { name: "版本", exact: true });
  await page.eventually(() => expect(history.text()).toContain("共 2 版，最新 v2"));
  expect(page.requests.filter((r) => r.url.pathname === CANDIDATES)).toHaveLength(0);
});

test("the version picker marks the line's newest and names each held candidate", async () => {
  const page = app();
  withVersions(page, { versions: [listed(HELD, 3, true, false), ...SKILL_VERSIONS.versions] });
  await page.goto(`/skills/${SKILL}/evaluation-batches`);
  const options = () => [...document.querySelectorAll("option")].map((o) => o.textContent ?? "");
  await page.eventually(() =>
    expect(options().some((o) => o.startsWith("v3（第 1 輪候選 2，保留中）"))).toBe(true),
  );
  expect(options().filter((o) => o.includes("（最新）"))).toEqual([
    expect.stringMatching(/^v2（最新）/),
  ]);
});

function versionFacts(page: ReturnType<typeof app>) {
  return page.getByRole("heading").filter({ hasText: /^v\d+，/ });
}

test("an adopted candidate's own page is the newest and compares with the line version before it, not the held one", async () => {
  const page = app();
  withVersions(page, WITH_CANDIDATES);
  await page.goto(`/skills/${SKILL}/versions/${ADOPTED}`);
  await page.eventually(() => expect(versionFacts(page).text()).toBe("v4，最新版本"));
  await page.eventually(() =>
    expect(
      page.requests.some(
        (r) =>
          r.url.pathname === `/skills/${SKILL}/diff` &&
          r.url.searchParams.get("from") === VERSION &&
          r.url.searchParams.get("to") === ADOPTED,
      ),
    ).toBe(true),
  );
  expect(
    page.requests.some(
      (r) => r.url.pathname === `/skills/${SKILL}/diff` && r.url.searchParams.get("from") === HELD,
    ),
  ).toBe(false);
});

test("a held candidate's own page says it is held, and the line's version below it stays the newest", async () => {
  const page = app();
  withVersions(page, { versions: [listed(HELD, 3, true, false), ...SKILL_VERSIONS.versions] });
  await page.goto(`/skills/${SKILL}/versions/${HELD}`);
  await page.eventually(() => expect(versionFacts(page).text()).toBe("v3，評估中的候選，保留中"));
  await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
  await page.eventually(() => expect(versionFacts(page).text()).toBe("v2，最新版本"));
});

const NEW_BATCH = `/skills/${SKILL}/evaluation-batches?version=${VERSION}`;
const BATCH_PAGE = `/skills/${SKILL}/evaluation-batches/${BATCH}`;

const withNewerCandidate = (promoted: boolean) => ({
  versions: [listed(HELD, 3, true, promoted), ...SKILL_VERSIONS.versions],
});

test("a held candidate that was never adopted is not the newest version to re-run on", async () => {
  const page = app();
  withVersions(page, withNewerCandidate(false));
  await page.goto(BATCH_PAGE);
  const rerun = page.getByRole("link", { name: "用同樣的題目與次數重跑另一個版本" });
  await page.eventually(() => expect(rerun.count()).toBe(1));
  await page.goto(NEW_BATCH);
  await page.eventually(() =>
    expect(page.getByRole("link", { name: /^v2・每題 2 次/ }).count()).toBe(2),
  );
  expect(page.getByText(/還沒有評估批次。/).count()).toBe(0);
});

test("an adopted candidate is the newest version to re-run on", async () => {
  const page = app();
  withVersions(page, withNewerCandidate(true));
  await page.goto(BATCH_PAGE);
  const rerun = page.getByRole("link", { name: "用同樣的題目與次數重跑 v3" });
  await page.eventually(() => expect(rerun.count()).toBe(1));
});
