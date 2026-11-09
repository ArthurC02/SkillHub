import type { Absence } from "./ui/absence";

const UNMEASURED: Absence = "未測量";

type Overall = "met" | "partially_met" | "not_met" | "undetermined";
type CriterionOutcome = "passed" | "failed" | "undetermined";

export const OVERALL_LABEL: Record<Overall, string> = {
  met: "符合",
  partially_met: "部分符合",
  not_met: "未符合",
  undetermined: "無法判斷",
};

export const CRITERION_LABEL: Record<CriterionOutcome, string> = {
  passed: "通過",
  failed: "未通過",
  undetermined: "無法判斷",
};

export function labelOf(
  table: Readonly<Record<string, string>>,
  value: string,
): string | undefined {
  return Object.hasOwn(table, value) ? table[value] : undefined;
}

export function verdictWord(table: Readonly<Record<string, string>>, value?: string): string {
  return labelOf(table, value ?? "") ?? (value || UNMEASURED);
}

export function criterionTally(results: readonly { result?: string }[]): string {
  const count = (r: CriterionOutcome) => results.filter((x) => x.result === r).length;
  return (["passed", "failed", "undetermined"] as const)
    .map((r) => `${CRITERION_LABEL[r]} ${count(r)}`)
    .join("／");
}
