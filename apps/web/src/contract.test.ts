import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";
import * as generated from "@skillhub/api-client-ts";

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

const camel = (snake: string) => snake.replace(/_([a-z0-9])/g, (_, c: string) => c.toUpperCase());

type Fields = Map<string, boolean>;

function fieldsOf(body: string, name: string): Fields | null {
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
    .slice(open + 1, end)
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\/\/[^\n]*/g, "");

  const fields: Fields = new Map();
  let nest = 0;
  for (const line of inner.split("\n")) {
    const trimmed = line.trim();
    if (nest === 0 && !trimmed.startsWith("[")) {
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

function handWritten(name: string, types: string): Fields | null {
  const own = fieldsOf(types, name);
  if (!own) return null;
  const ext = new RegExp(`export interface ${name} extends ([\\w, ]+)\\s*\\{`).exec(types);
  if (!ext) return own;
  const merged: Fields = new Map(own);
  for (const parent of ext[1].split(",").map((p) => p.trim())) {
    const inherited = handWritten(parent, types);
    if (inherited) for (const [k, v] of inherited) if (!merged.has(k)) merged.set(k, v);
  }
  return merged;
}

test("鐵律 12: every hand-written interface with a generated twin has the same fields", () => {
  const types = [
    readFileSync(join(src, "api", "types.ts"), "utf8"),
    readFileSync(join(src, "api", "import.ts"), "utf8"),
    readFileSync(join(src, "api", "creation.ts"), "utf8"),
    readFileSync(join(src, "api", "lab.ts"), "utf8"),
    readFileSync(join(src, "api", "packaging.ts"), "utf8"),
  ].join("\n");
  const names = [...types.matchAll(/^export interface (\w+)/gm)].map((m) => m[1]);
  expect(names.length, "the api/*.ts scan parsed no interfaces — the scan broke").toBeGreaterThan(
    20,
  );

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
        problems.push(
          `${name}.${field} is in the contract and missing from the hand-written api/*.ts`,
        );
      }
    }
  }

  expect(compared, "no interface was actually compared — the name match broke").toBeGreaterThan(15);
  expect(problems.sort(), "api/types.ts and the generated client disagree").toEqual([]);
});

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
      if (typeof label === "string") expect(label.length).toBeGreaterThan(0);
    }
  });
}

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
  expect(
    [...IN_FLIGHT_RUN_STATUSES].filter((s) => TERMINAL_RUN_STATUSES.has(s)),
    "a status that is both in flight and terminal",
  ).toEqual([]);
});
