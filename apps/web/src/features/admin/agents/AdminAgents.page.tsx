import { useSearch } from "@tanstack/react-router";
import { usePlatformAgentRuns, usePlatformAgents, type PlatformAgentRun } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { AgentControls } from "./components/AgentControls";
import { AgentRunList, AgentRunSteps } from "./components/AgentRuns";

const DAILY_REPORT_AGENT = "daily-report";

type ReportItem = { status: "fine" | "attention"; text: string; cites: string[] };

function reportItems(run: PlatformAgentRun): ReportItem[] {
  const items = run.result?.items;
  return Array.isArray(items) ? (items as ReportItem[]) : [];
}

function DailyReport({ runs }: { runs: PlatformAgentRun[] }) {
  const latest = runs.find((run) => run.agent === DAILY_REPORT_AGENT && run.status === "completed");
  if (!latest) return <p>還沒有完成的日報：0 份。</p>;
  const items = reportItems(latest);
  const attention = items.filter((item) => item.status === "attention");
  const fine = items.filter((item) => item.status === "fine");
  return (
    <>
      <p className="note">
        <Timestamp at={latest.finished_at ?? latest.started_at} />{" "}
        完成。每一項都附它根據的事實，平台已核對這些事實都在當天的維運報表裡。
      </p>
      {[
        { heading: `需要注意：${attention.length} 項`, list: attention },
        { heading: `正常：${fine.length} 項`, list: fine },
      ].map(({ heading, list }) => (
        <section key={heading}>
          <h3>{heading}</h3>
          <ul className="download-list">
            {list.map((item) => (
              <li className="download-item" key={item.text}>
                <p>{item.text}</p>
                <p className="note">
                  依據：
                  {item.cites.map((cite) => (
                    <code key={cite}> {cite}</code>
                  ))}
                </p>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}

export function AdminAgents() {
  const { run } = useSearch({ from: "/admin/agents" });
  const agents = usePlatformAgents();
  const runs = usePlatformAgentRuns();

  return (
    <AdminPage
      heading="平台 Agent"
      lede="平台自己的 Agent 只讀維運事實、不讀任何人的資料；每一步都記在下面的執行紀錄裡。"
    >
      <h2>最近的日報</h2>
      {runs.isPending && <Loading what="執行紀錄" />}
      <ReadFailure error={runs.error} what="執行紀錄" />
      {runs.data && <DailyReport runs={runs.data.runs} />}

      {agents.isPending && <Loading what="Agent 清單" />}
      <ReadFailure error={agents.error} what="Agent 清單" />
      {agents.data && <AgentControls agents={agents.data.agents} brake={agents.data.brake} />}

      <h2>最近的執行</h2>
      {runs.data && <AgentRunList runs={runs.data.runs} />}

      {run && <AgentRunSteps run={run} />}
    </AdminPage>
  );
}
