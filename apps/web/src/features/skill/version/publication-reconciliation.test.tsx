import { expect, test } from "vitest";
import {
  OWN_PUBLICATION,
  OWN_PUBLICATIONS,
  PUBLISHER,
  SKILL,
  VERSION,
  skillDetail,
} from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

type Scenario = {
  action: "publish" | "delist" | "first";
  held: "own" | "overview" | "write";
  status: number;
  readFailure?: boolean;
};
type Write = { method: string; body: unknown };

const scenarios: Scenario[] = [];
for (const action of ["publish", "delist"] as const) {
  for (const held of ["own", "overview"] as const) {
    for (const status of [200, 503]) scenarios.push({ action, held, status });
  }
  scenarios.push({ action, held: "own", status: 503, readFailure: true });
}
for (const status of [201, 503]) scenarios.push({ action: "first", held: "write", status });

const NAME_NOTE = "首次發佈後，這個小工具的發佈名稱就不能更改。";

function controls(page: App) {
  return {
    section: page.getByRole("region", { name: "發佈", exact: true }),
    heading: page.getByRole("heading", { name: "發佈", exact: true }),
    attestation: page.getByRole("checkbox", { name: "我有權散布這些內容" }),
    license: page.getByRole("combobox", { name: "授權條款（選填）" }),
    nameInput: page.getByRole("textbox", { name: "發佈名稱", exact: true }),
    publish: page.getByRole("button", { name: "發佈 v2", exact: true }),
    delist: page.getByRole("button", { name: "撤回", exact: true }),
    logout: page.getByRole("button", { name: "登出", exact: true }),
  };
}

function stubPublicationOperation(page: App, scenario: Scenario) {
  page.route(`**/api/skills/${SKILL}*`, () => ({
    json: {
      ...skillDetail(SKILL, "PDF Summariser"),
      redistribution: { value: "generated", label: "平台生成", note: "" },
    },
  }));
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let written = false;
  let published = scenario.action !== "first";
  const reads = { own: 0, overview: 0 };
  const writes: Write[] = [];
  const name = scenario.action === "first" ? "chosen-name" : OWN_PUBLICATION.name;
  const changed = {
    ...OWN_PUBLICATION,
    name,
    address: `/p/${PUBLISHER}/${name}`,
    status: scenario.action === "delist" ? "delisted" : "published",
    availability: {
      value: scenario.action === "delist" ? "delisted" : "available",
      label: "目前狀態",
      note: "目前取得條件。",
    },
    acquisition: { available: scenario.action !== "delist", note: "目前取得條件。" },
  };
  page.route(`**/skills/${SKILL}/publication`, async ({ method, body }) => {
    if (method !== "GET") {
      writes.push({ method, body: body ?? null });
      if (scenario.held === "write") await gate;
      written = true;
      if (scenario.action === "first") published = scenario.status === 201;
      return {
        status: scenario.status,
        json: scenario.status === 503 ? { error: "upstream unavailable" } : changed,
      };
    }
    reads.own += 1;
    if (written && scenario.held === "own") await gate;
    if (scenario.readFailure && reads.own === 2)
      return { status: 503, json: { error: "unavailable" } };
    return published
      ? { json: written ? changed : OWN_PUBLICATION }
      : { status: 404, json: { error: "not published" } };
  });
  page.route("**/me/publications", async () => {
    reads.overview += 1;
    if (written && scenario.held === "overview") await gate;
    return {
      json: {
        publications: OWN_PUBLICATIONS.publications.map((row) => ({
          ...row,
          ...(written
            ? {
                name,
                address: changed.address,
                status: changed.status,
                acquisition: changed.acquisition,
                catalog_exposure: {
                  state: scenario.action === "delist" ? "not_eligible" : "listed",
                },
              }
            : {}),
        })),
      },
    };
  });
  return { release, reads, writes };
}

async function assertReconciledPublication(page: App, scenario: Scenario, writes: Write[]) {
  const { section, heading, attestation, license, nameInput, publish, logout } = controls(page);
  const alerts = section.locator("[role=alert]");
  if (scenario.readFailure) {
    await page.eventually(() => {
      expect(alerts.text()).toContain("暫時無法讀取發佈狀態");
      expect(publish.count()).toBe(0);
    });
    await section.locator("button").filter({ hasText: "再試一次" }).click();
  }
  await page.eventually(() => {
    expect(publish.isDisabled()).toBe(false);
    expect(attestation.isDisabled()).toBe(false);
    expect(license.isDisabled()).toBe(false);
    expect((scenario.held === "overview" ? logout : heading).isFocused()).toBe(true);
    if (scenario.status === 503) expect(alerts.text()).toContain("上面的狀態已重新讀取");
    else expect(alerts.count()).toBe(0);
    if (scenario.action === "first" && scenario.status === 503)
      expect(nameInput.value()).toBe("chosen-name");
    else
      expect(
        section
          .locator("[role=status]")
          .filter({ hasText: /^狀態：/ })
          .text(),
      ).toContain(scenario.action === "delist" ? "已撤回" : "已發佈");
  });
  expect(writes).toEqual([
    {
      method: scenario.action === "delist" ? "DELETE" : "POST",
      body:
        scenario.action === "delist"
          ? null
          : {
              ...(scenario.action === "first" ? { name: "chosen-name" } : {}),
              version_id: VERSION,
              rights_attested: true,
              declared_license: "MIT",
            },
    },
  ]);
}

for (const scenario of scenarios) {
  test(`publication ${scenario.action} waits for ${scenario.held} after ${scenario.status}${scenario.readFailure ? " and read failure" : ""}`, async () => {
    const page = app();
    const { release, reads, writes } = stubPublicationOperation(page, scenario);
    const { section, heading, attestation, license, nameInput, publish, delist, logout } =
      controls(page);
    try {
      await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
      if (scenario.action === "first") {
        await page.eventually(() => expect(nameInput.count()).toBe(1));
        const described = nameInput.attribute("aria-describedby");
        expect(page.locator(`#${described}`).text()).toBe(NAME_NOTE);
        expect(page.getByText(NAME_NOTE, { exact: true }).count()).toBe(1);
        await nameInput.fill("chosen-name");
      } else await page.eventually(() => expect(section.text()).toContain("已列入 Catalog"));
      await attestation.check();
      await page.eventually(() =>
        expect((attestation.element() as HTMLInputElement).checked).toBe(true),
      );
      await license.select("MIT");
      if (scenario.action === "delist") {
        await delist.click();
        await section.locator("button").filter({ hasText: "確認撤回" }).click();
      } else await publish.click();
      await page.eventually(() => {
        expect(writes.length).toBe(1);
        if (scenario.held !== "write") expect(reads[scenario.held]).toBe(2);
        expect(heading.isFocused()).toBe(true);
        expect(attestation.isDisabled()).toBe(true);
        expect(license.isDisabled()).toBe(true);
        const submit = page.getByRole("button", {
          name: scenario.action === "delist" ? "發佈 v2" : "送出中…",
          exact: true,
        });
        expect(submit.isDisabled()).toBe(true);
        if (scenario.action === "publish") expect(delist.isDisabled()).toBe(true);
        if (scenario.action === "first") expect(nameInput.isDisabled()).toBe(true);
        expect(section.text()).not.toContain("上面的狀態已重新讀取");
        if (scenario.action !== "first")
          expect(
            section.locator("[role=status]").filter({ hasText: "正在送出或核對發佈狀態" }).count(),
          ).toBe(1);
      });
      if (scenario.held === "overview") await logout.focus();
      release();
      await assertReconciledPublication(page, scenario, writes);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      release();
    }
  });
}
