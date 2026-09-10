/// <reference types="node" />
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const css = readFileSync(join(import.meta.dirname, "index.css"), "utf8");

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

const PAIRS: [fg: string, bg: string, min: number, where: string][] = [
  ["text", "bg", 4.5, "body and .note/.rank/.file-size on the page"],
  ["text", "code-bg", 4.5, ".skill-md and .diff body text"],
  ["text-h", "bg", 4.5, "h1/h2, summary, .app-title, .verdict"],
  ["text-h", "code-bg", 4.5, "code, .counter, .badge"],
  ["danger", "bg", 4.5, ".file-script .script-tag"],
  ["danger", "code-bg", 4.5, ".badge-compat-failed/.badge-risk/.badge-expired text"],
  ["accent", "bg", 3, ".notice border-left — 1.4.11 non-text, never used as text"],
  ["link", "bg", 4.5, "a, .app-nav links — link text on the page"],
  ["link", "code-bg", 4.5, "a inside .skill-md / .diff / a styled control"],

  ["text", "surface", 4.5, "card body, .note inside a card, control labels"],
  ["text-h", "surface", 4.5, "h1/h2, .app-title, .verdict on a card or the header"],
  ["danger", "surface", 4.5, ".script-tag and ConfirmDelete's outlined button on a card"],
  ["link", "surface", 4.5, "a inside a card, .app-nav links on the header"],
  ["accent", "surface", 3, ".notice border-left where the notice is a surface"],
  ["border-strong", "surface", 3, "1.4.11 — input/secondary-button edge on a card"],
  ["border-strong", "bg", 3, "1.4.11 — the same edge where a control sits on 地"],

  ["text", "surface-hover", 4.5, "a hovered secondary button or row"],
  ["link", "surface-hover", 4.5, "a hovered link-shaped control"],
  ["text", "surface-active", 4.5, "a pressed secondary button or row"],
  ["link", "surface-active", 4.5, "a pressed link-shaped control — the tightest light pair"],
  ["text-h", "surface-active", 4.5, "a pressed control whose label is a heading token"],

  ["on-cta", "cta", 4.5, ".action — the one filled primary action per page"],

  ["danger", "danger-bg", 4.5, ".notice-danger heading and inline emphasis"],
  ["text-h", "danger-bg", 4.5, ".notice-danger's own heading"],
  ["text", "danger-bg", 4.5, ".notice-danger body text"],
];

test("QA-009: every colour token is declared once per theme", () => {
  const names = new Set(declarations.map(([, name]) => name));
  for (const name of names) {
    expect(
      declarations.filter(([, n]) => n === name),
      `--${name} must be declared in both :root and the dark @media block`,
    ).toHaveLength(2);
  }
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

const luminanceOrder = (hex: string) => contrast(hex, "#000000");

for (const [themeName, theme] of Object.entries(THEMES)) {
  test(`§4.6.2 (${themeName}): 面比地亮，而且亮得看得出來`, () => {
    expect(
      luminanceOrder(theme.surface),
      `--surface ${theme.surface} 必須比 --bg ${theme.bg} 亮（暗色模式唯一成立的方向）`,
    ).toBeGreaterThan(luminanceOrder(theme.bg));
    const step = contrast(theme.surface, theme.bg);
    expect(
      Number(step.toFixed(3)),
      `--surface 對 --bg 只有 ${step.toFixed(3)}:1；兩個不同的 token 不等於看得出來的一階`,
    ).toBeGreaterThanOrEqual(1.1);
  });

  test(`§4.6.2 (${themeName}): 卡片的邊在面上看得見`, () => {
    const edge = contrast(theme.border, theme.surface);
    expect(
      Number(edge.toFixed(3)),
      `--border 對 --surface 只有 ${edge.toFixed(3)}:1；卡片的邊不必達 3:1（那是 --border-strong 的工作），但必須看得見`,
    ).toBeGreaterThanOrEqual(1.4);
  });
}
