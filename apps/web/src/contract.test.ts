import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";
import * as generated from "@skillhub/api-client-ts";

/**
 * 鐵律 12 的另一半：**契約以 codegen 檢查 drift**，在 `apps/web` 這一側。
 *
 * `api/types.ts` 是 `contracts/openapi/public.yaml` 的 639 行手抄鏡像，而它的檔頭
 * 直到 2026-08-29 都寫著「Codegen into packages/api-client-ts is not wired yet」。
 * 那句話是假的：`packages/api-client-ts/src/generated/models/` 有 161 個 model 檔，
 * `Taskfile.yml`、`tools/devctl` 與 `.github/workflows/ci.yml` 每次都建置它。真正
 * 缺的不是 codegen，是**對帳**——這一側的 drift 檢查只是一句寫在註解裡的
 * 「standing sync obligation」，也就是一個承諾，不是一道門。
 *
 * 這個 repo 已經被同一個形狀咬過一次並逐字記錄（`04` 丙-43）：
 * `file_removed_by_packager` 進了 `public.yaml` 與生成 client，**沒進**
 * `api/packaging.ts` 的手寫 union，於是畫面印出「不能打包：」後面接空白。那次的修
 * 法是加一支對著生成 enum 逐值檢查的測試——做對了兩次，剩下十七份沒做。
 *
 * 三節，各擋一種漂移：
 *
 *   1. **欄位集合與 optionality**（同名 interface ↔ 生成 model）
 *   2. **每一份 enum→中文對照表**，逐值取得非空標籤
 *   3. **`RunStatus` 的三份子集**，聯集必須等於契約
 *
 * fixture 的型別是第四種，落在 `fixtures/platform.ts` 自己的 `satisfies` 上——那裡
 * 是編譯期，比在這裡再抄一次好。
 *
 * **今天它全綠，而那正是加它最好的時機**——加在紅的時候是修 bug，加在綠的時候是
 * 裝棘輪。
 *
 * **這裡刻意不做的事**：改成直接使用生成的 client。它是 camelCase ＋ runtime 轉換
 * 層，換過去是一次大遷移；問題從來不是手寫，是手寫沒有對帳。
 */

const src = import.meta.dirname;
const MODELS = join(
  src,
  "..",
  "..",
  "..",
  "packages",
  "api-client-ts",
  "src",
  "generated",
  "models",
);

// --- 1. field sets ----------------------------------------------------------

/** `deletion_scope` becomes `deletionScope`. The generator lower-camels every field. */
const camel = (snake: string) => snake.replace(/_([a-z0-9])/g, (_, c: string) => c.toUpperCase());

type Fields = Map<string, boolean>;

/**
 * The fields of one `export interface X { … }` block, name to optional.
 *
 * A brace-counting scan rather than a regex over the whole file: both sides
 * carry doc comments containing braces, and only nesting depth tells a field
 * apart from the inside of one.
 */
function fieldsOf(body: string, name: string): Fields | null {
  // `type X = { … }` as well as `interface X { … }`: `ImportResult` in
  // `api/import.ts` is the alias form, and `GenerateSkillResult` extends it.
  const at = body.search(new RegExp(`export (?:interface|type) ${name}\\b`));
  if (at === -1) return null;
  const open = body.indexOf("{", at);
  let depth = 0;
  let end = open;
  while (end < body.length) {
    if (body[end] === "{") depth++;
    else if (body[end] === "}" && --depth === 0) break;
    end++;
  }
  const inner = body
    // Comments first: they hold prose with colons and question marks.
    .slice(open + 1, end)
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\/\/[^\n]*/g, "");

  const fields: Fields = new Map();
  let nest = 0;
  for (const line of inner.split("\n")) {
    const trimmed = line.trim();
    if (nest === 0 && !trimmed.startsWith("[")) {
      // An index signature is not a field: `Me.features` is typed
      // `{ [key: string]: boolean; }` on the generated side.
      const m = /^([A-Za-z_]\w*)(\??)\s*:/.exec(trimmed);
      if (m) fields.set(m[1], m[2] === "?");
    }
    for (const ch of line) {
      if (ch === "{" || ch === "[" || ch === "(") nest++;
      else if (ch === "}" || ch === "]" || ch === ")") nest--;
    }
  }
  return fields;
}

/**
 * `extends` is followed on the hand-written side only.
 *
 * The generator flattens inheritance — `OwnSkill` carries `Skill`'s fields
 * inline — so comparing an `extends`-ing interface without its parent reports
 * every inherited field as missing. Three types here do that.
 */
function handWritten(name: string, types: string): Fields | null {
  const own = fieldsOf(types, name);
  if (!own) return null;
  const ext = new RegExp(`export interface ${name} extends ([\\w, ]+)\\s*\\{`).exec(types);
  if (!ext) return own;
  const merged: Fields = new Map(own);
  for (const parent of ext[1].split(",").map((p) => p.trim())) {
    // A parent may live in another file (api/import.ts); an unresolvable one is
    // skipped rather than guessed at, and the sentinel below keeps that from
    // emptying the comparison.
    const inherited = handWritten(parent, types);
    if (inherited) for (const [k, v] of inherited) if (!merged.has(k)) merged.set(k, v);
  }
  return merged;
}

test("鐵律 12: every hand-written interface with a generated twin has the same fields", () => {
  // Both files: three interfaces here extend one declared in `api/import.ts`,
  // and the generator flattens inheritance, so a parent this scan cannot see
  // reports every inherited field as missing from the contract.
  const types = [
    readFileSync(join(src, "api", "types.ts"), "utf8"),
    readFileSync(join(src, "api", "import.ts"), "utf8"),
  ].join("\n");
  const names = [...types.matchAll(/^export interface (\w+)/gm)].map((m) => m[1]);
  expect(names.length, "api/types.ts parsed no interfaces — the scan broke").toBeGreaterThan(20);

  const models = readdirSync(MODELS).filter((f) => f.endsWith(".ts"));
  expect(models.length, "no generated models — is packages/api-client-ts built?").toBeGreaterThan(
    100,
  );

  const problems: string[] = [];
  let compared = 0;

  for (const name of names) {
    if (!models.includes(`${name}.ts`)) continue;
    const model = readFileSync(join(MODELS, `${name}.ts`), "utf8");
    const there = fieldsOf(model, name);
    const here = handWritten(name, types);
    if (!there || !here) continue;
    compared++;

    for (const [field, optional] of here) {
      const twin = camel(field);
      if (!there.has(twin)) {
        problems.push(`${name}.${field} is not in the contract (generated has no ${twin})`);
      } else if (there.get(twin) !== optional) {
        problems.push(
          `${name}.${field} is ${optional ? "optional" : "required"} here and ` +
            `${there.get(twin) ? "optional" : "required"} in the contract`,
        );
      }
    }
    for (const field of there.keys()) {
      if (![...here.keys()].some((f) => camel(f) === field)) {
        problems.push(`${name}.${field} is in the contract and missing from api/types.ts`);
      }
    }
  }

  // Sentinel: a parse that quietly compares nothing would pass on any drift at
  // all — the exact failure this file exists to stop somebody else having.
  expect(compared, "no interface was actually compared — the name match broke").toBeGreaterThan(15);
  expect(problems.sort(), "api/types.ts and the generated client disagree").toEqual([]);
});

// --- 2. every enum-to-label table, against the enum -------------------------

/**
 * `generate.test.tsx:262` 的六行，再做十五次。
 *
 * `04` 丙-43 把機制寫得比我能寫的更清楚：「TS 的 `Record<union, string>` 只能綁住
 * union 與表，**綁不住 union 與契約**」。兩種失效模式，兩種都已經在這個 repo 發生過：
 *
 *   - `Record<Union, …>`：契約多一個值 → `LABEL[x]` 取到 `undefined` → **畫面印出
 *     一個空白**，正是設計 §2.1 禁止的那一格。
 *   - `Record<string, string>` ＋ `?? raw`：契約多一個值 → **英文 enum 直接出現在
 *     中文句子裡**，違反 `02:NFR-007` 第 3 條。
 *
 * 一張平鋪的清單而不是一層抽象：把六行複製十五次，凌晨三點讀起來比一個掃描登記表
 * 的巧妙迴圈便宜，而且每一列都指名它守的是哪一個畫面。
 */
const LABEL_TABLES: Array<{
  what: string;
  values: Record<string, string>;
  table: () => Promise<Record<string, unknown>>;
}> = [
  {
    what: "GenerationFailure.failure → 一句失敗說明 (GEN-003)",
    values: generated.GenerationFailureFailureEnum,
    table: async () => (await import("./components/generateFailureSentence")).FAILURE_SENTENCE,
  },
  {
    what: "Skill.redistribution → 打包閘門 (Packaging)",
    values: generated.SkillRedistributionEnum,
    table: async () => (await import("./pages/Packaging")).REDISTRIBUTION_GATE,
  },
  {
    what: "OwnSkill.redistribution → 我的 Skill 的徽章",
    values: generated.OwnSkillRedistributionEnum,
    table: async () => (await import("./pages/WorkspaceSkills")).REDISTRIBUTION_BADGE,
  },
  {
    what: "Run.status → 執行狀態措辭 (ADR-025)",
    values: generated.RunStatusEnum,
    table: async () => (await import("./pages/RunEvaluation")).RUN_STATUS_LABEL,
  },
  {
    what: "Evaluation.overall → 任務判定",
    values: generated.EvaluationOverallEnum,
    table: async () => (await import("./pages/RunEvaluation")).OVERALL_LABEL,
  },
  {
    what: "CriterionResult.result → 逐條判定",
    values: generated.CriterionResultResultEnum,
    table: async () => (await import("./pages/RunEvaluation")).CRITERION_LABEL,
  },
  {
    what: "CriterionResult.source → 判定來源",
    values: generated.CriterionResultSourceEnum,
    table: async () => (await import("./pages/RunEvaluation")).SOURCE_LABEL,
  },
  {
    what: "DeterministicFinding.category → 發現分類",
    values: generated.DeterministicFindingCategoryEnum,
    table: async () => (await import("./pages/RunEvaluation")).FINDING_CATEGORY_LABEL,
  },
  {
    what: "DeterministicFinding.severity → 嚴重度",
    values: generated.DeterministicFindingSeverityEnum,
    table: async () => (await import("./pages/RunEvaluation")).SEVERITY_LABEL,
  },
  {
    what: "ImprovementSuggestion.category → 建議分類",
    values: generated.ImprovementSuggestionCategoryEnum,
    table: async () => (await import("./pages/RunEvaluation")).SUGGESTION_CATEGORY_LABEL,
  },
  {
    what: "EvidenceRef.match → 引文回驗說明 (ADR-043)",
    values: generated.EvidenceRefMatchEnum,
    table: async () => (await import("./pages/RunEvaluation")).MATCH_NOTE,
  },
  {
    what: "EvidenceRef.kind → 證據種類",
    values: generated.EvidenceRefKindEnum,
    table: async () => (await import("./pages/RunEvaluation")).KIND_WORD,
  },
  {
    what: "RunPermissionSummary.content.scripts.status → Script 揭露",
    values: generated.RunPermissionSummaryContentScriptsStatusEnum,
    table: async () => (await import("./pages/RunPreflight")).SCRIPT_LABEL,
  },
  {
    what: "SkillLicense.source → License 出處",
    values: generated.SkillLicenseSourceEnum,
    table: async () => (await import("./components/LicenseBadge")).SOURCE_LABELS,
  },
  {
    what: "SubmitFeedbackRequest.kind → 回報種類 (BETA-004/005)",
    values: generated.SubmitFeedbackRequestKindEnum,
    table: async () => (await import("./components/FeedbackEntry")).KIND_LABEL,
  },
  {
    what: "SubmitFeedbackRequest.kind → 回報種類的例子",
    values: generated.SubmitFeedbackRequestKindEnum,
    table: async () => (await import("./components/FeedbackEntry")).KIND_NOTE,
  },
];

for (const { what, values, table } of LABEL_TABLES) {
  test(`04 丙-43: every contract value has a label — ${what}`, async () => {
    const labels = await table();
    const contract = Object.values(values);
    expect(contract.length, `${what}: the generated enum is empty`).toBeGreaterThan(0);
    for (const value of contract) {
      const label = labels[value];
      expect(
        label,
        `${what}: no entry for ${JSON.stringify(value)} — the contract has it and this ` +
          `table does not, so the screen renders a blank or the raw enum`,
      ).not.toBeUndefined();
      // `null` is a legal entry where the table maps to a gate rather than to
      // copy (REDISTRIBUTION_GATE's 「不擋」); an empty string never is.
      if (typeof label === "string") expect(label.length).toBeGreaterThan(0);
    }
  });
}

/**
 * The tables keyed on `value` with a documented fallback rather than on a closed
 * union. 「Every value has a row」 is not their rule — `RunVerdict` deliberately
 * has two rows and four states (§5.3 records that trade). What IS their rule is
 * the other direction: a row for a value the contract never sends is copy no
 * reader can ever reach, and it is how a table quietly stops describing the
 * thing it is named after.
 */
const FALLBACK_TABLES: Array<{
  what: string;
  values: string[];
  table: () => Promise<Record<string, unknown>>;
}> = [
  {
    what: "Evaluation 判定徽章 (RunVerdict)",
    values: Object.values(generated.EvaluationOverallEnum),
    table: async () => (await import("./components/RunVerdict")).VERDICT_BADGE,
  },
  {
    what: "相容性三軸的色調 (CompatibilityStatus)",
    // Three axes, three enums, one tint table — `spec_validation` and `runtime`
    // are CompatibilityResult, `capability` is AgentCapability.
    values: ["unverified", "passed", "failed", "activated", "not_activated"],
    table: async () => (await import("./components/CompatibilityStatus")).BADGE_TINT,
  },
  {
    what: "Run.cleanup_status → 清理狀態的色調 (WorkspaceRuns)",
    values: ["pending", "cleaning_up", "cleaned", "failed"],
    table: async () => (await import("./pages/WorkspaceRuns")).CLEANUP_BADGE,
  },
];

for (const { what, values, table } of FALLBACK_TABLES) {
  test(`04 丙-43: no row for a value the contract never sends — ${what}`, async () => {
    const labels = await table();
    const contract = new Set(values);
    expect(
      Object.keys(labels).filter((k) => !contract.has(k)),
      `${what}: a row keyed on a value the contract does not have — dead copy`,
    ).toEqual([]);
  });
}

// --- 3. the three subsets of RunStatus --------------------------------------

/**
 * The assertion that closes three tables at once.
 *
 * `CANCELLABLE`（＝ `IN_FLIGHT_RUN_STATUSES`）and `TERMINAL_RUN_STATUSES` used to
 * be two independently hand-written sets whose union happened to equal the
 * contract. A tenth status in neither meant `InFlight` never disappeared and
 * `useTrace` polled every three seconds forever — with every existing test
 * green. `TERMINAL` is derived as the complement now, so this is really an
 * assertion about `RUN_STATUSES` itself.
 */
test("04 丙-43: 進行中 ∪ 終態 is exactly the contract's RunStatus", async () => {
  const { IN_FLIGHT_RUN_STATUSES, TERMINAL_RUN_STATUSES, RUN_STATUSES } =
    await import("./api/trace");
  const contract = new Set<string>(Object.values(generated.RunStatusEnum));

  expect(new Set<string>(RUN_STATUSES), "RUN_STATUSES has drifted from the contract").toEqual(
    contract,
  );
  expect(
    new Set([...IN_FLIGHT_RUN_STATUSES, ...TERMINAL_RUN_STATUSES]),
    "a status that is neither in flight nor terminal, or one that is both",
  ).toEqual(contract);
  // Disjoint, which is the half a union assertion cannot see: a status in both
  // sets would keep `InFlight` on screen for a finished run.
  expect(
    [...IN_FLIGHT_RUN_STATUSES].filter((s) => TERMINAL_RUN_STATUSES.has(s)),
    "a status that is both in flight and terminal",
  ).toEqual([]);
});
