import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import { useRun } from "../runs.service";
import { InFlight } from "./components/InFlight";
import { useTrace } from "../trace.service";
import { EvaluationPanel } from "../evaluation/EvaluationPanel";
import { CANCELLABLE, CancelRunControl } from "./components/CancelRunControl";
import { RunArtifacts } from "./components/RunArtifacts";
import { GeneralMode } from "./components/GeneralMode";
import { AdvancedMode } from "./components/AdvancedMode";

export function RunTrace() {
  const { runId } = useParams({ from: "/runs/$runId" });
  const { events: linkedEvents } = useSearch({ strict: false }) as { events?: string };
  const [mode, setMode] = useState<"general" | "advanced">(linkedEvents ? "advanced" : "general");
  const run = useRun(runId);
  const general = useTrace(runId, "general");

  return (
    <section>
      <h1>Run 結果</h1>
      <ReadFailure error={run.error} what="這個 Run" />
      {run.data && (
        <p className="note">
          <Link to="/skills/$skillId" params={{ skillId: run.data.skill_id }}>
            回到這個 Run 的 Skill
          </Link>
        </p>
      )}

      <EvaluationPanel runId={runId} runStatus={general.data?.status} />

      {general.data && <InFlight summary={general.data} />}

      <CancelRunControl runId={runId} status={general.data?.status} />

      <p>
        <Link to="/runs/$runId/compare" params={{ runId }} search={{ against: "" }}>
          與另一個 Run 比較
        </Link>
      </p>

      <h2>執行紀錄</h2>
      <p className="note" data-role="teaching">
        一般模式是摘要，進階模式是這次 Run 的原始事件（已遮罩）——同一份紀錄的兩種詳細度，
        只影響下面這一節。
      </p>
      <div role="group" aria-label="執行紀錄的詳細度">
        <button type="button" aria-pressed={mode === "general"} onClick={() => setMode("general")}>
          一般模式
        </button>
        <button
          type="button"
          aria-pressed={mode === "advanced"}
          onClick={() => setMode("advanced")}
        >
          進階模式
        </button>
      </div>
      {mode === "general" ? (
        <GeneralMode runId={runId} />
      ) : (
        <AdvancedMode
          key={runId}
          runId={runId}
          active={Boolean(general.data?.status && CANCELLABLE.has(general.data.status))}
        />
      )}

      <RunArtifacts runId={runId} />
    </section>
  );
}
