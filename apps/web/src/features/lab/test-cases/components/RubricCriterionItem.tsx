import type { RubricItem } from "../../testcases.service";

export function RubricCriterionItem({
  criterionId,
  criterionText,
  item,
  onUpdate,
}: {
  criterionId: string;
  criterionText: string;
  item: RubricItem | undefined;
  onUpdate: (patch: Partial<RubricItem>) => void;
}) {
  return (
    <li className="criterion">
      <p className="note">驗收條件：{criterionText}</p>
      <label htmlFor={`rubric-${criterionId}`} className="note">
        這一條的 rubric 說明（留空＝這條沒有 rubric）
      </label>
      <br />
      <textarea
        id={`rubric-${criterionId}`}
        rows={3}
        cols={60}
        maxLength={2000}
        value={item?.text ?? ""}
        onChange={(e) => onUpdate({ text: e.target.value })}
      />
      <p>
        <label htmlFor={`rubric-weight-${criterionId}`}>權重</label>{" "}
        <input
          id={`rubric-weight-${criterionId}`}
          type="number"
          min={0}
          step={1}
          size={4}
          value={item?.weight ?? ""}
          onChange={(e) =>
            onUpdate({
              weight: e.target.value === "" ? undefined : Number(e.target.value),
            })
          }
        />{" "}
        <label htmlFor={`rubric-evidence-${criterionId}`}>
          <input
            id={`rubric-evidence-${criterionId}`}
            type="checkbox"
            checked={item?.evidence_required ?? false}
            onChange={(e) => onUpdate({ evidence_required: e.target.checked })}
          />{" "}
          要求引出原文
        </label>
      </p>
    </li>
  );
}
