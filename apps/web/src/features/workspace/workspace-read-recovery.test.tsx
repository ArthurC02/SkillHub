import { expect, test, vi } from "vitest";
import {
  ACTIVITY,
  OWN_PUBLICATIONS,
  PUBLICATION,
  PUBLISHER,
  RUN,
  RUNS,
  platformResponse,
} from "../../testing/fixtures/platform";
import { Locator, renderedApp, type App } from "../../testing/app";

const app = renderedApp();

const HOME_CREATION = {
  id: "88888888-8888-4888-8888-888888888888",
  revision: 1,
  state: "waiting_input",
  snapshot: {
    messages: [],
    brief: "尚待確認的創作",
    brief_confirmed: false,
    acceptance_criteria: [],
    diagram_understanding: "",
    diagram_confirmed: false,
    references: [],
    pending_action: "",
    budget_credits: 1300,
    reserved_credits: 0,
    usage_unknown: false,
    steps: 1,
    tool_calls: 0,
  },
  created_at: "2026-10-14T00:00:00Z",
  updated_at: "2026-10-14T00:00:00Z",
  expires_at: "2099-01-01T00:00:00Z",
  deadline: "2099-01-01T00:00:00Z",
};

const UNAVAILABLE = {
  complete: false,
  unavailable_sources: ["evaluation", "publishing"],
  error: "service unavailable",
};

const ACTIVITY_ITEMS = ".activity-list > li";

function withCreationFeatures(page: App) {
  page.route("**/me", () => ({
    json: {
      ...(platformResponse("/me").body as object),
      features: { generate_skill: true, creation_skill: true },
    },
  }));
}

function writes(page: App) {
  return page.requests.filter((request) => request.method !== "GET");
}

for (const status of [401, 403, 404]) {
  test(`activity pagination ${status} withholds previously loaded records`, async () => {
    const page = app();
    let firstReads = 0;
    let nextReads = 0;
    page.route("**/me/activity?*", ({ url }) => {
      if (url.searchParams.has("cursor")) {
        nextReads += 1;
        return {
          status,
          json: {
            error:
              status === 401 ? "not authenticated" : status === 403 ? "forbidden" : "not found",
          },
        };
      }
      firstReads += 1;
      return { json: firstReads === 1 ? { ...ACTIVITY, next_cursor: "older-page" } : ACTIVITY };
    });
    await page.goto("/activity");
    await expect(page.locator(ACTIVITY_ITEMS)).toHaveCount(3);
    await page.getByRole("button", { name: "載入更多活動" }).click();
    if (status === 401) {
      await page.eventually(() => {
        expect(page.getByRole("status").filter({ hasText: "Workspace 活動需要登入" }).count()).toBe(
          1,
        );
        expect(page.getByRole("button", { name: "再試一次" }).count()).toBe(0);
      });
    } else {
      await expect(page.getByRole("alert")).toContainText("暫時無法讀取Workspace 活動");
    }
    await page.eventually(() => {
      expect(page.locator(".activity-list, .activity-summary").count()).toBe(0);
      expect(page.getByText("以下僅保留已載入的紀錄").count()).toBe(0);
    });
    if (status !== 401) {
      await page.getByRole("button", { name: "再試一次" }).click();
      await page.eventually(() => {
        expect(page.locator(ACTIVITY_ITEMS).count()).toBe(3);
        expect(page.locator("#activity-heading").isFocused()).toBe(true);
      });
      expect(firstReads).toBe(2);
      expect(nextReads).toBe(1);
    }
  });
}

for (const empty of [false, true]) {
  test(`activity withdraws a cached ${empty ? "empty" : "populated"} result after a failed reread`, async () => {
    const page = app();
    let reads = 0;
    page.route("**/me/activity?*", () => {
      reads += 1;
      return reads === 2
        ? { status: 503, json: UNAVAILABLE }
        : { json: empty && reads === 1 ? { ...ACTIVITY, items: [] } : ACTIVITY };
    });
    await page.goto("/activity");
    await expect(page.locator("dd").filter({ hasText: "5 / 5" })).toHaveCount(1);
    await page.getByRole("button", { name: "重新整理", exact: true }).press("Enter");
    await page.eventually(() => {
      expect(page.getByRole("alert").text()).toContain("Evaluation、Publishing");
      expect(page.locator(".activity-summary").count()).toBe(0);
      expect(page.locator(".activity-list").count()).toBe(0);
      expect(page.getByRole("heading", { name: "目前沒有活動", exact: true }).count()).toBe(0);
      expect(page.locator("#activity-heading").isFocused()).toBe(true);
    });
    await page.getByRole("button", { name: "再試一次" }).press("Enter");
    await page.eventually(() => {
      expect(page.locator(ACTIVITY_ITEMS).count()).toBe(3);
      expect(page.getByRole("alert").count()).toBe(0);
    });
    expect(reads).toBe(3);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}

test("an activity next-page failure retains the loaded page and retries its cursor", async () => {
  const page = app();
  let firstReads = 0;
  const cursors: string[] = [];
  page.route("**/me/activity?*", ({ url }) => {
    const cursor = url.searchParams.get("cursor");
    if (!cursor) {
      firstReads += 1;
      return { json: { ...ACTIVITY, next_cursor: "older-page" } };
    }
    cursors.push(cursor);
    return cursors.length === 1
      ? { status: 503, json: UNAVAILABLE }
      : {
          json: {
            ...ACTIVITY,
            items: [
              {
                ...ACTIVITY.items[0],
                source_id: RUN,
                summary: "較早的試跑",
                continuation: { kind: "run", run_id: RUN },
              },
            ],
          },
        };
  });
  await page.goto("/activity");
  await expect(page.locator(ACTIVITY_ITEMS)).toHaveCount(3);
  await page.getByRole("button", { name: "載入更多活動" }).click();
  await page.eventually(() => {
    expect(page.getByRole("alert").text()).toContain("僅保留已載入的紀錄，尚未讀完");
    expect(page.getByRole("alert").text()).toContain("Evaluation、Publishing");
    expect(page.locator(ACTIVITY_ITEMS).count()).toBe(3);
    expect(page.locator(".activity-summary").count()).toBe(0);
    expect(page.getByRole("button", { name: "載入更多活動" }).count()).toBe(0);
  });
  await page.getByRole("button", { name: "再試一次" }).press("Enter");
  await page.eventually(() => {
    expect(page.locator(ACTIVITY_ITEMS).count()).toBe(4);
    expect(page.getByText("已載入全部活動", { exact: true }).count()).toBe(1);
    expect(page.locator("#activity-heading").isFocused()).toBe(true);
  });
  expect(firstReads).toBe(1);
  expect(cursors).toEqual(["older-page", "older-page"]);
  expect(await page.axeViolations("main")).toEqual([]);
});

function stubHomeSummaries(page: App, empty: boolean) {
  const owners = [
    {
      pattern: "**/runs?*",
      error: "需要處理的試跑",
      heading: "workspace-attention-heading",
      initial: empty
        ? { runs: [] }
        : { runs: [RUNS.runs[1], { ...RUNS.runs[0], status: "running" }] },
      recovered: { runs: [RUNS.runs[1]] },
    },
    {
      pattern: "**/skills?*",
      error: "你的資產",
      heading: "workspace-assets-heading",
      initial: empty
        ? { skills: [], total: 0, limit: 100, truncated: false }
        : platformResponse("/skills").body,
      recovered: platformResponse("/skills").body,
    },
    {
      pattern: "**/me/publications",
      error: "發佈成果",
      heading: "workspace-publications-heading",
      initial: empty ? { publications: [] } : OWN_PUBLICATIONS,
      recovered: OWN_PUBLICATIONS,
    },
    {
      pattern: "**/creation-sessions",
      error: "最近的創作",
      heading: "workspace-creations-heading",
      initial: empty ? [] : [HOME_CREATION],
      recovered: [HOME_CREATION],
    },
  ];
  let fail = false;
  let releaseFailure!: () => void;
  const resumedRead = new Promise<void>((resolve) => {
    releaseFailure = resolve;
  });
  const reads = owners.map(() => 0);
  const retried = owners.map(() => false);
  for (const [index, owner] of owners.entries()) {
    page.route(owner.pattern, async () => {
      reads[index] += 1;
      if (fail && !retried[index]) await resumedRead;
      return fail && !retried[index]
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: retried[index] ? owner.recovered : owner.initial };
    });
  }
  return {
    owners,
    reads,
    retried,
    releaseFailure,
    failReads: () => {
      fail = true;
    },
  };
}

function homeSection(page: App, headingId: string) {
  return new Locator(() =>
    page
      .locator(`#${headingId}`)
      .all()
      .map((heading) => heading.closest(".workspace-home-section")!),
  );
}

async function recoverHomeOwners(page: App, summaries: ReturnType<typeof stubHomeSummaries>) {
  const { owners, reads, retried } = summaries;
  for (const [index, owner] of owners.entries()) {
    const section = homeSection(page, owner.heading);
    await expect(section.getByRole("alert")).toContainText(`暫時無法讀取${owner.error}`);
    retried[index] = true;
    const before = [...reads];
    try {
      await page.setOffline(true);
      await section.getByRole("button", { name: "再試一次" }).press("Enter");
      await page.eventually(() => {
        expect(
          section
            .getByRole("status")
            .filter({ hasText: `等待連線以讀取${owner.error}` })
            .count(),
        ).toBe(1);
        expect(section.getByRole("alert").count()).toBe(0);
        expect(page.getByText("目前無法確認哪些試跑仍在執行").count()).toBe(0);
      });
      expect(reads).toEqual(before);
    } finally {
      await page.setOffline(false);
    }
    await page.eventually(() => {
      expect(section.getByRole("alert").count()).toBe(0);
      expect(reads.map((count, i) => count - before[i])).toEqual(
        owners.map((_, i) => (i === index ? 1 : 0)),
      );
      expect(section.getByRole("status").filter({ hasText: "等待連線以讀取" }).count()).toBe(0);
      expect(page.locator(`#${owner.heading}`).isFocused()).toBe(true);
    });
  }
}

for (const [empty, phase] of [
  [false, "paused"],
  [true, "paused"],
  [false, "recovery"],
  [true, "recovery"],
] as const) {
  test(`workspace home ${phase} with cached ${empty ? "empty" : "populated"} summaries ${phase === "paused" ? "waits without claiming current outcomes" : "retries each owner independently"}`, async () => {
    const page = app();
    withCreationFeatures(page);
    const summaries = stubHomeSummaries(page, empty);
    const { owners, reads, releaseFailure, failReads } = summaries;
    await page.goto("/workspace");
    await page.eventually(() => {
      expect(page.getByRole("heading", { name: "繼續推進你的工作" }).count()).toBe(1);
      expect(reads.every((count) => count === 1)).toBe(true);
      expect(page.locator("main [data-loading]").count()).toBe(0);
    });
    await page.getByRole("link", { name: "資料保存政策", exact: true }).click();
    await page.eventually(() => expect(page.pathname()).toBe("/policy"));
    failReads();
    const main = page.locator("main");
    try {
      await page.setOffline(true);
      await page
        .getByRole("navigation", { name: "主要導覽" })
        .getByRole("link", { name: "工作台", exact: true })
        .click();
      await page.eventually(() => {
        for (const owner of owners)
          expect(
            main
              .getByRole("status")
              .filter({ hasText: `等待連線以讀取${owner.error}` })
              .count(),
          ).toBe(1);
        expect(main.getByRole("alert").count()).toBe(0);
        expect(main.locator(".loading-skeleton").count()).toBe(0);
        expect(main.locator(".workspace-home-list, .workspace-home-assets").count()).toBe(
          empty ? 0 : 5,
        );
        expect(main.getByText("目前沒有").count()).toBe(0);
        expect(main.getByText("資產庫目前是空的。").count()).toBe(0);
        expect(main.getByRole("button", { name: "等待連線…", exact: true }).count()).toBe(
          empty ? 0 : 1,
        );
      });
      expect(reads).toEqual([1, 1, 1, 1]);
      for (const refresh of main.getByRole("button", { name: "等待連線…", exact: true }).all())
        expect((refresh as HTMLButtonElement).disabled).toBe(true);
      if (phase === "paused") expect(await page.axeViolations("main")).toEqual([]);
      await page.setOffline(false);
      await page.eventually(() => {
        expect(reads).toEqual([2, 2, 2, 2]);
        expect(main.getByRole("status").filter({ hasText: "等待連線以讀取" }).count()).toBe(0);
        if (empty)
          for (const owner of owners)
            expect(
              main
                .getByRole("status")
                .filter({ hasText: `載入${owner.error}中…` })
                .count(),
            ).toBe(1);
        expect(main.getByText("目前沒有").count()).toBe(0);
        expect(main.getByText("資產庫目前是空的。").count()).toBe(0);
      });
    } finally {
      releaseFailure();
      await page.setOffline(false);
    }
    await page.eventually(() => {
      expect(page.getByRole("alert").count()).toBe(4);
      expect(page.getByText("目前無法確認哪些試跑仍在執行").count()).toBe(1);
      expect(page.locator(".workspace-home-list, .workspace-home-assets").count()).toBe(0);
      for (const text of [
        "目前沒有需要你留意的試跑。",
        "目前沒有正在執行的試跑。",
        "目前沒有發佈成果。",
        "資產庫目前是空的。",
      ])
        expect(page.getByText(text).count()).toBe(0);
    });
    if (phase === "paused") {
      expect(writes(page)).toEqual([]);
      return;
    }
    await recoverHomeOwners(page, summaries);
    await expect(page.getByText("尚待確認的創作", { exact: true })).toHaveCount(1);
    expect(await page.axeViolations("main")).toEqual([]);
    expect(writes(page)).toEqual([]);
  });
}

test("workspace home with a late creation response does not offer an expired continuation", async () => {
  const page = app();
  const startedAt = Date.parse("2026-10-17T00:00:00Z");
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(startedAt);
  withCreationFeatures(page);
  let reads = 0;
  let release!: () => void;
  const response = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route("**/creation-sessions", async () => {
    reads++;
    await response;
    return {
      json: [
        {
          ...HOME_CREATION,
          snapshot: { ...HOME_CREATION.snapshot, brief: "不應出現的過期創作" },
          deadline: new Date(startedAt + 60_000).toISOString(),
          expires_at: new Date(startedAt + 120_000).toISOString(),
        },
      ],
    };
  });
  try {
    await page.goto("/workspace");
    const main = page.locator("main");
    await page.eventually(() => {
      expect(main.getByRole("heading", { name: "繼續推進你的工作", exact: true }).count()).toBe(1);
      expect(reads).toBe(1);
      expect(main.getByRole("status").filter({ hasText: "載入最近的創作中…" }).count()).toBe(1);
    });
    const navigation = page
      .getByRole("navigation", { name: "主要導覽" })
      .getByRole("link", { name: "工作台", exact: true });
    await navigation.focus();
    vi.setSystemTime(startedAt + 60_001);
    release();
    await page.eventually(() => {
      expect(main.getByRole("status").filter({ hasText: "載入最近的創作中…" }).count()).toBe(0);
      expect(main.getByRole("heading", { name: "繼續進行", exact: true }).count()).toBe(0);
      expect(main.getByText("不應出現的過期創作", { exact: true }).count()).toBe(0);
      expect(main.getByRole("alert").count()).toBe(0);
      expect(navigation.isFocused()).toBe(true);
    });
    expect(reads).toBe(1);
    expect(writes(page)).toEqual([]);
    expect(await page.axeViolations("main")).toEqual([]);
  } finally {
    release();
    vi.useRealTimers();
  }
});

test("publishing reread failure hides cached releases and retries without losing its continuation", async () => {
  const page = app();
  let reads = 0;
  page.route("**/me/publications", () => {
    reads += 1;
    return reads === 2
      ? { status: 503, json: { error: "service unavailable" } }
      : { json: OWN_PUBLICATIONS };
  });
  await page.goto(`/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`);
  const section = page.locator("#skill-publications");
  await expect(section.getByText("續接位置", { exact: true })).toHaveCount(1);
  await section.getByRole("button", { name: "重新整理", exact: true }).press("Enter");
  await page.eventually(() => {
    expect(section.getByRole("alert").text()).toContain("暫時無法讀取小工具發佈清單");
    expect(section.locator(".download-item").count()).toBe(0);
    expect(section.getByText("還沒有任何小工具 Publication。").count()).toBe(0);
    expect(page.locator("#publication-overview-title").isFocused()).toBe(true);
  });
  await section.getByRole("button", { name: "再試一次" }).press("Enter");
  await page.eventually(() => {
    expect(section.getByText("續接位置", { exact: true }).count()).toBe(1);
    expect(page.locator("#publication-overview-title").isFocused()).toBe(true);
  });
  expect(new URLSearchParams(page.search()).get("publication")).toBe(`${PUBLISHER}/${PUBLICATION}`);
  expect(reads).toBe(3);
  expect(await page.axeViolations("main")).toEqual([]);
});
