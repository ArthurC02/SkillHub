import { useSearch } from "@tanstack/react-router";
import { usePlatformAgentRuns, usePlatformAgents } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { AdminPage } from "../components/AdminPage";
import { AgentControls } from "./components/AgentControls";
import { AgentRunList, AgentRunSteps } from "./components/AgentRuns";
import { DailyReport } from "./components/DailyReport";
import { FindingDetail } from "./components/FindingDetail";
import { FindingInbox } from "./components/FindingInbox";
import { ProposalDetail } from "./components/ProposalDetail";
import { ProposalList } from "./components/ProposalList";

export function AdminAgents() {
  const { status, finding, proposal, run } = useSearch({ from: "/admin/agents" });
  const agents = usePlatformAgents();
  const runs = usePlatformAgentRuns();
  const opened = runs.data?.runs.find((r) => r.id === run);

  return (
    <AdminPage
      heading="平台 Agent"
      lede="平台自己的 Agent 只讀維運事實、不讀任何人的資料。日報說需要注意的事會進待辦，同一件事隔天再被報出來就併在同一筆，事實恢復正常時自動標成已自行恢復。Agent 想補跑逾期的維運工作時只能提案，由你核准才會執行。"
    >
      <h2>待核准</h2>
      {proposal ? <ProposalDetail key={proposal} id={proposal} /> : <ProposalList />}

      <h2>待辦</h2>
      {finding ? <FindingDetail id={finding} /> : <FindingInbox status={status} />}

      <h2>日報與執行紀錄</h2>
      {runs.isPending && <Loading what="執行紀錄" />}
      <ReadFailure error={runs.error} what="執行紀錄" />
      {runs.data && <AgentRunList runs={runs.data.runs} />}
      {run && (
        <section aria-labelledby="admin-agent-run-heading">
          <h2 id="admin-agent-run-heading">這次執行</h2>
          {opened && <DailyReport run={opened} />}
          <AgentRunSteps run={run} />
        </section>
      )}

      {agents.isPending && <Loading what="Agent 清單" />}
      <ReadFailure error={agents.error} what="Agent 清單" />
      {agents.data && <AgentControls agents={agents.data.agents} brake={agents.data.brake} />}
    </AdminPage>
  );
}
