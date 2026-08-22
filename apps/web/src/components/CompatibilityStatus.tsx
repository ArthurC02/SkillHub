import type { SkillCompatibility } from "../api/types";

/**
 * The three DISC-008 axes, kept apart because they answer different questions
 * and have different sources: spec validation is static analysis of the package,
 * the other two are a sandbox measurement (0022). An axis with no answer renders
 * an explicit 未驗證 rather than being hidden, because a missing row reads as
 * "fine" and 未驗證 does not.
 *
 * **The words come from the server** (04 丙-29 ③, 設計 §4.4). There were three
 * label tables here and one of them lived in `Home.tsx` as
 * `spec_validation === "passed" ? "通過" : "未驗證"` — three values collapsed into
 * two, so a `failed` spec read as 未驗證 on the row a reader meets first while
 * this view called it 未通過. One authority per fact, and it is not this file.
 */

/**
 * Badge tint, and only for the states that have one. §4.4 gives the two tints
 * a specific meaning each — `--danger` = 不通過／會擋住你, `--accent-border` =
 * 未知／未驗證 — and `transpiled` is neither: the sandbox did run the Skill and
 * this is the answer it measured (0022 got it for 33 of 45). Painting a measured
 * outcome with the 未知 border made the colour contradict the word, which is
 * §2.3 backwards. It takes the untinted `.badge` and lets its own label carry
 * the caveat, which is why the server spells that one out.
 *
 * Keyed by `value`, which is the half of `Labelled` that stays stable. The tint
 * is presentation and belongs here; the words do not.
 */
const BADGE_TINT: Record<string, string> = {
  unverified: "unverified",
  not_activated: "failed",
  failed: "failed",
};

export function CompatibilityStatus({ compatibility }: { compatibility: SkillCompatibility }) {
  const axes = [
    { key: "spec_validation", label: "規格驗證", axis: compatibility.spec_validation },
    { key: "capability", label: "能力相容", axis: compatibility.capability },
    // Not 實測相容 any more. That axis is a rule about the image, not an
    // observation of anything running (backfill-agent-compatibility.sql), and an
    // axis called 實測 sitting next to a measured_at timestamp was the strongest
    // claim on this block and the least earned one.
    { key: "runtime", label: "執行環境相容", axis: compatibility.runtime },
  ];

  return (
    <div>
      {/* Each row IS the badge, so .compat-list turns off the flex `stretch`
          that ran these pills the full width of the page. */}
      <ul className="compat-list">
        {axes.map(({ key, label, axis }) => (
          <li
            key={key}
            className={
              BADGE_TINT[axis.value] ? `badge badge-compat-${BADGE_TINT[axis.value]}` : "badge"
            }
          >
            {label}：{axis.label}
          </li>
        ))}
      </ul>
      {/*
        Only the axes that carry one. The server leaves `note` empty for the
        states the block note below already covers, so this renders a line
        exactly where the label alone would be misread — `transpiled` above all,
        where the Run worked and the work was not the Skill's own code. Repeating
        the block note per axis would be checklist 第 14 條's failure.
      */}
      {axes
        .filter(({ axis }) => axis.note !== "")
        .map(({ key, label, axis }) => (
          <p key={key} className="note">
            {label}：{axis.note}
          </p>
        ))}
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
