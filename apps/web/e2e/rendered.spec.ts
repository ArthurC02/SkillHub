import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { RUN, SKILL, platformResponse } from "../src/fixtures/platform";
import { PHONE_ROUTES, ROUTES } from "./routes";
import { stubPlatform } from "./stub";

test.describe("QA-008 composite pixels", () => {
  for (const [route, where] of [
    ["/?q=pdf", "search results, both .notice bars"],
    ["/policy", "the retention table"],
  ] as const) {
    test(`color-contrast decides and passes: ${where}`, async ({ page }) => {
      await stubPlatform(page);
      await page.goto(route);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      if (route === "/?q=pdf") {
        await expect(page.locator(".notice")).toHaveCount(2);
      }

      const results = await new AxeBuilder({ page }).withRules(["color-contrast"]).analyze();

      expect(
        results.incomplete.filter((r) => r.id === "color-contrast"),
        "color-contrast came back incomplete — this tier decided nothing",
      ).toEqual([]);
      expect(
        results.passes.some((r) => r.id === "color-contrast"),
        "color-contrast never ran at all",
      ).toBe(true);
      expect(results.violations).toEqual([]);
    });
  }

  test("the focus ring is actually painted", async ({ page }) => {
    await stubPlatform(page);
    await page.goto("/");

    let outline: { width: string; style: string } | null = null;
    for (let i = 0; i < 6 && outline === null; i++) {
      await page.keyboard.press("Tab");
      outline = await page.evaluate(() => {
        const el = document.activeElement;
        if (!el || el === document.body) return null;
        const s = getComputedStyle(el);
        return { width: s.outlineWidth, style: s.outlineStyle };
      });
    }

    expect(outline, "six presses of Tab focused nothing in the page").not.toBeNull();
    expect(outline!.style).not.toBe("none");
    expect(parseFloat(outline!.width)).toBeGreaterThan(0);
  });
});

test.describe("QA-008 real layout", () => {
  for (const [name, url] of PHONE_ROUTES) {
    test(`the page does not scroll sideways at 375px: ${name}`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();

      const doc = await page.evaluate(() => {
        const limit = document.documentElement.clientWidth;
        const over = Array.from(document.querySelectorAll("*")).filter(
          (el) =>
            el.getBoundingClientRect().right > limit + 0.5 || el.scrollWidth > el.clientWidth + 0.5,
        );
        return {
          scrollWidth: document.documentElement.scrollWidth,
          clientWidth: limit,
          // Deepest only: an ancestor of an overflowing element overflows too,
          // and a list led by html/body would name nothing useful.
          culprits: over
            .filter((el) => !over.some((other) => other !== el && el.contains(other)))
            .slice(0, 5)
            .map((el) => {
              const box = el.getBoundingClientRect();
              const cls = typeof el.className === "string" ? el.className.trim() : "";
              const at = `${el.tagName.toLowerCase()}${cls ? "." + cls.split(/\s+/).join(".") : ""}`;
              return `${at} w=${Math.round(box.width)} right=${Math.round(box.right)} scrollWidth=${el.scrollWidth}`;
            }),
        };
      });
      expect(
        doc.scrollWidth,
        `the page scrolls horizontally: ${doc.scrollWidth}px inside ${doc.clientWidth}px` +
          ` — past the edge: ${doc.culprits.join(" | ") || "(no element found)"}`,
      ).toBeLessThanOrEqual(doc.clientWidth);
    });
  }

  test("a wider native file widget does not push the page sideways: skill-detail", async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto(`/skills/${SKILL}`);
    await page.waitForSelector('.version-upload input[type="file"]');
    await page.addStyleTag({
      content: '.version-upload input[type="file"] { font-size: 20px }',
    });
    const doc = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
      file: Math.round(
        document.querySelector('.version-upload input[type="file"]')!.getBoundingClientRect().width,
      ),
    }));
    expect(
      doc.scrollWidth,
      `a wider file widget (${doc.file}px) pushed the page to ${doc.scrollWidth}px`,
    ).toBeLessThanOrEqual(doc.clientWidth);
  });

  test("手機頁首收在兩列以內，標題與身分同一列（設計 §4.5）", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    await page.goto("/workspace/skills");
    await expect(page.locator(".app-nav a").first()).toBeVisible();

    const header = await page.evaluate(() => {
      const el = document.querySelector(".app-header")!;
      const box = (node: Element) => {
        const r = node.getBoundingClientRect();
        return { top: r.top, bottom: r.bottom };
      };
      return {
        height: Math.round(el.getBoundingClientRect().height),
        title: box(el.querySelector(".app-title")!),
        auth: box(el.lastElementChild!),
      };
    });

    expect(header.height, `375px 下頁首高 ${header.height}px：它又長回三列了`).toBeLessThanOrEqual(
      130,
    );
    expect(
      header.title.bottom > header.auth.top && header.auth.bottom > header.title.top,
      `標題與身分沒有在同一列上——頁首的第一列又被一個 auto 留白推開了：` +
        `標題 ${Math.round(header.title.top)}–${Math.round(header.title.bottom)}、` +
        `身分 ${Math.round(header.auth.top)}–${Math.round(header.auth.bottom)}（頁首高 ${header.height}px）`,
    ).toBe(true);
  });

  for (const width of [1440, 1280]) {
    test(`頁首橫貫視窗，標題與 h1 同一條左緣：${width}px（設計 §4.5）`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/workspace/skills");
      await expect(page.locator(".app-title")).toBeVisible();

      const m = await page.evaluate(() => {
        const x = (sel: string) =>
          Math.round(document.querySelector(sel)!.getBoundingClientRect().x);
        return {
          headerWidth: Math.round(
            document.querySelector(".app-header")!.getBoundingClientRect().width,
          ),
          viewport: document.documentElement.clientWidth,
          title: x(".app-title"),
          h1: x("main h1"),
        };
      });

      expect(
        m.headerWidth,
        `頁首只有 ${m.headerWidth}px 而視窗是 ${m.viewport}px：它又縮回欄寬裡了`,
      ).toBe(m.viewport);
      expect(m.title, `標題左緣 ${m.title} 對不上 h1 的 ${m.h1}`).toBe(m.h1);
    });
  }

  test("建立卡的動作落在同一條基線上（設計 §4.3）", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/workspace/skills");
    await expect(page.locator(".create-cards > li").first()).toBeVisible();

    const tops = await page.evaluate(() =>
      [...document.querySelectorAll(".create-cards > li > p:last-child")].map((el) =>
        Math.round(el.getBoundingClientRect().top),
      ),
    );

    expect(tops.length, "一張卡都沒有量到").toBeGreaterThan(1);
    expect(new Set(tops).size, `三顆動作落在 ${tops.join("／")} 三個高度上`).toBe(1);
  });

  test("工作區導覽是一條列，標題與連結同高（設計 §4.3）", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/workspace/skills");
    await expect(page.locator(".workspace-index")).toBeVisible();

    const rows = await page.evaluate(() => {
      const box = (s: string) => {
        const r = document.querySelector(s)!.getBoundingClientRect();
        return { top: r.top, bottom: r.bottom };
      };
      return { label: box(".workspace-index h2"), links: box(".workspace-index .chip-row") };
    });

    const overlap =
      Math.min(rows.label.bottom, rows.links.bottom) - Math.max(rows.label.top, rows.links.top);
    expect(overlap, "標題被推到連結上面一列去了").toBeGreaterThan(0);

    const links = await page.evaluate(() =>
      [...document.querySelectorAll(".workspace-index a")].map((a) => {
        const cs = getComputedStyle(a);
        return {
          text: a.textContent ?? "",
          height: Math.round(a.getBoundingClientRect().height),
          framed: cs.borderStyle !== "none" || cs.backgroundColor !== "rgba(0, 0, 0, 0)",
        };
      }),
    );

    expect(links.length, "一條連結都沒有量到").toBeGreaterThan(0);
    for (const l of links) {
      expect(l.framed, `「${l.text}」被畫成按鈕了`).toBe(false);
      expect(l.height, `「${l.text}」的命中區只有 ${l.height}px`).toBeGreaterThanOrEqual(40);
    }
  });

  test("a wide table scrolls inside its own container", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto("/policy");
    await expect(page.locator(".compare-table")).toBeVisible();

    const scroller = await page
      .locator(".table-scroll")
      .first()
      .evaluate((el) => ({ scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }));
    expect(scroller.scrollWidth, "nothing to scroll — the table squeezed instead").toBeGreaterThan(
      scroller.clientWidth,
    );
  });

  test("no paragraph is wider than the §4.5 measure", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator("h1")).toBeVisible();

    const over = await page.evaluate(() => {
      const bad: string[] = [];
      for (const el of Array.from(document.querySelectorAll("main p, main li, main dd"))) {
        if (el.closest("table")) continue;
        const text = (el.textContent || "").replace(/\s+/g, "");
        if (text.length < 20) continue;
        const em = parseFloat(getComputedStyle(el).fontSize);
        const range = document.createRange();
        range.selectNodeContents(el);
        const w = Math.max(...Array.from(range.getClientRects()).map((r) => r.width), 0);
        if (w > 40 * em + 1) bad.push(`${Math.round(w / em)}em: ${text.slice(0, 20)}`);
      }
      return bad;
    });
    expect(over, `wider than 40em: ${over.join(" / ")}`).toEqual([]);
  });

  test("a primary action link is a control-sized target", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator("a.action").first()).toBeVisible();

    const boxes = await page.evaluate(() =>
      Array.from(document.querySelectorAll("a.action")).map((a) => ({
        text: (a.textContent || "").trim().slice(0, 14),
        height: Math.round(a.getBoundingClientRect().height),
        border: getComputedStyle(a).borderTopWidth,
      })),
    );
    expect(boxes.length, "no primary action on the skill page").toBeGreaterThan(0);
    for (const b of boxes) {
      expect(
        b.height,
        `「${b.text}」 is ${b.height}px tall — a line, not a control`,
      ).toBeGreaterThanOrEqual(32);
      expect(b.border, `「${b.text}」 has no box`).not.toBe("0px");
    }
  });

  test("a disclaimer beside a badge is not fused to it", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/?q=pdf+%E6%91%98%E8%A6%81");
    await expect(page.locator(".search-result").first()).toBeVisible();

    const fused = await page.evaluate(() => {
      const bad: string[] = [];
      for (const pill of Array.from(document.querySelectorAll(".badge"))) {
        let note: Element | null = pill.nextElementSibling;
        while (note && !note.classList.contains("note")) {
          if (!note.classList.contains("tip")) break;
          note = note.nextElementSibling;
        }
        if (!note?.classList.contains("note")) continue;
        const range = document.createRange();
        range.selectNodeContents(note);
        const first = Array.from(range.getClientRects()).find((r) => r.width > 0);
        const box = pill.getBoundingClientRect();
        if (!first) continue;
        if (Math.min(box.bottom, first.bottom) - Math.max(box.top, first.top) <= 4) continue;
        const gap = first.left - box.right;
        if (gap < 6) bad.push(`${gap.toFixed(1)}px after 「${pill.textContent?.trim()}」`);
      }
      return bad;
    });
    expect(fused, `fused to the badge: ${fused.join(" / ")}`).toEqual([]);
  });

  test("a fact and its qualifier are not the same colour on a result card", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/?q=pdf+%E6%91%98%E8%A6%81");
    await expect(page.locator(".search-result").first()).toBeVisible();

    const same = await page.evaluate(() => {
      const bad: string[] = [];
      const dds = Array.from(document.querySelectorAll(".search-result .result-facets dd"));
      let checked = 0;
      for (const dd of dds) {
        const note = dd.querySelector(".note");
        if (!note) continue;
        checked++;
        const value = getComputedStyle(dd).color;
        const qualifier = getComputedStyle(note).color;
        if (value === qualifier)
          bad.push(`「${(dd.textContent ?? "").trim().slice(0, 20)}」 both ${value}`);
      }
      if (checked === 0) bad.push("no facet row on this page carried a qualifier");
      return bad;
    });
    expect(same, `fact and qualifier share a colour: ${same.join(" / ")}`).toEqual([]);
  });

  test("at most one filled primary action per page, and only on a.action/button.action", async ({
    page,
  }) => {
    test.slow();
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });

    const bad: string[] = [];
    let routesWithOne = 0;

    for (const [name, url] of ROUTES) {
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();

      const found = await page.evaluate(() => {
        const probe = document.createElement("div");
        document.body.appendChild(probe);
        const unpainted = getComputedStyle(probe).backgroundColor;
        probe.style.backgroundColor = "var(--cta)";
        const cta = getComputedStyle(probe).backgroundColor;
        probe.remove();

        const filled: { tag: string; cls: string; text: string; action: boolean }[] = [];
        for (const el of Array.from(document.body.querySelectorAll("*"))) {
          if (getComputedStyle(el).backgroundColor !== cta) continue;
          const tag = el.tagName.toLowerCase();
          filled.push({
            tag,
            cls: el.getAttribute("class") ?? "",
            text: (el.textContent ?? "").trim().slice(0, 16),
            action: (tag === "a" || tag === "button") && el.classList.contains("action"),
          });
        }
        return { cta, unpainted, filled };
      });

      if (!found.cta || found.cta === found.unpainted) {
        bad.push(
          `${name}: --cta resolved to 「${found.cta}」 — nothing was measured on this route`,
        );
        continue;
      }

      if (found.filled.length > 1) {
        const which = found.filled.map((f) => `<${f.tag}.${f.cls}>「${f.text}」`).join(" + ");
        bad.push(`${name}: ${found.filled.length} filled actions — ${which}`);
      }
      if (found.filled.length === 1) routesWithOne++;

      for (const f of found.filled) {
        if (f.cls.split(/\s+/).includes("badge")) {
          bad.push(`${name}: a .badge is filled 「${f.text}」 — 填色屬於動作，不屬於主張`);
        } else if (!f.action) {
          bad.push(`${name}: <${f.tag} class="${f.cls}">「${f.text}」 carries the --cta fill`);
        }
      }
    }

    if (routesWithOne === 0) {
      bad.push("no route had a filled primary action at all — this test proved nothing");
    }

    expect(bad, `§4.6.3 一頁一個主要動作: ${bad.join(" / ")}`).toEqual([]);
  });
});

test.describe("QA-008 the real Tab key", () => {
  test("tab order never goes backwards through the document", async ({ page }) => {
    await stubPlatform(page);
    await page.goto("/");
    await expect(page.locator(".app-nav a").first()).toBeVisible();

    await page.evaluate(() => {
      document.querySelectorAll("*").forEach((el, i) => el.setAttribute("data-dom-index", `${i}`));
    });

    const seen: number[] = [];
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press("Tab");
      const at = await page.evaluate(() => {
        const el = document.activeElement;
        if (!el || el === document.body) return null;
        const raw = el.getAttribute("data-dom-index");
        return raw === null ? null : Number(raw);
      });
      if (at === null) continue;
      if (seen.includes(at)) break;
      seen.push(at);
    }

    expect(seen.length, "Tab reached nothing in the page at all").toBeGreaterThan(2);
    const sorted = [...seen].sort((x, y) => x - y);
    expect(seen, `focus jumped backwards: ${seen.join(" → ")}`).toEqual(sorted);
  });
});

test.describe("ADR-065 the text budget and the fourth disclosure, in a real engine", () => {
  const TEACHING_FLAT: Record<string, number> = {
    policy: 95,
    "skill-detail": 78,
    packaging: 61,
    "lab-run": 84,
    "lab-datasets": 16,
    "lab-test-cases": 37,
    "lab-test-case-detail": 319,
    "run-trace": 51,
    "workspace-account": 42,
    "workspace-downloads": 107,
    "workspace-runs": 18,
    "workspace-skills": 21,
  };

  test("flat teaching text: ≤100 runes a block, and never more than the day it was measured", async ({
    page,
  }) => {
    test.slow();
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });

    const bad: string[] = [];
    const measured: Record<string, number> = {};
    for (const [name, url] of ROUTES) {
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      const blocks = await page.evaluate(() =>
        Array.from(document.querySelectorAll('[data-role="teaching"]')).map((el) => ({
          flat: !el.closest("[hidden]") && el.getClientRects().length > 0,
          runes: [...(el.textContent ?? "").replace(/\s+/g, "")].length,
          head: (el.textContent ?? "").trim().slice(0, 16),
        })),
      );
      const flat = blocks.filter((b) => b.flat);
      measured[name] = flat.reduce((sum, b) => sum + b.runes, 0);
      for (const b of flat) {
        if (b.runes > 100) {
          bad.push(
            `${name}: a ${b.runes}-rune teaching block 「${b.head}」 — §2.13 單一 D 區塊 ≤100`,
          );
        }
      }
      const cap = TEACHING_FLAT[name] ?? 0;
      if (measured[name] > cap) {
        bad.push(
          `${name}: ${measured[name]} flat teaching runes, over the ${cap} measured on 2026-09-04 — the ratchet only moves down`,
        );
      }
    }
    if (Object.values(measured).every((n) => n === 0)) {
      bad.push("no route has any data-role=teaching — the marks are gone or the scan broke");
    }
    expect(
      bad,
      `§2.13 D 類預算: ${bad.join(" / ")}\nmeasured: ${JSON.stringify(measured)}`,
    ).toEqual([]);
  });

  test("a Tip opens without moving a neighbour, and Escape closes it", async ({ page }) => {
    await stubPlatform(page);
    await page.route(`**/runs/${RUN}/trace`, (route) => {
      const { body, status } = platformResponse(route.request().url());
      return route.fulfill({ status, json: { ...(body as object), status: "running" } });
    });
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`/runs/${RUN}`);

    const trigger = page.locator("button.tip-trigger");
    await expect(trigger).toHaveCount(1);
    await expect(trigger).toContainText("為什麼可以關掉這一頁");
    const content = page.locator("p.tip-content");
    await expect(content).toBeHidden();

    const box = await trigger.boundingBox();
    expect(box?.height ?? 0, "the trigger is smaller than the 24px floor").toBeGreaterThanOrEqual(
      24,
    );

    // Document coordinates (+ scrollY), not viewport-relative: opening the
    // trigger scrolls it into view, which would shift every viewport-relative
    // top. Skips <option>: Chromium reports it as zero-size until interacted with.
    const positions = () =>
      page.evaluate(() =>
        Array.from(document.querySelectorAll("main *"))
          .filter((el) => !el.closest("[data-tip]") && el.tagName !== "OPTION")
          .map((el) => Math.round(el.getBoundingClientRect().top + window.scrollY)),
      );
    const before = await positions();
    const heightBefore = await page.evaluate(() => document.documentElement.scrollHeight);

    await trigger.click();
    await expect(content).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(await positions(), "opening the Tip moved something else on the page").toEqual(before);
    expect(
      await page.evaluate(() => document.documentElement.scrollHeight),
      "opening the Tip changed the page height",
    ).toBe(heightBefore);

    await page.keyboard.press("Escape");
    await expect(content).toBeHidden();
    await expect(trigger).toBeFocused();
  });
});
