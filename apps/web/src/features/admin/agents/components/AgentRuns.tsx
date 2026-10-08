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
import "./AgentRuns.css";

const RUN_STATUS: Record<PlatformAgentRunStatus, string> = {
  running: "執行中",
  completed: "完成",
  incomplete: "未完成",
  stopped: "已停止",
  failed: "失敗",
};

const RUN_TONE: Record<PlatformAgentRunStatus, string> = {
  running: "badge",
  completed: "badge",
  incomplete: "badge badge-warning",
  stopped: "badge badge-warning",
  failed: "badge badge-danger",
};

function runCost(run: PlatformAgentRun): string {
  const known = usd(run.usd_micros);
  return run.unpriced_steps > 0 ? `${known}（另有 ${run.unpriced_steps} 步沒有回報花費）` : known;
}

export function AgentRunFacts({ run }: { run: PlatformAgentRun }) {
  return (
    <>
      <p className="badge-row">
        <span className={RUN_TONE[run.status]}>{RUN_STATUS[run.status]}</span>
      </p>
      {run.reason && <p>原因：{run.reason}</p>}
      <p className="note">
        開始於 <Timestamp at={run.started_at} />；{run.steps} 步；花費 {runCost(run)}
      </p>
    </>
  );
}

export function AgentRunList({ runs, total }: { runs: PlatformAgentRun[]; total: number }) {
  if (runs.length === 0) return <p>還沒有任何執行：0 次。</p>;
  return (
    <>
      {total > runs.length && (
        <p className="note">
          共 {total} 次；目前顯示最近 {runs.length} 次，這份清單最多顯示 50 次。
        </p>
      )}
      <ul className="download-list agent-run-list">
        {runs.map((run) => (
          <li className="download-item" key={run.id}>
            <p>
              <strong>{run.agent}</strong>
            </p>
            <AgentRunFacts run={run} />
            {run.status === "running" && (
              <p className="note">
                {run.last_step_at ? (
                  <>
                    最近一步記錄於 <Timestamp at={run.last_step_at} relative />。
                  </>
                ) : (
                  "尚未記錄第一步。"
                )}
              </p>
            )}
            <p>
              <Link to="/admin/agents" search={{ run: run.id }}>
                看這次的步驟
              </Link>
            </p>
          </li>
        ))}
      </ul>
    </>
  );
}

export function AgentRunSteps({ run, live }: { run: string; live: boolean }) {
  const steps = usePlatformAgentSteps(run, live);
  const latest = steps.data?.steps.at(-1);
  return (
    <>
      <h3>這次執行的步驟</h3>
      {steps.isPending && <Loading what="執行步驟" />}
      <ReadFailure error={steps.error} what="執行步驟" />
      {live && steps.data && !steps.error && latest && (
        <p className="note">
          最近一步記錄於 <Timestamp at={latest.created_at} relative />。
        </p>
      )}
      {steps.data && !steps.error && steps.data.steps.length === 0 && (
        <p>這次執行沒有任何步驟：0 步。</p>
      )}
      {steps.data && !steps.error && steps.data.steps.length > 0 && (
        <ol className="download-list">
          {steps.data.steps.map((step) => (
            <li className="download-item" key={step.seq}>
              <p>
                <strong>{step.tool === "finish" ? "交出結果" : `呼叫 ${step.tool}`}</strong>
              </p>
              <p className="note">
                模型 {step.model}；輸入 {step.prompt_tokens} tokens、輸出 {step.completion_tokens}{" "}
                tokens；花費 {step.usd_micros === undefined ? "沒有回報" : usd(step.usd_micros)}；
                記錄於 <Timestamp at={step.created_at} />
              </p>
              <details className="agent-step-raw">
                <summary>{step.result === "" ? "查看輸入" : "查看輸入與結果"}</summary>
                <p>輸入</p>
                <pre className="agent-step-text">
                  <Reveal text={step.arguments} />
                </pre>
                {step.result !== "" && (
                  <>
                    <p>結果</p>
                    <pre className="agent-step-text">
                      <Reveal text={step.result} />
                    </pre>
                  </>
                )}
              </details>
            </li>
          ))}
        </ol>
      )}
    </>
  );
}
