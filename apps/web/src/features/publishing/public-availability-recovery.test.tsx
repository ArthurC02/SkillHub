import { expect, test } from "vitest";
import { PUBLIC_BUNDLE_PUBLICATION, PUBLIC_PUBLICATION } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

const DELISTED_NOTE = "作者撤回了這個發佈物，這一頁不再提供它的內容。";
const SEARCH = 'input[placeholder="搜尋小工具或描述任務"]';

function loseConnectivity() {
  Object.defineProperty(navigator, "onLine", { configurable: true, get: () => false });
  window.dispatchEvent(new Event("offline"));
}

async function publicationScenario(
  page: App,
  kind: "skill" | "bundle",
  next: "unavailable" | "missing" | "failure" | "available",
  pause = false,
) {
  const initial = kind === "skill" ? PUBLIC_PUBLICATION : PUBLIC_BUNDLE_PUBLICATION;
  const address = `/publications/${initial.publisher}/${initial.name}`;
  let release!: () => void;
  const hold = new Promise<void>((resolve) => {
    release = resolve;
  });
  const state = { reads: 0, writes: 0, release, url: initial.address };
  const unavailable = {
    kind: initial.kind,
    publisher: initial.publisher,
    name: initial.name,
    address: initial.address,
    availability: { value: "delisted", label: "作者已撤回", note: DELISTED_NOTE },
    delisted_at: "2026-10-20T18:00:00Z",
    releases: [],
    exposure: { available: false, note: DELISTED_NOTE },
    acquisition: { available: false, note: DELISTED_NOTE },
  };
  page.route(
    (url) => url.pathname === address + "/acquisitions",
    async ({ method }) => {
      if (method !== "POST") return undefined;
      state.writes++;
      if (pause) loseConnectivity();
      return next === "missing"
        ? { status: 404, json: { error: "not found" } }
        : { status: 409, json: { error: DELISTED_NOTE, reason: "delisted" } };
    },
  );
  page.route(
    (url) => url.pathname === address,
    async ({ method }) => {
      if (method !== "GET") return undefined;
      state.reads++;
      if (state.reads === 1) return { json: initial };
      await hold;
      if (next === "missing")
        return { status: 404, json: { error: "no publication has this address" } };
      if (next === "failure" && state.reads === 2)
        return { status: 500, json: { error: "publication lookup failed" } };
      return { json: next === "available" ? initial : unavailable };
    },
  );
  await page.goto(state.url);
  const download = page.getByRole("button", { name: "下載", exact: true });
  await page.eventually(() => expect(download.count()).toBe(1));
  await download.click();
  return state;
}

async function withoutObsoleteContent(page: App) {
  await page.eventually(() => {
    expect(page.getByRole("button", { name: "下載", exact: true }).count()).toBe(0);
    expect(page.locator(".publication-trust").count()).toBe(0);
    expect(page.getByRole("heading", { name: "取得這份套件", exact: true }).count()).toBe(0);
    expect(page.locator("main").isFocused()).toBe(true);
  });
}

async function expectAccessible(page: App) {
  expect(await page.axeViolations("main")).toEqual([]);
}

for (const kind of ["skill", "bundle"] as const) {
  for (const status of [404, 409]) {
    test(`a ${kind} acquisition rejected with ${status} withholds obsolete content through its availability check`, async () => {
      const page = app();
      const state = await publicationScenario(
        page,
        kind,
        status === 404 ? "missing" : "unavailable",
      );
      try {
        await page.eventually(() => {
          expect(page.getByRole("status").text()).toContain("載入這個發佈物中");
          expect(page.getByRole("heading", { name: "公開發佈物", exact: true }).count()).toBe(1);
          expect(state.reads).toBe(2);
        });
        await withoutObsoleteContent(page);
        await expectAccessible(page);
        state.release();
        await page.eventually(() => {
          expect(page.getByRole("alert").text()).toBe(
            status === 404 ? "沒有這個發佈物。" : "作者已撤回",
          );
          if (status === 409)
            expect(page.getByText(DELISTED_NOTE, { exact: true }).count()).toBe(1);
        });
        await withoutObsoleteContent(page);
        expect(page.pathname()).toBe(state.url);
        expect(state.reads).toBe(2);
        expect(state.writes).toBe(1);
        await expectAccessible(page);
      } finally {
        state.release();
      }
    });
  }

  test(`a ${kind} availability read failure retries the read without resending its rejected acquisition`, async () => {
    const page = app();
    const state = await publicationScenario(page, kind, "failure");
    try {
      await page.eventually(() =>
        expect(page.getByRole("status").text()).toContain("載入這個發佈物中"),
      );
      state.release();
      await page.eventually(() =>
        expect(page.getByRole("alert").text()).toContain("暫時無法讀取這個發佈物"),
      );
      await withoutObsoleteContent(page);
      await expectAccessible(page);
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() => expect(page.getByRole("alert").text()).toBe("作者已撤回"));
      await withoutObsoleteContent(page);
      expect(page.pathname()).toBe(state.url);
      expect(state.reads).toBe(3);
      expect(state.writes).toBe(1);
      await expectAccessible(page);
    } finally {
      state.release();
    }
  });
}

test("a paused public availability check explains the wait and resumes only the read", async () => {
  const page = app();
  const state = await publicationScenario(page, "skill", "unavailable", true);
  try {
    await page.eventually(() =>
      expect(page.getByRole("status").text()).toContain("等待連線以讀取這個發佈物"),
    );
    await withoutObsoleteContent(page);
    expect(state.reads).toBe(1);
    expect(state.writes).toBe(1);
    await expectAccessible(page);
    state.release();
    await page.setOffline(false);
    await page.eventually(() => expect(page.getByRole("alert").text()).toBe("作者已撤回"));
    await withoutObsoleteContent(page);
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
    await expectAccessible(page);
  } finally {
    state.release();
    await page.setOffline(false);
  }
});

test("a newly available public address retains its rejected acquisition outcome", async () => {
  const page = app();
  const state = await publicationScenario(page, "skill", "available");
  try {
    await page.eventually(() =>
      expect(page.getByRole("status").text()).toContain("載入這個發佈物中"),
    );
    await withoutObsoleteContent(page);
    await expectAccessible(page);
    state.release();
    const refusal = page.getByRole("alert").filter({ hasText: DELISTED_NOTE });
    await page.eventually(() => {
      expect(page.getByText("上次下載的回饋", { exact: true }).count()).toBe(1);
      expect(refusal.isFocused()).toBe(true);
      expect(page.getByRole("button", { name: "下載", exact: true }).isDisabled()).toBe(false);
      expect(page.getByRole("heading", { name: "PDF Summariser", exact: true }).count()).toBe(1);
    });
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
    await expectAccessible(page);
  } finally {
    state.release();
  }
});

test("an availability check preserves focus moved to the page header", async () => {
  const page = app();
  const state = await publicationScenario(page, "skill", "unavailable");
  try {
    await page.eventually(() =>
      expect(page.getByRole("status").text()).toContain("載入這個發佈物中"),
    );
    const search = page.locator(SEARCH);
    await search.fill("PDF");
    await expectAccessible(page);
    state.release();
    await page.eventually(() => {
      expect(page.getByRole("alert").text()).toBe("作者已撤回");
      expect(search.isFocused()).toBe(true);
    });
    await search.fill("PDF tools");
    expect(search.value()).toBe("PDF tools");
    expect(page.getByRole("button", { name: "下載", exact: true }).count()).toBe(0);
    expect(page.locator(".publication-trust").count()).toBe(0);
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
    await expectAccessible(page);
  } finally {
    state.release();
  }
});

for (const kind of ["skill", "bundle"] as const) {
  test(`a reopened ${kind} publication preserves ongoing header search input`, async () => {
    const page = app();
    const state = await publicationScenario(page, kind, "available");
    try {
      await page.eventually(() =>
        expect(page.getByRole("status").text()).toContain("載入這個發佈物中"),
      );
      const search = page.locator(SEARCH);
      await search.fill("PDF");
      await expectAccessible(page);
      state.release();
      await page.eventually(() => {
        expect(
          page
            .getByRole("heading", {
              name: kind === "skill" ? "PDF Summariser" : "pdf-toolkit",
              exact: true,
            })
            .count(),
        ).toBe(1);
        expect(page.getByRole("alert").text()).toBe(DELISTED_NOTE);
        expect(search.isFocused()).toBe(true);
      });
      await search.fill("PDF tools");
      expect(search.value()).toBe("PDF tools");
      expect(page.getByRole("button", { name: "下載", exact: true }).isDisabled()).toBe(false);
      expect(state.reads).toBe(2);
      expect(state.writes).toBe(1);
      await expectAccessible(page);
    } finally {
      state.release();
    }
  });
}
