import type { SkillCompatibility } from "../api/types";
import { StateIcon, type IconState } from "./StateIcon";
import { Timestamp } from "./Timestamp";

export const BADGE_TINT: Record<string, string> = {
  unverified: "unverified",
  not_activated: "failed",
  failed: "failed",
};

function axisIconState(value: string, tint?: string): IconState | undefined {
  if (tint === "unverified") return "unknown";
  if (tint === "failed") return "fail";
  if (value === "passed") return "pass";
  return undefined;
}

export function CompatibilityStatus({ compatibility }: { compatibility: SkillCompatibility }) {
  const axes = [
    { key: "spec_validation", label: "規格驗證", axis: compatibility.spec_validation },
    { key: "capability", label: "能力相容", axis: compatibility.capability },
    { key: "runtime", label: "執行環境相容", axis: compatibility.runtime },
  ];

  return (
    <div>
      <ul className="compat-list">
        {axes.map(({ key, label, axis }) => {
          const iconState = axisIconState(axis.value, BADGE_TINT[axis.value]);
          return (
            <li
              key={key}
              className={
                BADGE_TINT[axis.value] ? `badge badge-compat-${BADGE_TINT[axis.value]}` : "badge"
              }
            >
              {iconState && <StateIcon state={iconState} />}
              {label}：{axis.label}
            </li>
          );
        })}
      </ul>
      {axes
        .filter(({ axis }) => axis.note !== "")
        .map(({ key, label, axis }) => (
          <p key={key} className="note">
            {label}：{axis.note}
          </p>
        ))}
      {compatibility.runtime_image && (
        <p className="note">
          實測環境：<code>{compatibility.runtime_image}</code>
          {compatibility.measured_at ? (
            <>
              （<Timestamp at={compatibility.measured_at} />）
            </>
          ) : (
            ""
          )}
        </p>
      )}
      <p className="note">{compatibility.note}</p>
    </div>
  );
}
