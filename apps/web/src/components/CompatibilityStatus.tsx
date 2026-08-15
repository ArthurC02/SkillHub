import type { CompatibilityResult, SkillCompatibility } from "../api/types";

/**
 * The three DISC-008 axes, kept apart. Only spec validation has an answer
 * before M2; the other two render an explicit 未驗證 rather than being hidden,
 * because a missing row reads as "fine" and 未驗證 does not.
 */
const RESULT_LABELS: Record<CompatibilityResult, string> = {
  unverified: "未驗證",
  passed: "通過",
  failed: "未通過",
};

const AXES: Array<{ key: keyof SkillCompatibility; label: string }> = [
  { key: "spec_validation", label: "規格驗證" },
  { key: "capability", label: "能力相容" },
  { key: "runtime", label: "實測相容" },
];

export function CompatibilityStatus({ compatibility }: { compatibility: SkillCompatibility }) {
  return (
    <div>
      <ul className="compat-list">
        {AXES.map(({ key, label }) => {
          const status = compatibility[key] as CompatibilityResult;
          return (
            <li key={key} className={`badge badge-compat-${status}`}>
              {label}：{RESULT_LABELS[status]}
            </li>
          );
        })}
      </ul>
      <p className="note">{compatibility.note}</p>
    </div>
  );
}
