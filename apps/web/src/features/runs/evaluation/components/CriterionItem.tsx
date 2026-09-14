import { StateIcon, type IconState } from "../../../../shared/ui/StateIcon";
import type { CriterionResult } from "../../evaluation.service";
import { CRITERION_LABEL, isEvidenceUnverifiable, SOURCE_LABEL } from "../evaluation.model";
import { EvidenceList } from "./EvidenceList";

export function CriterionItem({
  criterion: c,
  listSource: shared,
}: {
  criterion: CriterionResult;
  listSource?: CriterionResult["source"];
}) {
  const downgraded = isEvidenceUnverifiable(c);
  const criterionIconState: IconState = downgraded
    ? "degraded"
    : c.result === "passed"
      ? "pass"
      : c.result === "failed"
        ? "fail"
        : "unknown";

  return (
    <li className={`criterion criterion-${c.result}${downgraded ? " criterion-unverifiable" : ""}`}>
      <p>
        <span
          className={`badge badge-criterion-${c.result}${
            downgraded ? " badge-criterion-unverifiable" : ""
          }`}
        >
          <StateIcon state={criterionIconState} />
          {downgraded ? "證據無法回驗" : CRITERION_LABEL[c.result]}
        </span>{" "}
        {c.text}
      </p>
      {downgraded ? (
        <p className="note">
          判定來源：平台降級（模型原本有結論，但它引用的證據在平台資料裡對不上，因此不採信）。
          <strong>這不是「模型自己說不知道」</strong>
          ——那一種會顯示為「無法判斷」。這一條要查的是引用為什麼回驗不過，不是模型有沒有把握。
        </p>
      ) : (
        c.source !== shared && <p className="note">判定來源：{SOURCE_LABEL[c.source]}</p>
      )}
      {c.reason && <p>{c.reason}</p>}
      <EvidenceList evidence={c.evidence} />
    </li>
  );
}
