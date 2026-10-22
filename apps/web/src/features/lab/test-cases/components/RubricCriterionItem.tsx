import { RubricItemTextMaxLength } from "@skillhub/api-client-ts/limits";
import type { RubricItem } from "../../testcases.service";
import { textLengthReason } from "../test-cases.model";
import { useRevealOnChange } from "../../../../shared/ui/useRevealOnChange";

export function RubricCriterionItem({
  criterionId,
  criterionText,
  item,
  onUpdate,
  pending,
}: {
  criterionId: string;
  criterionText: string;
  item: RubricItem | undefined;
  onUpdate: (patch: Partial<RubricItem>) => void;
  pending: boolean;
}) {
  const text = item?.text ?? "";
  const problem =
    text.trim() === "" ? null : textLengthReason(text, "Rubric 說明", RubricItemTextMaxLength);
  const problemNote = useRevealOnChange<HTMLParagraphElement>(problem);
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
        value={text}
        aria-invalid={problem !== null}
        aria-describedby={problem ? `rubric-size-${criterionId}` : undefined}
        onChange={(e) => onUpdate({ text: e.target.value })}
        readOnly={pending}
      />
      {problem && (
        <p className="note" role="status" id={`rubric-size-${criterionId}`} ref={problemNote}>
          {problem}
        </p>
      )}
      <p>
        <label htmlFor={`rubric-weight-${criterionId}`}>權重</label>{" "}
        <input
          id={`rubric-weight-${criterionId}`}
          type="number"
          min={0}
          step={1}
          size={4}
          value={item?.weight ?? ""}
          readOnly={pending}
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
            disabled={pending}
            onChange={(e) => onUpdate({ evidence_required: e.target.checked })}
          />{" "}
          要求引出原文
        </label>
      </p>
    </li>
  );
}
