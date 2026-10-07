import { Link } from "@tanstack/react-router";
import {
  usd,
  usePlatformAgentSteps,
  type PlatformAgentRun,
  type PlatformAgentRunStatus,
} from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Reveal } from "../../../../shared/ui/Reveal";
import { Timestamp } from "../../../../shared/ui/Timestamp";

const RUN_STATUS: Record<PlatformAgentRunStatus, string> = {
  running: "執行中",
  completed: "完成",
  incomplete: "未完成",
  stopped: "已停止",
  failed: "失敗",
};

function runCost(run: PlatformAgentRun): string {
  const known = usd(run.usd_micros);
  return run.unpriced_steps > 0 ? `${known}（另有 ${run.unpriced_steps} 步沒有回報花費）` : known;
}

export function AgentRunList({ runs }: { runs: PlatformAgentRun[] }) {
  if (runs.length === 0) return <p>還沒有任何執行：0 次。</p>;
  return (
    <ul className="download-list">
      {runs.map((run) => (
        <li className="download-item" key={run.id}>
          <p>
            <strong>{run.agent}</strong>
          </p>
          <p className="badge-row">
            <span className={run.status === "completed" ? "badge" : "badge badge-danger"}>
              {RUN_STATUS[run.status]}
            </span>
          </p>
          {run.reason && <p>原因：{run.reason}</p>}
          <p className="note">
            開始於 <Timestamp at={run.started_at} />；{run.steps} 步；花費 {runCost(run)}
          </p>
          <p>
            <Link to="/admin/agents" search={{ run: run.id }}>
              看這次的步驟
            </Link>
          </p>
        </li>
      ))}
    </ul>
  );
}

export function AgentRunSteps({ run }: { run: string }) {
  const steps = usePlatformAgentSteps(run);
  return (
    <>
      <h2>這次執行的步驟</h2>
      {steps.isPending && <Loading what="執行步驟" />}
      <ReadFailure error={steps.error} what="執行步驟" />
      {steps.data && steps.data.steps.length === 0 && <p>這次執行沒有任何步驟：0 步。</p>}
      {steps.data && steps.data.steps.length > 0 && (
        <ol className="download-list">
          {steps.data.steps.map((step) => (
            <li className="download-item" key={step.seq}>
              <p>
                <strong>{step.tool === "finish" ? "交出結果" : `呼叫 ${step.tool}`}</strong>
              </p>
              <p className="note">
                模型 {step.model}；輸入 {step.prompt_tokens} tokens、輸出 {step.completion_tokens}{" "}
                tokens；花費 {step.usd_micros === undefined ? "沒有回報" : usd(step.usd_micros)}
              </p>
              <pre>
                <Reveal text={step.arguments} />
              </pre>
              {step.result !== "" && (
                <pre>
                  <Reveal text={step.result} />
                </pre>
              )}
            </li>
          ))}
        </ol>
      )}
    </>
  );
}
