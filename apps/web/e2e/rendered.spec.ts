import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { RUN, SKILL, platformResponse } from "../src/fixtures/platform";
import { PHONE_ROUTES, ROUTES } from "./routes";
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
        // The next `.note`, not the next sibling: §4.7 lets a Tip trigger sit
        // between a badge and its qualifier, and `nextElementSibling` would then
        // find the button, skip the row, and pass without measuring anything.
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

  /**
   * 設計 §3 第 18 條 / §4.6.3（ADR-064 決策 4）— 一頁一個主要動作，數的是**畫出來的填色**。
   *
   * The rule has two halves and they are the same measurement: filling is the
   * channel that belongs to actions, so (a) at most one thing per page may be
   * filled with `--cta`, and (b) the thing that is filled must be the control
   * that finishes the page's work — `<a class="action">` or
   * `<button class="action">`, never a badge. A filled badge reads as an
   * endorsement (§2.3、§2.11(c)), and the first thing that will want one is a
   * paid-tier mark, which is precisely the reading the rule forbids.
   *
   * Only a real engine can count it. What is being compared is the *resolved
   * cascade of a custom property* — the same reason the facet-colour test above
   * lives here — and jsdom does not substitute `var()` in `getComputedStyle` at
   * all, so under jsdom every element's background is the literal string and
   * the count is undecidable rather than wrong. `design-system.test.ts` can
   * prove `.action` has a rule; it cannot prove that exactly one element on a
   * page ends up painted with it.
   *
   * `--cta` is resolved by painting it on a throwaway element and reading back
   * whatever this engine serialises, then comparing that string to other
   * strings from the same engine's serialiser. Nothing here depends on
   * `rgb(…)` vs `rgba(…)` vs `color(…)` formatting, which differs between the
   * three engines and would otherwise make this test a Chromium test.
   *
   * All 18 routes in one test rather than one test per route, because the
   * sentinel is a statement about the set: a suite where every page has zero
   * filled actions passes both assertions above while proving nothing.
   */
  test("at most one filled primary action per page, and only on a.action/button.action", async ({
    page,
  }) => {
    test.slow(); // 18 navigations in one test; the assertion is about the set.
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
        // Read the engine's own serialisation of "no background" first, so the
        // guard below is an equality against this engine rather than a guess at
        // how it spells transparent.
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

      // An unresolvable --cta computes to the initial transparent — the same
      // value every unpainted element on the page has — so the count would be
      // the whole document rather than the one control. Nothing below was
      // measured in that case; say that instead of reporting a number.
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

    // The same hole the facet-colour test guards with `checked === 0`: zero
    // filled actions everywhere satisfies 「至多一個」 and 「只能是 .action」
    // vacuously, and would let the CSS rule be deleted under a green suite.
    if (routesWithOne === 0) {
      bad.push("no route had a filled primary action at all — this test proved nothing");
    }

    expect(bad, `§4.6.3 一頁一個主要動作: ${bad.join(" / ")}`).toEqual([]);
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

test.describe("ADR-065 the text budget and the fourth disclosure, in a real engine", () => {
  /**
   * 設計 §2.13 / ADR-065 決策 1. Class D (teaching) is the one class of visible
   * text with a budget, and until 2026-09-04 the class existed only in people's
   * heads — §6 said so. `data-role="teaching"` is the mark; this sums it per
   * route in the state §2.13 calls default (logged in, loaded, one row, nothing
   * expanded), counting runes the way `FeedbackEntry.tsx` does.
   *
   * Two assertions, of two kinds. Per block, the rule's own number: no flat D
   * block over 100 runes. Per route, a RATCHET and not a threshold — ADR-065
   * 待決策 1 says the value is set from the first measured distribution and may
   * only move down, so the table is that distribution and an entry may only get
   * smaller. A route absent from the table has no flat D at all. Text inside a
   * closed Tip counts zero: it has no client rect, which is the whole point of
   * having moved it.
   *
   * What it cannot see: A/B/C, so it cannot check 「D＋F ≤ A＋B＋C」 — nobody
   * has marked those and §6 keeps saying so. What it will not do: judge whether
   * a mark is on the right sentence. The audit that placed them is the
   * argument; a mark on a caveat makes the caveat count, it does not hide it.
   */
  const TEACHING_FLAT: Record<string, number> = {
    // Measured 2026-09-04 (chromium, 1280×900, shared fixtures). The largest is
    // the Test Case detail page — nine separate teaching notes — and it is the
    // one route the audit found over 「D＋F ≤ A＋B＋C」; §2.13 第 6 條 says the
    // way down from there is copy and dedup, not Tips.
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
    "workspace-skills": 112,
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
    // The vacuous pass: every mark deleted, every route at zero, table satisfied.
    if (Object.values(measured).every((n) => n === 0)) {
      bad.push("no route has any data-role=teaching — the marks are gone or the scan broke");
    }
    expect(
      bad,
      `§2.13 D 類預算: ${bad.join(" / ")}\nmeasured: ${JSON.stringify(measured)}`,
    ).toEqual([]);
  });

  /**
   * 設計 §2.13 Tip 第 2 條 and §4.7: the half of the Tip contract only layout can
   * decide. `tip.test.tsx` proves the DOM shape; this proves that opening one
   * moves nothing else on the page (a folded thing that pushes its neighbours is
   * a `<details>`, and would have been written as one), that the trigger is a
   * real target (WCAG 2.5.8, the app's 32px floor), and that Escape closes it
   * with focus still on the button.
   *
   * The run has to be in flight for `InFlight` — the Tip's first and so far only
   * home — to render at all, and the shared fixture's run is finished. One route
   * override, registered after the stub so it wins, turns the summary into a
   * running one; everything else stays the shared body.
   */
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

    // Document coordinates, not viewport: the click scrolls the trigger into
    // view, and every viewport-relative top shifts by that scroll. What must
    // not move is where things sit on the page.
    const positions = () =>
      page.evaluate(() =>
        Array.from(document.querySelectorAll("main *"))
          // An <option> has no box of its own: Chromium reports 0 for it until the
          // page has been interacted with, then the select's box. Not layout.
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
