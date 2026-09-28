import type { ComparisonSide } from "../../evaluation.service";

export const SIDE_LABEL = ["這一邊", "另一邊"];

export function costNote(side: ComparisonSide): string {
  return `${side.cost.is_lower_bound ? "這是下界，不是總額。" : ""}權威來源：${
    side.cost.authoritative_source
  }`;
}

export function credits(value: number | null): string {
  return value === null ? "未測量" : `${value} 點`;
}
