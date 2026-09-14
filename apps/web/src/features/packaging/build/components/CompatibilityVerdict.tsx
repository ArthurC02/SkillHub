import type { SkillCompatibility } from "../../../../core/api/types";

export function CompatibilityVerdict({ compatibility }: { compatibility: SkillCompatibility }) {
  const axes: Array<[string, string]> = [
    ["規格驗證", compatibility.spec_validation.label],
    ["能力相容", compatibility.capability.label],
    ["執行環境相容", compatibility.runtime.label],
  ];
  return (
    <p className="compat-list">{axes.map(([label, value]) => `${label}：${value}`).join("／")}</p>
  );
}
