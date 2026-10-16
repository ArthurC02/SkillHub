import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUNS, platformResponse } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

const lists = [
  {
    name: "runs",
    url: "/workspace/runs",
    key: "runs",
    id: "run_id",
    title: "skill_name",
    count: 50,
    first: 51,
    next: 100,
    selector: ".download-list > li",
    button: "main > section > button",
    heading: "workspace-runs-heading",
    what: "試跑活動",
  },
  {
    name: "assets",
    url: "/library",
    key: "skills",
    id: "skill_id",
    title: "name",
    count: 24,
    first: 24,
    next: 48,
    selector: ".skill-grid > li",
    button: ".skill-list-progress button",
    heading: "workspace-skills-heading",
    what: "資產庫內容",
  },
] as const;
type List = (typeof lists)[number];
type Arrival = "populated" | "empty" | "error" | "more";

async function workspacePages(page: Page, list: List, arrival: Arrival) {
  await stubPlatform(page);
  const assets = platformResponse("/skills").body as { skills: Array<Record<string, unknown>> };
  const base = list.name === "runs" ? RUNS.runs[0] : assets.skills[0];
  const record = (index: number, title: string) => ({
    ...base,
    [list.id]: `cccccccc-cccc-4ccc-8ccc-${String(index).padStart(12, "0")}`,
    [list.title]: title,
  });
  const initial = Array.from({ length: list.first }, (_, index) =>
    record(index + 1, `初始項目 ${index + 1}`),
  );
  const next =
    arrival === "empty"
      ? []
      : Array.from({ length: arrival === "more" ? list.first : 2 }, (_, index) =>
          record(
            index + 1000,
            index === 0 ? "第一筆新紀錄" : index === 1 ? "第二筆新紀錄" : `後續項目 ${index + 1}`,
          ),
        );
  const offsets: number[] = [];
  const total = arrival === "more" ? list.next + 1 : list.count + 2;
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(`**/${list.key}?*`, async (route) => {
    const offset = Number(new URL(route.request().url()).searchParams.get("offset"));
    offsets.push(offset);
    if (offset === 0)
      return route.fulfill({
        json: { [list.key]: initial, total, limit: 24, truncated: true },
      });
    if (offset === list.next)
      return route.fulfill({
        json: { [list.key]: [], total: list.next, limit: 24, truncated: false },
      });
    await gate;
    if (arrival === "error")
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    return route.fulfill({
      json: {
        [list.key]: next,
        total: arrival === "empty" ? list.count : total,
        limit: 24,
        truncated: arrival === "more",
      },
    });
  });
  return { offsets, writes, release };
}

async function inspect(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  const targets = report.incomplete.flatMap((review) => {
    expect(review.id).toBe("color-contrast");
    return review.nodes.map((node) => {
      expect(node.target).toHaveLength(1);
      return String(node.target[0]);
    });
  });
  const decorations = await page.evaluate(
    (selectors) =>
      selectors.map((selector) => {
        const matches = document.querySelectorAll(selector);
        if (matches.length !== 1) throw new Error("Expected one review target");
        const element = matches[0];
        const card = element.closest(".skill-grid > .skill-card");
        const name = card?.querySelector<HTMLElement>(".skill-card-name");
        function decorationKind(target: Element) {
          if (target.matches(".skill-mono")) return "monogram";
          if (target.matches(".skill-card-validation > span")) return "arrow";
          return target.matches(".skill-menu > summary > .skill-menu-caret") ? "caret" : "unknown";
        }
        const kind = decorationKind(element);
        const counterpart = kind === "monogram" ? name : element.parentElement;
        const rect = counterpart?.getBoundingClientRect();
        function contrastOf(target: Element) {
          const colors = Reflect.get(window, "axe").commons.color;
          const foreground = new colors.Color();
          foreground.parseString(getComputedStyle(target).color);
          let background = new colors.Color();
          let solid = false;
          let opaque = foreground.alpha === 1;
          for (let current: Element | null = target; current; current = current.parentElement) {
            const style = getComputedStyle(current);
            opaque = opaque && style.opacity === "1" && style.backgroundImage === "none";
            background = new colors.Color();
            background.parseString(style.backgroundColor);
            if (background.alpha === 1) {
              solid = true;
              break;
            }
            opaque = opaque && background.alpha === 0;
          }
          return { solid, opaque, contrast: colors.getContrast(background, foreground) };
        }
        const measured =
          kind === "monogram"
            ? { solid: false, opaque: false, contrast: null }
            : contrastOf(element);
        return {
          kind,
          hidden: element.getAttribute("aria-hidden"),
          glyph: element.textContent?.trim() ?? "",
          name: name?.textContent?.trim() ?? "",
          counterpart: counterpart?.textContent?.trim() ?? "",
          label: counterpart?.getAttribute("aria-label"),
          tag: counterpart?.tagName,
          visible: Boolean(card && rect && rect.width > 0 && rect.height > 0),
          ...measured,
        };
      }),
    targets,
  );
  const reviewed = decorations.map((decoration, index) => {
    expect(decoration.hidden).toBe("true");
    expect(decoration.visible).toBe(true);
    expect(decoration.name).not.toBe("");
    if (decoration.kind === "monogram") {
      expect(decoration.name.startsWith(decoration.glyph)).toBe(true);
      expect(Array.from(decoration.glyph)).toHaveLength(1);
    } else if (decoration.kind === "arrow") {
      expect(decoration.glyph).toBe("→");
      expect(decoration.tag).toBe("A");
      expect(decoration.counterpart).toContain("測試題與試跑");
    } else {
      expect(decoration.kind).toBe("caret");
      expect(decoration.glyph).toBe("▾");
      expect(decoration.tag).toBe("SUMMARY");
      expect(decoration.counterpart).toContain("管理");
      expect(decoration.label).toBe(`管理「${decoration.name}」`);
    }
    if (decoration.kind !== "monogram") {
      expect(decoration.solid).toBe(true);
      expect(decoration.opaque).toBe(true);
      expect(decoration.contrast).toBeGreaterThanOrEqual(4.5);
    }
    return { target: [targets[index]], decoration };
  });
  await writeFile(
    test.info().outputPath(`${label}-decorative-review.json`),
    JSON.stringify(reviewed),
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`${label}-phone.png`) });
}

for (const list of lists) {
  for (const arrival of ["populated", "empty", "error", "more"] as const) {
    for (const moved of [false, true]) {
      test(`workspace ${list.name} ${arrival} pagination ${moved ? "preserves moved focus" : "continues keyboard focus"}`, async ({
        page,
      }) => {
        await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
        await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
        const { offsets, writes, release } = await workspacePages(page, list, arrival);
        const rows = page.locator(list.selector);
        const more = page
          .locator(list.button)
          .filter({ hasText: /^(載入更多|載入中…|等待連線…)$/ });
        try {
          await page.goto(list.url);
          await expect(rows).toHaveCount(list.count);
          await more.focus();
          await page.keyboard.press("Enter");
          await expect(more).toHaveText("載入中…");
          await expect(more).toBeDisabled();
          const navigation = page
            .getByRole("navigation", { name: "主要導覽", exact: true })
            .getByRole("link", { name: "工作台", exact: true });
          if (moved) await navigation.focus();
          release();
          if (arrival === "error") {
            await expect(page.getByRole("alert")).toContainText("尚未讀完");
            await expect(rows).toHaveCount(list.count);
          } else
            await expect(rows).toHaveCount(
              arrival === "empty" ? list.count : arrival === "more" ? list.next : list.count + 2,
            );
          if (arrival !== "more") await expect(more).toHaveCount(0);
          if (moved) await expect(navigation).toBeFocused();
          else if (arrival === "error")
            await expect(page.locator(`#${list.heading}`)).toBeFocused();
          else if (arrival === "more") {
            await expect(more).toBeFocused();
            await page.keyboard.press("Enter");
            await expect(more).toHaveCount(0);
            await expect(rows.filter({ hasText: `後續項目 ${list.count}` })).toBeFocused();
          } else {
            const continuation = rows.filter({
              hasText: arrival === "empty" ? `初始項目 ${list.count}` : "第一筆新紀錄",
            });
            await expect(continuation).toBeFocused();
            expect(
              await continuation.evaluate((element) => {
                const rect = element.getBoundingClientRect();
                const top = document.elementFromPoint(
                  rect.x + rect.width / 2,
                  rect.y + rect.height / 2,
                );
                return Boolean(top && element.contains(top));
              }),
            ).toBe(true);
          }
          expect(offsets).toEqual(
            arrival === "more" && !moved ? [0, list.count, list.next] : [0, list.count],
          );
          expect(writes).toEqual([]);
          await inspect(page, "workspace-pagination");
        } finally {
          release();
          await writeFile(
            test.info().outputPath("workspace-pagination-state.json"),
            JSON.stringify({
              surface: list.name,
              arrival,
              moved,
              offsets,
              writes,
              focus: await page.evaluate(() => ({
                tag: document.activeElement?.tagName,
                id: document.activeElement?.id,
                text: document.activeElement?.textContent?.slice(0, 100),
              })),
            }),
          );
        }
      });
    }
  }

  test(`workspace ${list.name} offline pagination explains the wait and resumes once`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 900 });
    await page.emulateMedia({ colorScheme: "dark" });
    const { offsets, writes, release } = await workspacePages(page, list, "populated");
    const rows = page.locator(list.selector);
    const more = page.locator(list.button).filter({ hasText: /^(載入更多|載入中…|等待連線…)$/ });
    try {
      await page.goto(list.url);
      await expect(rows).toHaveCount(list.count);
      await setOffline(page, true);
      await more.focus();
      await page.keyboard.press("Enter");
      await expect(
        page.getByRole("status").filter({ hasText: `等待連線以讀取${list.what}` }),
      ).toBeVisible();
      await expect(more).toBeDisabled();
      await expect(more).toHaveText("等待連線…");
      await expect(rows).toHaveCount(list.count);
      expect(offsets).toEqual([0]);
      await inspect(page, "workspace-pagination-paused");
      await setOffline(page, false);
      await expect.poll(() => offsets.length).toBe(2);
      release();
      await expect(rows).toHaveCount(list.count + 2);
      await expect(rows.filter({ hasText: "第一筆新紀錄" })).toBeFocused();
      expect(offsets).toEqual([0, list.count]);
      expect(writes).toEqual([]);
      await inspect(page, "workspace-pagination-resumed");
    } finally {
      release();
      await setOffline(page, false);
    }
  });
}
