import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { RETENTION, SEARCH } from "./fixtures";

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

/**
 * Every request the app makes is answered from here — no platform, no Postgres,
 * no model key. Two things this had to learn the hard way:
 *
 * Routes match in reverse registration order, so the catch-all goes in first.
 * And it cannot be scoped by path prefix: the platform owns /me and /policy/*
 * as well as /api/*, and anything let through reaches the preview server, which
 * answers index.html to every unknown path — a 200 full of HTML, parsed as
 * JSON, failing a long way from its cause. So it catches everything and filters
 * on what the request is for; documents, styles and scripts are the app itself
 * loading and have to go through.
 *
 * There is no `/api/skills/*` route even though a detail fixture is the obvious
 * way to reach a comparison table: that glob also matches `/api/skills/search`
 * and, registered later, would win — handing the search page a skill-detail
 * body. /policy reaches the same table through one four-field response instead.
 */
async function stubApi(page: Page) {
  await page.route("**/*", (route, request) => {
    const kind = request.resourceType();
    if (kind === "document" || kind === "stylesheet" || kind === "script" || kind === "font") {
      return route.continue();
    }
    return route.fulfill({ status: 401, json: { error: "unstubbed in e2e" } });
  });
  await page.route("**/me", (route) =>
    route.fulfill({ status: 401, json: { error: "anonymous" } }),
  );
  await page.route("**/api/skills/search**", (route) => route.fulfill({ json: SEARCH }));
  await page.route("**/policy/data-retention", (route) => route.fulfill({ json: RETENTION }));
}

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
   * it looks. Of the four `rgba()` tokens 04 丙-21③ named as out of reach:
   *
   *   - `--social-bg` and `--shadow` are declared in both themes and used
   *     nowhere. Nothing composites them because nothing paints them.
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
      await stubApi(page);
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
    await stubApi(page);
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
   * /policy needs no session and its response has four fields, so the fixture
   * behind this table cannot drift far from the truth. The page must not
   * scroll sideways; the table inside it must.
   */
  test("a comparison table does not push the page sideways at 375px", async ({ page }) => {
    await stubApi(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto("/policy");
    await expect(page.locator(".compare-table")).toBeVisible();

    const doc = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(doc.scrollWidth, "the page itself scrolls horizontally").toBeLessThanOrEqual(
      doc.clientWidth,
    );

    // ...and the overflow went where it was meant to go, rather than the table
    // squeezing its columns into shreds the way it did before .table-scroll.
    const scroller = await page
      .locator(".table-scroll")
      .first()
      .evaluate((el) => ({ scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }));
    expect(scroller.scrollWidth, "nothing to scroll — the table squeezed instead").toBeGreaterThan(
      scroller.clientWidth,
    );
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
    await stubApi(page);
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
