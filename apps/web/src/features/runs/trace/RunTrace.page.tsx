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
import { SkillWorkspaceNav } from "../../skill";
import "./RunTrace.page.css";

function RunSourceContext({
  skillId,
  versionId,
  testCaseId,
}: {
  skillId: string;
  versionId: string;
  testCaseId?: string;
}) {
  return (
    <nav className="download-item run-source-context" aria-label="這次試跑的來源">
      <strong>這次試跑的來源</strong>
      <ul className="chip-row">
        <li>
          <Link to="/skills/$skillId" params={{ skillId }}>
            來源小工具
          </Link>
        </li>
        <li>
          <Link to="/skills/$skillId/versions/$versionId" params={{ skillId, versionId }}>
            來源 Version
          </Link>
        </li>
        {testCaseId && (
          <li>
            <Link
              to="/lab/test-cases/$testCaseId"
              params={{ testCaseId }}
              search={{ version: versionId }}
            >
              來源測試題
            </Link>
          </li>
        )}
      </ul>
    </nav>
  );
}

function RunWorkspaceRail({ runId, status }: { runId: string; status?: string }) {
  return (
    <aside className="run-workspace-rail" aria-label="試跑紀錄操作與區段導覽">
      <section>
        <h2>這次試跑</h2>
        <nav aria-label="試跑結果導覽">
          <ul className="run-section-nav">
            <li>
              <a href="#run-decision">任務判定</a>
            </li>
            <li>
              <a href="#run-trace">執行紀錄</a>
            </li>
            <li>
              <a href="#run-artifacts">產出</a>
            </li>
          </ul>
        </nav>
        <p>
          <Link to="/runs/$runId/compare" params={{ runId }}>
            與另一個試跑比較
          </Link>
        </p>
        <CancelRunControl runId={runId} status={status} />
      </section>
    </aside>
  );
}

export function RunTrace() {
  const { runId } = useParams({ from: "/runs/$runId" });
  const { events: linkedEvents } = useSearch({ strict: false }) as { events?: string };
  const [mode, setMode] = useState<"general" | "advanced">(linkedEvents ? "advanced" : "general");
  const run = useRun(runId);
  const general = useTrace(runId, "general");

  return (
    <article className="run-workspace">
      <header className="run-workspace-header">
        <p className="run-eyebrow">驗證與執行</p>
        <h1>試跑結果</h1>
        <p className="run-identity" data-role="evidence">
          <strong>試跑紀錄 ID：</strong>
          <code>{runId}</code>
        </p>
        <ReadFailure error={run.error} what="這次試跑" />
        {run.data && (
          <>
            <SkillWorkspaceNav
              skillId={run.data.skill_id}
              versionId={run.data.skill_version_id}
              testCaseId={run.data.test_case_id}
            />
            <RunSourceContext
              skillId={run.data.skill_id}
              versionId={run.data.skill_version_id}
              testCaseId={run.data.test_case_id}
            />
          </>
        )}
      </header>

      <section className="run-workspace-panel run-decision" id="run-decision">
        <EvaluationPanel runId={runId} runStatus={general.data?.status} />
        {general.data && <InFlight summary={general.data} />}
      </section>

      <div className="run-workspace-layout">
        <RunWorkspaceRail runId={runId} status={general.data?.status} />

        <div className="run-workspace-main">
          <section className="run-workspace-panel" id="run-trace">
            <p className="run-eyebrow">執行證據</p>
            <h2>執行紀錄</h2>
            <p className="note" data-role="teaching">
              一般模式是摘要，進階模式是這次試跑的原始事件（已遮罩）——同一份紀錄的兩種詳細度，
              只影響下面這一節。
            </p>
            <div className="run-mode-switch" role="group" aria-label="執行紀錄的詳細度">
              <button
                type="button"
                aria-pressed={mode === "general"}
                onClick={() => setMode("general")}
              >
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
              <GeneralMode runId={runId} general={general} />
            ) : (
              <AdvancedMode
                key={runId}
                runId={runId}
                active={Boolean(general.data?.status && CANCELLABLE.has(general.data.status))}
              />
            )}
          </section>

          <section className="run-workspace-panel" id="run-artifacts">
            <RunArtifacts runId={runId} />
          </section>
        </div>
      </div>
    </article>
  );
}
