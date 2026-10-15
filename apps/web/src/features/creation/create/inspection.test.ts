import { expect, test } from "vitest";
import { readDraftInspection } from "./create.model";

const WARNING = {
  severity: "warning",
  code: "embedded-script",
  message: "SKILL.md 內含可執行程式碼區塊。",
};
const INFO = { severity: "info", code: "external-url", message: "SKILL.md 內有外部網址" };
const ERROR = { severity: "error", code: "skill-md-missing", message: "套件根目錄找不到 SKILL.md" };

test.each([
  ["empty", [], false, { errors: [], warnings: [], infos: [] }],
  ["nil", null, false, { errors: [], warnings: [], infos: [] }],
  ["warning", [WARNING], false, { errors: [], warnings: [WARNING], infos: [] }],
  ["information", [INFO], false, { errors: [], warnings: [], infos: [INFO] }],
  [
    "mixed blocking",
    [INFO, ERROR, WARNING],
    true,
    { errors: [ERROR], warnings: [WARNING], infos: [INFO] },
  ],
] as const)(
  "a complete %s inspection keeps every finding in its severity group",
  (_name, findings, blocked, grouped) => {
    expect(readDraftInspection(JSON.stringify({ findings, blocked }), blocked)).toEqual({
      findings: grouped,
    });
  },
);

test("a server-refused package keeps its plain-text structure reason", () => {
  const reason =
    "套件結構無法通過驗證：duplicate file SKILL.md。frontmatter 與 SKILL.md 由 Go 從 name、description、compatibility、allowed_tools 與 body 產生；files 不得包含 SKILL.md，也沒有 license 欄位可填。";
  expect(readDraftInspection(reason, true)).toEqual({ refusal: reason });
});

test.each<[string, unknown, boolean]>([
  ["null report", null, false],
  ["array report", [], false],
  ["primitive report", 5, false],
  ["missing report fields", {}, false],
  ["missing blocked verdict", { findings: [] }, false],
  ["string blocked verdict", { findings: [], blocked: "false" }, false],
  ["different outer verdict", { findings: [], blocked: false }, true],
  ["missing findings", { blocked: false }, false],
  ["non-array findings", { findings: {}, blocked: false }, false],
  ["null finding", { findings: [null], blocked: false }, false],
  ["primitive finding", { findings: [5], blocked: false }, false],
  ["unknown severity", { findings: [{ ...WARNING, severity: "critical" }], blocked: false }, false],
  ["partial known finding", { findings: [{ severity: "warning" }], blocked: false }, false],
  ["object finding message", { findings: [{ ...WARNING, message: {} }], blocked: false }, false],
  [
    "non-string finding details",
    { findings: [{ ...WARNING, details: [3] }], blocked: false },
    false,
  ],
  [
    "mixed unreadable findings",
    { findings: [INFO, { severity: "critical" }], blocked: false },
    false,
  ],
  ["error without a blocking verdict", { findings: [ERROR], blocked: false }, false],
  ["blocked without errors", { findings: [WARNING], blocked: true }, true],
])(
  "an inspection with %s stays unknown instead of claiming no findings",
  (_name, report, blocked) => {
    expect(readDraftInspection(JSON.stringify(report), blocked)).toBeUndefined();
  },
);

test.each([
  ["empty report", "", true],
  ["blank report", " \n ", true],
  ["unconfirmed plain text", "validation unavailable", false],
  ["truncated object", '{"findings":[', true],
  ["truncated array", "[", true],
] as const)(
  "an inspection with %s cannot become a structure refusal or a clean report",
  (_name, raw, blocked) => {
    expect(readDraftInspection(raw, blocked)).toBeUndefined();
  },
);
