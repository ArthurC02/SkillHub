/// <reference types="node" />
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

/**
 * 04 丙-21 ③ / 02:NFR-007 — the colour-contrast guard, at the token layer.
 *
 * `a11y.test.tsx` says in its own header that **a contrast regression will not
 * fail that test**: axe answers `incomplete` for `color-contrast` under jsdom,
 * which computes no layout. The palette was measured by hand instead, and a hand
 * measurement decays the first time somebody edits `index.css`. This file is
 * that measurement, re-run on every build.
 *
 * **What it proves.** For each pair listed in PAIRS below, the two static hex
 * tokens in `index.css` — in both the `:root` and the `prefers-color-scheme:
 * dark` sets — meet the stated WCAG 2.1 ratio. The values are read out of
 * `index.css` itself (plain `readFileSync` — no CSS parser, no new dependency;
 * `?raw` is not an option because Vitest stubs CSS imports empty unless
 * `test.css` is on), never copied here, so the two cannot drift apart: editing a
 * token re-runs the arithmetic against the new value, and deleting one fails the
 * "defined in both themes" assertion below. A second hand-kept copy of the
 * palette in this file would be the thing that rots, which is the failure this
 * test exists to end.
 *
 * **What it does not prove**, in the same spirit as the a11y header:
 *
 * 1. **Anything involving alpha.** `--accent-bg` and `--accent-border` are
 *    `rgba()`; whatever sits on top of them is composited against a background
 *    this file never resolves. `.notice` (text on `--accent-bg`) is therefore
 *    unchecked here — but no longer unchecked anywhere: the browser tier
 *    (ADR-036) composites it in three engines, and that gap was verified closed
 *    by breaking `--accent-bg` on purpose and watching this file stay green
 *    while the browser tier failed.
 * 2. **`opacity`.** A multiplier lands wherever it lands regardless of the
 *    token — which is exactly why QA-009 removed the `opacity: 0.65`–`0.8`
 *    mutes from `index.css` rather than tuning them. Nothing stops them coming
 *    back; only a rendered-pixel check would catch that.
 * 3. **Which pairs actually occur on screen.** PAIRS is a hand-written list,
 *    read off `index.css` and the pages that use those classes. A new rule that
 *    puts `--text` on some third background is a pairing this file has never
 *    heard of. Adding a token pairing means adding a line here.
 * 4. **Rendered pixels** — anti-aliasing, font weight and size at the real
 *    breakpoints, browser colour management. That is QA-008's job (real browser
 *    plus manual walkthrough), and it stays QA-008's job; this is the cheap
 *    layer underneath it that runs in CI.
 */

// `join(import.meta.dirname, …)` rather than `new URL("./index.css",
// import.meta.url)`: Vite rewrites that second form at transform time into an
// asset URL, so it never reaches the filesystem.
const css = readFileSync(join(import.meta.dirname, "index.css"), "utf8");

/**
 * Both palettes, without a CSS parser: `index.css` declares each colour token
 * exactly twice — once in `:root`, once in the dark `@media` block, in that
 * order — so the first hex for a name is light and the second is dark. The
 * "exactly twice" assertion below is what keeps that assumption honest: a token
 * that stops being redefined for dark mode fails here rather than silently
 * inheriting the light value onto a dark background.
 */
const declarations = [...css.matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{3,8})\s*;/g)];

function palette(nth: 0 | 1): Record<string, string> {
  const out: Record<string, string> = {};
  const seen = new Set<string>();
  for (const [, name, hex] of declarations) {
    if (seen.has(name) === Boolean(nth)) out[name] = hex;
    seen.add(name);
  }
  return out;
}

const THEMES = { light: palette(0), dark: palette(1) } as const;

/** WCAG 2.1 relative luminance, sRGB. */
function luminance(hex: string): number {
  const h =
    hex.length === 4
      ? [...hex.slice(1)].map((c) => parseInt(c + c, 16))
      : [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  const [r, g, b] = h.map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

/**
 * The pairings that exist in `index.css`, with the rule each one answers to.
 *
 * 4.5:1 is WCAG 2.1 **1.4.3 Contrast (Minimum)** for body text. Nothing here
 * claims the 3:1 large-text exemption — `h1` (56px) and `.verdict` (20px/500)
 * would qualify, but they use `--text-h`, which clears 4.5:1 anyway, so the
 * stricter bar costs nothing and survives a font-size change.
 *
 * The single 3:1 line is `--accent`, which `index.css` uses only as the 3px
 * `border-left` of `.notice` — never as text. That is WCAG 2.1 **1.4.11
 * Non-text Contrast**. `--border` and `--accent-border` are deliberately absent:
 * they outline cards, table cells and badges whose state is always also stated
 * in words (NFR-007), so they are decoration under 1.4.11, not the sole carrier
 * of any information.
 */
const PAIRS: [fg: string, bg: string, min: number, where: string][] = [
  ["text", "bg", 4.5, "body and .note/.rank/.file-size on the page"],
  ["text", "code-bg", 4.5, ".skill-md and .diff body text"],
  ["text-h", "bg", 4.5, "h1/h2, summary, .app-title, .verdict"],
  ["text-h", "code-bg", 4.5, "code, .counter, .badge"],
  ["danger", "bg", 4.5, ".file-script .script-tag"],
  ["danger", "code-bg", 4.5, ".badge-compat-failed/.badge-risk/.badge-expired text"],
  ["accent", "bg", 3, ".notice border-left — 1.4.11 non-text, never used as text"],
  // --link exists because --accent is 4.39:1 on --bg: it can outline a .notice
  // and it cannot be a link. That distinction only holds while something checks
  // it, so these two lines are the check — without them the reason --link was
  // added is a sentence in a comment rather than a property of the palette.
  ["link", "bg", 4.5, "a, .app-nav links — link text on the page"],
  ["link", "code-bg", 4.5, "a inside .skill-md / .diff / a styled control"],
];

test("QA-009: every colour token is declared once per theme", () => {
  const names = new Set(declarations.map(([, name]) => name));
  for (const name of names) {
    expect(
      declarations.filter(([, n]) => n === name),
      `--${name} must be declared in both :root and the dark @media block`,
    ).toHaveLength(2);
  }
  // Guards the regex itself: a rename or a switch to another colour notation
  // would otherwise leave PAIRS silently comparing `undefined`.
  for (const [fg, bg] of PAIRS) {
    for (const theme of Object.values(THEMES)) {
      expect(theme[fg], `--${fg} not found as a hex token`).toMatch(/^#/);
      expect(theme[bg], `--${bg} not found as a hex token`).toMatch(/^#/);
    }
  }
});

for (const [themeName, theme] of Object.entries(THEMES)) {
  for (const [fg, bg, min, where] of PAIRS) {
    test(`QA-009 (${themeName}): --${fg} on --${bg} ≥ ${min}:1 — ${where}`, () => {
      const ratio = contrast(theme[fg], theme[bg]);
      expect(
        Number(ratio.toFixed(2)),
        `${theme[fg]} on ${theme[bg]} is ${ratio.toFixed(2)}:1, below ${min}:1 (${where})`,
      ).toBeGreaterThanOrEqual(min);
    });
  }
}
