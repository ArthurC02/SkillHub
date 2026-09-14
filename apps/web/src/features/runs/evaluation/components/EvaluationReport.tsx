import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { Evaluation } from "../../evaluation.service";
import { OVERALL_LABEL, FINDING_CATEGORY_LABEL, SEVERITY_LABEL } from "../evaluation.model";
import { ExecutionState } from "./ExecutionState";
import { CriterionSection } from "./CriterionSection";
import { MatchLegend } from "./MatchLegend";
import { EvidenceList } from "./EvidenceList";

function credits(value: number | null): string {
  return value === null ? "未測量" : `${value} 點`;
}

export function EvaluationReport({
  evaluation,
  runStatus,
}: {
  evaluation: Evaluation;
  runStatus?: string;
}) {
  return (
    <div>
      {evaluation.superseded_at && (
        <p className="notice">
          你正在看歷史判定，它已於 <Timestamp at={evaluation.superseded_at} /> 被較新的評估取代。
        </p>
      )}

      {evaluation.status === "failed" && (
        <p className="notice">
          <strong>評估未完成</strong>：這次判定沒有跑完（例如模型閘道不可用或證據讀不到）。
          這與「未評估」不同，也不會被當成通過。
        </p>
      )}
      {evaluation.status === "pending" && <p className="notice">評估進行中，以下結果尚未定案。</p>}

      <p className="verdict">
        任務判定：<strong>{OVERALL_LABEL[evaluation.overall]}</strong>
      </p>
      {runStatus && <ExecutionState runStatus={runStatus} />}
      {evaluation.summary && <p>{evaluation.summary}</p>}

      {!evaluation.evidence_complete && (
        <p className="notice">
          判定所依據的材料不完整（Trace 有缺漏、Artifact 讀不到，或輸入被截斷）。
          在這個前提下，逐條判定不會記為通過。
        </p>
      )}

      <h3>逐條驗收條件</h3>
      {evaluation.criterion_results.length === 0 ? (
        <p>這次評估沒有逐條結果。</p>
      ) : (
        <CriterionSection results={evaluation.criterion_results} />
      )}

      <h3>這個 Run 的問題（六類）</h3>
      {evaluation.deterministic_findings.length === 0 ? (
        <p>沒有列出問題。這不等於一切正常，只表示這些檢查沒有產生發現。</p>
      ) : (
        <>
          <MatchLegend evidence={evaluation.deterministic_findings.flatMap((f) => f.evidence)} />
          <ul className="finding-list">
            {evaluation.deterministic_findings.map((f, i) => (
              <li className="criterion" key={`${f.category}-${i}`}>
                <p>
                  <span className="badge">{FINDING_CATEGORY_LABEL[f.category]}</span>{" "}
                  <span className={`badge badge-severity-${f.severity}`}>
                    {SEVERITY_LABEL[f.severity]}
                  </span>{" "}
                  {f.message}
                </p>
                <EvidenceList evidence={f.evidence} />
              </li>
            ))}
          </ul>
        </>
      )}

      <h3>評估本身用掉的點數</h3>
      <p>
        {credits(evaluation.cost.evaluation_credits)}
        {evaluation.cost.source === "gateway" && "（模型閘道實付）"}
        {evaluation.cost.source === "estimated" && "（估算值）"}
      </p>
      <p className="note">
        {evaluation.cost.note}
        {" 這是平台判定用掉的點數，與 Run 自己用掉的分開列，不相加。"}
      </p>

      <details>
        <summary>判定資訊（Judge 模型與版本）</summary>
        <ul className="note">
          {evaluation.judge_model ? (
            <>
              <li>Judge 模型：{evaluation.judge_model}</li>
              <li>Judge prompt 版本：{evaluation.judge_prompt_version}</li>
            </>
          ) : (
            <li>
              Judge：這次沒有跑（沒有驗收條件，或平台沒有設定 Judge），所以沒有模型與 prompt
              版本可記
            </li>
          )}
          <li>Rubric 版本：{evaluation.rubric_version ?? "無 rubric（不是採用預設 rubric）"}</li>
          <li>
            評估時間：
            <Timestamp at={evaluation.evaluated_at} />
          </li>
        </ul>
      </details>
    </div>
  );
}
