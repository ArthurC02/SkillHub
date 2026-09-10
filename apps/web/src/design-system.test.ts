import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const css = readFileSync(join(import.meta.dirname, "index.css"), "utf8");

const doc = readFileSync(
  join(import.meta.dirname, "..", "..", "..", "docs", "design", "system.md"),
  "utf8",
);

function boldPx(section: string): number[] {
  const from = doc.indexOf(`### ${section}`);
  expect(from, `system.md has no section ${section}`).toBeGreaterThan(-1);
  const next = doc.indexOf("### ", from + 4);
  const body = doc.slice(from, next === -1 ? doc.length : next);
  return [...new Set([...body.matchAll(/\*\*(\d+)px\*\*/g)].map(([, n]) => Number(n)))];
}

const TYPE_SCALE = boldPx("4.1");

const SPACE_SCALE = [0, ...boldPx("4.2")];

const DEVIATIONS = boldPx("5.2");

test("ADR-039: the document's tables are still machine-readable", () => {
  expect(TYPE_SCALE.length, "§4.1 parsed no type steps").toBeGreaterThan(6);
  expect(SPACE_SCALE.length, "§4.2 parsed no spacing steps").toBeGreaterThan(6);
  expect(TYPE_SCALE, "§4.1 lost its body step").toContain(18);
  expect(SPACE_SCALE, "§4.2 lost its base step").toContain(8);
  expect(DEVIATIONS, "§5.2 parsed no deviations").not.toEqual([]);
});

function allowed(kind: "type" | "space") {
  const scale = kind === "type" ? TYPE_SCALE : SPACE_SCALE;
  return new Set([...scale, ...DEVIATIONS]);
}

test("ADR-039 §4.1: every font-size is on the type scale or named in §5", () => {
  const sizes = [...css.matchAll(/font-size:\s*(\d+)px/g)].map(([, n]) => Number(n));
  expect(sizes.length, "no font-size found — the regex or the file moved").toBeGreaterThan(5);
  const ok = allowed("type");
  expect(
    [...new Set(sizes)].filter((s) => !ok.has(s)).sort((a, b) => a - b),
    "a font-size that is neither on the scale nor in the deviation list",
  ).toEqual([]);
});

test("ADR-039 §4.2: every padding/margin/gap length is on the 4px grid or named in §5", () => {
  const decls = [...css.matchAll(/(?:padding|margin|gap)(?:-\w+)?:\s*([^;]+);/g)];
  expect(decls.length, "no spacing declaration found").toBeGreaterThan(20);
  const values = decls.flatMap(([, v]) => [...v.matchAll(/(\d+)px/g)].map(([, n]) => Number(n)));
  const ok = allowed("space");
  expect(
    [...new Set(values)].filter((v) => !ok.has(v)).sort((a, b) => a - b),
    "a spacing value that is neither on the grid nor in the deviation list",
  ).toEqual([]);
});

test("ADR-039 §5: the vocabulary may shrink, not grow", () => {
  expect(
    TYPE_SCALE.length,
    "a tenth type step — merge it into an existing one or argue for it in §4.1",
  ).toBeLessThanOrEqual(11);
  expect(
    SPACE_SCALE.length,
    "an eighth spacing step — 4px grid, or argue for it in §4.2",
  ).toBeLessThanOrEqual(8);
  expect(
    DEVIATIONS.length,
    "the deviation list may only shrink; a new value belongs on the scale, not here",
  ).toBeLessThanOrEqual(2);
});

test("ADR-039 §2.7: colour lives in tokens, and nothing multiplies it", () => {
  const body = css.replace(/\/\*[\s\S]*?\*\//g, "").replace(/--[\w-]+:[^;]+;/g, "");
  expect(
    body.match(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/g) ?? [],
    "a colour literal outside the token blocks — contrast.test.ts cannot see it",
  ).toEqual([]);
  expect(
    body.match(/(^|[;{\s])opacity\s*:/g) ?? [],
    "opacity on anything: removed twice already, and no colour token can follow a multiplier",
  ).toEqual([]);
});

test("ADR-039 §4: index.css is still the only stylesheet", () => {
  expect(
    readdirSync(import.meta.dirname, { recursive: true })
      .map((f) => String(f).replaceAll("\\", "/"))
      .filter((f) => f.endsWith(".css"))
      .sort(),
    "a second stylesheet — the scale guards above only read index.css",
  ).toEqual(["index.css"]);
});

const UNSTYLED: Record<string, string> = {
  "badge-source-package":
    "settled decision: index.css says 作者原文 is a settled fact, so this badge " +
    "deliberately does not take --accent-border's 未知／未驗證 tint. Plain .badge is the visual.",

  "feedback-entry":
    "test hook: a11y.test.tsx selects .feedback-entry form and .feedback-entry [role=alert] (BETA-004)",
  "risk-infos":
    "test hook: disc.test.tsx selects details.risk-infos to assert the info findings stay collapsed",
  "license-expression":
    "test hook: disc.test.tsx asserts an unknown license shows no expression (DISC-008); the class is the only handle",
  "badge-license-source":
    "test hook: the other half of the same DISC-008 assertion in disc.test.tsx",
  "badge-source-template":
    "test hook: disc.test.tsx counts this selector to assert template copy does not borrow the model marker",
};

const INTERPOLATION_MARK = "\u0000";

function classesInMarkup(): Map<string, string[]> {
  const used = new Map<string, string[]>();
  for (const entry of readdirSync(import.meta.dirname, { recursive: true })) {
    const file = String(entry).replaceAll("\\", "/");
    if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
    const body = readFileSync(join(import.meta.dirname, file), "utf8");

    for (const at of body.matchAll(/className=/g)) {
      const start = at.index + at[0].length;
      let expr: string;
      if (body[start] === '"') {
        expr = body.slice(start, body.indexOf('"', start + 1) + 1);
      } else if (body[start] === "{") {
        let depth = 0;
        let end = start;
        while (end < body.length) {
          if (body[end] === "{") depth++;
          else if (body[end] === "}" && --depth === 0) break;
          end++;
        }
        expr = body.slice(start, end + 1);
      } else {
        continue;
      }

      for (const lit of expr.matchAll(/"([^"]*)"|'([^']*)'|`([^`]*)`/g)) {
        if (/[=!]==?\s*$/.test(expr.slice(0, lit.index))) continue;
        const raw = (lit[1] ?? lit[2] ?? lit[3]).replace(/\$\{[^}]*\}/g, INTERPOLATION_MARK);
        for (const token of raw.split(/\s+/)) {
          if (!token || token.includes(INTERPOLATION_MARK)) continue;
          if (!used.has(token)) used.set(token, []);
          if (!used.get(token)!.includes(file)) used.get(token)!.push(file);
        }
      }
    }
  }
  return used;
}

test("ADR-039 §3 第 16 條: every class in the markup has a rule, or a reason", () => {
  const used = classesInMarkup();
  expect(used.size, "no class found in any .tsx — the className scan broke").toBeGreaterThan(40);

  const defined = new Set(
    [...css.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/\.([a-zA-Z][\w-]*)/g)].map(([, c]) => c),
  );

  const unexplained = [...used.keys()].filter((c) => !defined.has(c) && !(c in UNSTYLED)).sort();
  expect(
    unexplained.map((c) => `${c} (${used.get(c)!.join(", ")})`),
    "a class with no rule in index.css and no line in UNSTYLED — either give it a " +
      "visual, or say there which of the two reasons it has for not having one",
  ).toEqual([]);

  expect(
    Object.keys(UNSTYLED).length,
    "the unstyled list may only get shorter; a new class belongs in index.css",
  ).toBeLessThanOrEqual(7);

  expect(
    Object.keys(UNSTYLED)
      .filter((c) => defined.has(c) || !used.has(c))
      .sort(),
    "an entry that is now styled or no longer in the markup — delete the line",
  ).toEqual([]);
});

function componentFiles(): Array<[string, string]> {
  const files: Array<[string, string]> = [];
  for (const entry of readdirSync(import.meta.dirname, { recursive: true })) {
    const file = String(entry).replaceAll("\\", "/");
    if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
    files.push([file, readFileSync(join(import.meta.dirname, file), "utf8")]);
  }
  return files;
}

const TOOLTIP_ONLY: Record<string, string> = {
  "pages/Home.tsx: title={reason}":
    "the disabled filter's own reason, rendered as visible .note text by the same map " +
    "(UNAVAILABLE_FILTERS) two lines below — the scan cannot follow one identifier to two uses",

  'pages/Home.tsx: title="這段摘要由模型改寫，不是套件作者寫的；你的 Agent 讀的是套件自己的 description"':
    "explained in the 標記說明 line above the results list (pages/Home.tsx)",
  'pages/Home.tsx: title="套件自己的 frontmatter description"':
    "explained in the 標記說明 line above the results list",
  'pages/Home.tsx: title="伺服器沒有回報這段摘要的來源"':
    "explained in the 標記說明 line above the results list",
  'pages/Home.tsx: title="這段說明由模型產生，未經人工核對"':
    "explained in the 標記說明 line above the results list",
  'pages/Home.tsx: title="依查詢與文件的關鍵字重疊組出"':
    "explained in the 標記說明 line above the results list",

  'pages/SkillFiles.tsx: title="此檔案為可執行 Script"':
    "the file tree states it below the list in a longer sentence that also gives the count " +
    "and says the platform never runs them — the visible text 設計 §3 第 4 條 asks for",
};

test("ADR-039 §2.4/§2.11(c): a title is never the only place a qualification exists", () => {
  const offenders: string[] = [];
  const seen = new Set<string>();
  let scanned = 0;

  for (const [file, body] of componentFiles()) {
    for (const at of body.matchAll(/title=(?:"([^"]*)"|\{([^}]*)\})/g)) {
      const element = body.slice(Math.max(0, body.lastIndexOf("<", at.index)), at.index);
      if (element.includes("disabled")) continue;
      scanned++;

      const literal = at[1];
      const expression = at[2];
      const visible = literal
        ? body.split(literal).length > 2
        : new RegExp(
            `(?<!title=)\\{\\s*${expression.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\s*\\}`,
          ).test(body);
      const key = `${file}: ${at[0]}`;
      seen.add(key);
      if (!visible && !(key in TOOLTIP_ONLY)) offenders.push(key);
    }
  }

  expect(
    Object.keys(TOOLTIP_ONLY).filter((key) => !seen.has(key)),
    "an exemption on TOOLTIP_ONLY was never encountered: either the scan broke, " +
      "or that title is gone and the line must go with it",
  ).toEqual([]);
  expect(scanned, "no title= found at all — the scan broke").toBeGreaterThan(0);
  expect(
    offenders.sort(),
    "a qualification that exists only in a tooltip. Render it as visible text " +
      "(the tooltip may stay), or add a line to TOOLTIP_ONLY saying where the " +
      "reader actually meets it",
  ).toEqual([]);

  expect(Object.keys(TOOLTIP_ONLY).length, "the list may only get shorter").toBeLessThanOrEqual(7);
});

const RAW_TIMESTAMP: Record<string, string> = {
  "components/Timestamp.tsx: ${at}":
    "the component itself: `<time dateTime={at}>` is where the exact instant belongs",
  "pages/Compare.tsx: ${skill.version.created_at}":
    "a row's `signature`, which is the comparison key the 有差異 highlight is computed from. " +
    "Never rendered — the cell beside it uses <Timestamp>",
  "pages/Downloads.tsx: ${r.downloaded_at}":
    "a React `key`, not a child. The same row renders the instant with <Timestamp>",
};

test("ADR-039 §2.12: no page prints a raw server timestamp", () => {
  const bare = /(?<![=$])\{\s*[A-Za-z0-9_.?[\]]*[A-Za-z0-9_]+_(at|since)\s*\}/g;
  const interpolated = /\$\{[A-Za-z0-9_.?[\]]*[A-Za-z0-9_]+_(at|since)\}/g;
  const sliced = /[A-Za-z0-9_]+_(at|since)\s*\.\s*(slice|substring|substr|split)\s*\(/g;

  const offenders: string[] = [];
  for (const [file, body] of componentFiles()) {
    for (const m of [
      ...body.matchAll(bare),
      ...body.matchAll(interpolated),
      ...body.matchAll(sliced),
    ]) {
      const key = `${file}: ${m[0]}`;
      if (!(key in RAW_TIMESTAMP)) offenders.push(key);
    }
  }

  expect(
    offenders.sort(),
    "a server timestamp rendered as-is. Use <Timestamp at={…} /> (or formatAt for " +
      "an <option> label), so the reader gets their own clock and the DOM keeps the instant",
  ).toEqual([]);
  expect(Object.keys(RAW_TIMESTAMP).length, "the list may only get shorter").toBeLessThanOrEqual(3);
});

const OWN_FAILURE_COPY: Record<string, string> = {
  "components/AuthControls.tsx":
    "a sign-out mutation, not a read — a 401 here means it already worked",
  "components/GenerateSkill.tsx":
    "POST /skills/generate: a mutation the user just pressed, and its refusals are the " +
    "contract's own GenerationFailure values (generateFailureSentence.ts), not read failures",
  "pages/ImportSkill.tsx":
    "POST /skills/import/*: a mutation, and the rejection body is the acceptance criterion " +
    "(CategorizedFindings). LoginRequired covers this page's signed-out arrival before the form",
  "components/SignIn.tsx":
    "POST /auth/dev/login: a sign-IN mutation, the mirror of the sign-out entry above. A 401 " +
    "here cannot mean 「需要登入」 — that is what the user just tried to do — so ReadFailure " +
    "would render the one sentence that is certainly wrong",
};

test("IA-6: a page that writes its own read-failure sentence has to be listed", () => {
  const offenders: string[] = [];
  let scanned = 0;

  for (const [file, body] of componentFiles()) {
    for (const at of body.matchAll(/role="alert"/g)) {
      const rest = body.slice(at.index, at.index + 400);
      const text = rest.slice(0, Math.max(rest.indexOf("</p>"), rest.indexOf("</h")));
      if (!text.includes("失敗")) continue;
      scanned++;
      if (body.slice(Math.max(0, at.index - 300), at.index + 700).includes("ReadFailure")) continue;
      if (file in OWN_FAILURE_COPY) continue;
      offenders.push(file);
    }
  }

  expect(scanned, "no failure alert found at all — the scan broke").toBeGreaterThan(4);
  expect(
    [...new Set(offenders)].sort(),
    "a read failure worded by hand. Use <ReadFailure error={…} what={…} /> so a 401 says " +
      "「需要登入」 and every other status keeps the server's own message; if this failure " +
      "is a mutation rather than a read, add a line to OWN_FAILURE_COPY saying which",
  ).toEqual([]);
  expect(Object.keys(OWN_FAILURE_COPY).length, "the list may only get shorter").toBeLessThanOrEqual(
    4,
  );
});

test("ADR-065 §4.7: at most six icon shapes, every one inline and aria-hidden", () => {
  const sites: string[] = [];
  const meaningful: string[] = [];
  for (const [file, body] of componentFiles()) {
    const code = body.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");
    for (const at of code.matchAll(/<svg\s([^>]*)>/g)) {
      sites.push(file);
      if (!/aria-hidden="true"/.test(at[1])) meaningful.push(file);
    }
  }
  expect(
    sites.length,
    "no <svg> in any component — the scan broke (Tip.tsx has one)",
  ).toBeGreaterThan(0);
  expect(sites, "more than six icon sites — §4.7: 六個以內、逐個有主張的形狀不是集").toHaveLength(
    Math.min(sites.length, 6),
  );
  expect(meaningful, "an <svg> without aria-hidden — §4.7: 圖示不得單獨承載語義").toEqual([]);
  expect(
    readdirSync(join(import.meta.dirname, "..")).filter((f) => /icons?$/i.test(String(f))),
    "an icon directory — §4.7: 不加資產目錄、不加字型、不加套件",
  ).toEqual([]);
});

test("§4.6.3: a door-shaped link wears the same box as the button beside it", () => {
  const rules = css.split("}");
  const base = rules.find((r) => /(^|\n)button,/.test(r) && /min-height:\s*40px/.test(r));
  expect(base, "找不到那條基礎控制項規則（`button, select, textarea, input`）").toBeTruthy();
  expect(
    base,
    ".action-secondary 不在基礎控制項規則裡：戴著它的連結與旁邊的按鈕不再同框",
  ).toContain(".action-secondary");

  for (const state of ["hover", "active"]) {
    const rule = rules.find(
      (r) => r.includes(`button:${state}:not(`) && r.includes(".action-secondary"),
    );
    expect(rule, `.action-secondary 沒有跟著 button 拿到 :${state}——按下去不會有回饋`).toBeTruthy();
  }
});

test("§NFR-007: the nav says which item is current with more than one channel", () => {
  const rules = css.split("}");
  const base = rules.find((r) => r.includes(".app-nav a {"));
  expect(base, "找不到 `.app-nav a`").toBeTruthy();
  expect(
    base,
    "`.app-nav a` 沒有自己的 color：它會繼承全 app 連結的 --link，整列變成紫字",
  ).toContain("color: var(--text)");

  const active = rules.find((r) => r.includes('.app-nav a[data-status="active"]'));
  expect(active, "找不到當前頁那條規則").toBeTruthy();
  expect(active, "當前頁沒有換色——與非當前頁只差一條底線").toContain("color: var(--text-h)");
  expect(active, "當前頁沒有底線——顏色單獨承載了意義（NFR-007）").toContain(
    "text-decoration: underline",
  );
});
