import { formatAt } from "../../../../shared/ui/Timestamp.model";
import type { EvaluationRevision } from "../../evaluation.service";
import { OVERALL_LABEL } from "../evaluation.model";

export function RevisionPicker({
  revisions,
  value,
  onChange,
}: {
  revisions: EvaluationRevision[];
  value: string;
  onChange: (revisionId: string | undefined) => void;
}) {
  return (
    <p>
      <label htmlFor="evaluation-revision">評估版本</label>{" "}
      <select
        id="evaluation-revision"
        value={value}
        onChange={(e) => onChange(e.target.value === "" ? undefined : e.target.value)}
      >
        <option value="">目前的判定</option>
        {revisions.map((r) => (
          <option key={r.evaluation_id} value={r.evaluation_id}>
            {r.evaluated_at ? formatAt(r.evaluated_at) : "評估中"}｜{OVERALL_LABEL[r.overall]}
            ｜prompt {r.judge_prompt_version}
            {r.rubric_version ? `｜rubric ${r.rubric_version}` : ""}
            {r.superseded_at ? "（已被取代）" : ""}
          </option>
        ))}
      </select>
    </p>
  );
}
