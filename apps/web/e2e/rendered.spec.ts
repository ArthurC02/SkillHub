import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { SKILL } from "../src/fixtures/platform";
import { PHONE_ROUTES } from "./routes";
import { stubPlatform } from "./stub";

/**
 * 03:QA-008 / 04 丙-21③ — the three things `src/a11y.test.tsx` states in its own
 * header that it cannot prove, proven here and nowhere else.
 *
 * Nothing else belongs in this file. The jsdom tier already renders all 12
 * routes and scans them against 88 axe rules with none disabled; giving that
 * answer a second home would triple its cost across three engines and buy
 * nothing. What jsdom structurally cannot decide is:
 *
 *   1. **Composite pixels.** axe's `color-contrast` returns `incomplete` for
 *      every node under jsdom — with no layout there is no way to resolve what
 *      is behind one. `src/contrast.test.ts` measures static hex tokens instead
 *      and says plainly that alpha, `opacity` multipliers and pairs missing
 *      from its hand-written list are beyond it. A real engine composites, so
 *      the rule reaches a verdict.
 *   2. **Real layout.** `table-layout: fixed` does not overflow a phone, it
 *      squeezes, and jsdom computes neither.
 *   3. **The real Tab key.** jsdom does not implement it, so the other file
 *      asserts tab-ability rather than tab order.
 */

test.describe("QA-008 composite pixels", () => {
  /**
   * The second assertion is the one that matters. Zero `color-contrast`
   * violations proves nothing by itself — that is exactly what jsdom reports,
   * by never deciding. So this asserts the rule left `incomplete` first, and
   * only then that it found nothing. Without that check this tier quietly
   * becomes the hole it was built to close.
   */
  /**
   * Scanned on the two routes this tier can reach without inventing a fixture,
   * which is 2 of 12 — and the honest reading of that number is narrower than
   * it looks. 04 丙-21③ named four `rgba()` tokens as out of reach. Two of them,
   * `--social-bg` and `--shadow`, turned out to be painted by no rule at all and
   * have since been deleted. Of the two that remain:
   *
   *   - `--accent-border` is only ever a `border-color`. Borders are 1.4.11
   *     non-text, which `color-contrast` does not judge in any engine.
   *   - `--accent-bg` is the only one that lands behind text — `.notice` and
   *     `.compare-differs`, both compositing it over `--bg`. The first is on
   *     screen below.
   *
   * So the token that mattered is covered. What the other ten routes would add
   * is their own text on the ordinary background, which the static tier already
   * measures — worth having, not worth a forty-field fixture per route.
   */
  for (const [route, where] of [
    ["/?q=pdf", "search results, both .notice bars"],
    ["/policy", "the retention table"],
  ] as const) {
    /**
     * The second assertion is the one that matters. Zero `color-contrast`
     * violations proves nothing by itself — that is exactly what jsdom reports,
     * by never deciding. So this asserts the rule left `incomplete` first, and
     * only then that it found nothing. Without that check this tier quietly
     * becomes the hole it was built to close.
     */
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

  /**
   * The ring drawn for :focus-visible has to survive compositing too.
   *
   * Tab is pressed until something inside the page takes focus rather than
   * once: WebKit leaves links out of the tab sequence by default, matching
   * Safari's "Keyboard navigation" setting being off, so the first press there
   * lands on nothing while it reaches the masthead link in the other two. The
   * question this test asks — does the ring paint — is the same either way, and
   * hard-coding one press would have answered it only for Chromium and Firefox.
   */
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
  /**
   * Every address the router declares, at phone width. This used to check one
   * page, and one page was not enough: the Test Case detail screen carries
   * `<textarea cols={60}>` and `<input size={50}>`, whose intrinsic sizing is
   * wider than a 375px viewport, and it rendered 504px wide with controls
   * bleeding past the edge for as long as nothing looked.
   *
   * The assertion is on the document, not on any element: whatever a page does
   * internally, the page itself must not scroll sideways. A table that is too
   * wide is expected to scroll inside `.table-scroll`, and the second
   * assertion below keeps that distinction honest.
   */
  for (const [name, url] of PHONE_ROUTES) {
    test(`the page does not scroll sideways at 375px: ${name}`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();

      const doc = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(
        doc.scrollWidth,
        `the page scrolls horizontally: ${doc.scrollWidth}px inside ${doc.clientWidth}px`,
      ).toBeLessThanOrEqual(doc.clientWidth);
    });
  }

  /**
   * ...and the overflow a comparison table does have goes where it was meant
   * to. `table-layout: fixed` plus `width: 100%` does not overflow on a phone,
   * it squeezes, so before `.table-scroll` a 4-column table became shreds of
   * wrapped text rather than something a finger could push sideways.
   */
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

  /**
   * 設計 §4.5 的行長上限，量在真的排版過的段落上。
   *
   * jsdom cannot decide this at all: it has no line boxes, so a paragraph is as
   * wide as the assertion imagines. Before the cap, #root's 1126px was every
   * paragraph's width — measured 2026-09-03, the longest line on a page ran
   * between 60 and 104 CJK characters against a comfortable 25–40, and eight of
   * the 18 routes are 65–83% `.note` by character count. That is the shape
   * behind 「整頁文字非常滿」.
   *
   * Asserted on the element's own font-size rather than on a pixel width, which
   * is the rule itself: 40em is 40 CJK characters at any step of the §4.1 scale.
   * Tables are skipped for the same reason they are out of the CSS selector —
   * a cell's width belongs to its table.
   */
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
        // The widest LINE, not the widest box: a card's padding and a pill's
        // border are not line length, and measuring the box would report them
        // as if they were.
        const range = document.createRange();
        range.selectNodeContents(el);
        const w = Math.max(...Array.from(range.getClientRects()).map((r) => r.width), 0);
        if (w > 40 * em + 1) bad.push(`${Math.round(w / em)}em: ${text.slice(0, 20)}`);
      }
      return bad;
    });
    expect(over, `wider than 40em: ${over.join(" / ")}`).toEqual([]);
  });

  /**
   * The link that IS the page's action has a control's box, not a sentence's.
   *
   * `design-system.test.ts` proves the class has a CSS rule somewhere;
   * only a real engine proves the rule reaches this element and produces a
   * pressable target. Measured against the same floor as every other control —
   * WCAG 2.2 2.5.8's 24px, which the app sets at 32 — because that floor is the
   * whole reason to give a link a box: before this, 打包並下載這個版本 was a 20px
   * line of text in a paragraph while 儲存 and 刪除, on the same screens, were
   * boxes. Delete the rule and this drops to the line height.
   */
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

  /**
   * A `.note` that follows a `.badge` is separated from the pill's border.
   *
   * The note in that position is always a §2.11(c) disclaimer — 「收錄不等於精
   * 選。」, 「平台不曾執行它們——」 — whose entire job is to stop the badge being
   * read as an endorsement. Measured 2026-09-03 at 0.0px on four routes at both
   * widths, i.e. rendered as 「已收錄收錄不等於精選。」, which reads as part of the
   * badge instead of as a limit on it.
   *
   * Only a real engine can see it: the pill is `inline-flex` with 12px of its
   * own padding, so the glyphs are 12px apart while the border touches — the
   * defect is between an element's box and a neighbour's text, and jsdom has
   * neither.
   */
  test("a disclaimer beside a badge is not fused to it", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/?q=pdf+%E6%91%98%E8%A6%81");
    await expect(page.locator(".search-result").first()).toBeVisible();

    const fused = await page.evaluate(() => {
      const bad: string[] = [];
      for (const pill of Array.from(document.querySelectorAll(".badge"))) {
        const note = pill.nextElementSibling;
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

  /**
   * 04 丙-135 — a fact and its qualifier are two colours on a result card.
   *
   * Measured 2026-09-03 at 1280×900: two cards on `/?q=pdf` shared 86 of 200
   * characters verbatim, and the shared half sat in the same visual channel as
   * the half that discriminates — same 14px, same `--text`, 8px apart. What a
   * reader saw on one row was 「已收錄收錄不等於精選。」, one run of text with no
   * signal of where the fact ends and the caveat begins.
   *
   * The qualifier cannot be lifted out of the card to fix that. §2.11(c)
   * requires every badge to state what it does not cover 「在同一個區塊」, and
   * §4.3's whole argument for cards is 「每一則要能被單獨判斷」 — which makes the
   * card the block — and it states outright that a card has to hold the risk
   * summary, the verification state and the refusal reasons with none of them
   * folded. So the shape gives way instead: the value takes `--text-h`, the
   * token this app already uses for 「the thing the reader is looking for」
   * (`.verdict`, `.compare-table th[scope=row]`, `.badge`), and the note keeps
   * `--text`.
   *
   * Asserted here rather than in jsdom because it is the resolved cascade of a
   * custom property that is being compared, and because the two colours have to
   * differ **as painted** — the composite-contrast scan above already runs on
   * this same route, so a promotion that broke AA would be caught next to it.
   */
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
      // A run that found nothing to compare proves nothing — the same failure
      // shape the composite-contrast test above guards against.
      if (checked === 0) bad.push("no facet row on this page carried a qualifier");
      return bad;
    });
    expect(same, `fact and qualifier share a colour: ${same.join(" / ")}`).toEqual([]);
  });
});

test.describe("QA-008 the real Tab key", () => {
  /**
   * `a11y.test.tsx` states it cannot prove that a real browser's Tab order
   * matches DOM order. This presses the key and checks.
   *
   * It asserts the sequence never goes backwards rather than asserting one
   * exact list, because the three engines legitimately disagree about **which**
   * elements are in the sequence: WebKit omits links unless Safari's "Keyboard
   * navigation" is switched on, so an exact list would encode Chromium's answer
   * and fail WebKit for being Safari. What must hold everywhere is the ordering
   * itself — focus moving up the document, never jumping back — and that is the
   * property jsdom cannot see.
   */
  test("tab order never goes backwards through the document", async ({ page }) => {
    await stubPlatform(page);
    await page.goto("/");
    await expect(page.locator(".app-nav a").first()).toBeVisible();

    // Mark every element in document order so a focused node can report where
    // it sits without this side having to model focusability per engine.
    await page.evaluate(() => {
      document.querySelectorAll("*").forEach((el, i) => el.setAttribute("data-dom-index", `${i}`));
    });

    // Collection stops at the wrap rather than after a fixed count. Tab cycles,
    // and WebKit's sequence here is short — it leaves links out — so a fixed
    // twelve presses would run past the end, come back to the top and look
    // exactly like focus jumping backwards.
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
