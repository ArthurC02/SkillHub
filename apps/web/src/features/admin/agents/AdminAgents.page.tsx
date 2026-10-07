import { useSearch } from "@tanstack/react-router";
import { usePlatformAgentRuns, usePlatformAgents } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { AdminPage } from "../components/AdminPage";
import { AgentControls } from "./components/AgentControls";
import { AgentRunList, AgentRunSteps } from "./components/AgentRuns";
import { DailyReport } from "./components/DailyReport";

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
