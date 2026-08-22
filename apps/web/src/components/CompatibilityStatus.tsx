import type {
  AgentCapability,
  AgentRuntime,
  CompatibilityResult,
  SkillCompatibility,
} from "../api/types";

/**
 * The three DISC-008 axes, kept apart because they answer different questions
 * and have different sources: spec validation is static analysis of the package,
 * the other two are a sandbox measurement (0022). An axis with no answer renders
 * an explicit 未驗證 rather than being hidden, because a missing row reads as
 * "fine" and 未驗證 does not.
 */
const SPEC_LABELS: Record<CompatibilityResult, string> = {
  unverified: "未驗證",
  passed: "通過",
  failed: "未通過",
};

const CAPABILITY_LABELS: Record<AgentCapability, string> = {
  unverified: "未驗證",
  activated: "已啟用",
  not_activated: "未被啟用",
};

/**
 * 模型轉譯 is spelled out rather than shortened. It is the case a reader will
 * not guess from a badge, and it is the one that changes what they are choosing:
 * the Skill worked, but its own script never ran.
 */
const RUNTIME_LABELS: Record<AgentRuntime, string> = {
  unverified: "未驗證",
  native: "腳本可直接執行",
  transpiled: "腳本未執行，由模型轉譯",
  failed: "腳本無法執行",
};

/** Badge colouring: the three states that are not a clean pass share the styles. */
const BADGE_STATE: Record<string, string> = {
  unverified: "unverified",
  not_activated: "failed",
  failed: "failed",
  transpiled: "unverified",
};

export function CompatibilityStatus({ compatibility }: { compatibility: SkillCompatibility }) {
  const axes = [
    {
      key: "spec_validation",
      label: "規格驗證",
      value: compatibility.spec_validation as string,
      text: SPEC_LABELS[compatibility.spec_validation],
    },
    {
      key: "capability",
      label: "能力相容",
      value: compatibility.capability as string,
      text: CAPABILITY_LABELS[compatibility.capability],
    },
    {
      key: "runtime",
      label: "實測相容",
      value: compatibility.runtime as string,
      text: RUNTIME_LABELS[compatibility.runtime],
    },
  ];

  return (
    <div>
      {/* Each row IS the badge, so .compat-list turns off the flex `stretch`
          that ran these pills the full width of the page. */}
      <ul className="compat-list">
        {axes.map(({ key, label, value, text }) => (
          <li key={key} className={`badge badge-compat-${BADGE_STATE[value] ?? "passed"}`}>
            {label}：{text}
          </li>
        ))}
      </ul>
      {/*
        The image is shown with the verdict, never apart from it: the two sandbox
        axes are a measurement of a (version, image) pair, and the same package
        answers differently on an image carrying a different set of interpreters.
      */}
      {compatibility.runtime_image && (
        <p className="note">
          實測環境：<code>{compatibility.runtime_image}</code>
          {compatibility.measured_at ? `（${compatibility.measured_at}）` : ""}
        </p>
      )}
      <p className="note">{compatibility.note}</p>
    </div>
  );
}
