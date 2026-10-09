import { useEffect, useRef } from "react";
import { Link, useSearch } from "@tanstack/react-router";
import {
  AGENT_RUN_POLL_MS,
  canKeepLoadedFindings,
  usePlatformAgentFindings,
  usePlatformAgentProposals,
  usePlatformAgentRun,
  usePlatformAgentRuns,
  usePlatformAgents,
} from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { AgentControls, BrakeControls } from "./components/AgentControls";
import { AgentRunFacts, AgentRunList, AgentRunSteps } from "./components/AgentRuns";
import { DailyReport } from "./components/DailyReport";
import { FindingDetail } from "./components/FindingDetail";
import { FindingInbox } from "./components/FindingInbox";
import { ProposalDetail } from "./components/ProposalDetail";
import { ProposalList } from "./components/ProposalList";
import "./AdminAgents.page.css";

function readLabel(value: string | undefined, error: Error | null): string {
  if (error) return "無法取得";
  return value ?? "讀取中";
}

function AgentWorkbench({ status }: { status?: "resolved" | "dismissed" | "recovered" }) {
  const agents = usePlatformAgents();
  const proposals = usePlatformAgentProposals();
  const findings = usePlatformAgentFindings(status);
  const runs = usePlatformAgentRuns();
  const findingReadError = canKeepLoadedFindings(findings.error, findings.isFetchNextPageError)
    ? null
    : findings.error;
  const pendingDecisions = readLabel(
    proposals.data && `${proposals.data.total} 件`,
    proposals.error,
  );
  const liveFindings = readLabel(
    findings.data &&
      `${findings.data.pages[0].counts.open + findings.data.pages[0].counts.acknowledged} 件`,
    findingReadError,
  );
  const running = readLabel(
    runs.data && `${runs.data.runs.filter((item) => item.status === "running").length} 次`,
    runs.error,
  );
  const brake = readLabel(agents.data && (agents.data.brake ? "已拉下" : "已放開"), agents.error);

  return (
    <>
      <nav aria-label="平台 Agent 工作區" className="agent-workbench-summary">
        <a href="#admin-agent-proposals">
          待核准 <strong>{pendingDecisions}</strong>
        </a>
        <a href="#admin-agent-findings">
          待辦 <strong>{liveFindings}</strong>
        </a>
        <a href="#admin-agent-runs">
          最近 50 次內執行中 <strong>{running}</strong>
        </a>
        <a href="#admin-agent-brake" data-braked={agents.data?.brake ? true : undefined}>
          煞車 <strong>{brake}</strong>
        </a>
      </nav>

      <section id="admin-agent-brake" aria-labelledby="admin-agent-brake-heading">
        <h2 id="admin-agent-brake-heading">全域煞車</h2>
        {agents.isPending && <Loading what="Agent 控制" />}
        <ReadFailure error={agents.error} what="Agent 控制" />
        {agents.data && !agents.error && <BrakeControls brake={agents.data.brake} />}
      </section>

      <div className="agent-workbench-queues">
        <section id="admin-agent-proposals" aria-labelledby="admin-agent-proposals-heading">
          <h2 id="admin-agent-proposals-heading" tabIndex={-1}>
            提案
          </h2>
          <ProposalList />
        </section>

        <section id="admin-agent-findings" aria-labelledby="admin-agent-findings-heading">
          <h2 id="admin-agent-findings-heading" tabIndex={-1}>
            待辦
          </h2>
          <FindingInbox status={status} />
        </section>
      </div>

      <section id="admin-agent-runs" aria-labelledby="admin-agent-runs-heading">
        <h2 id="admin-agent-runs-heading" tabIndex={-1}>
          日報與執行紀錄
        </h2>
        {runs.isPending && <Loading what="執行紀錄" />}
        <ReadFailure error={runs.error} what="執行紀錄" />
        {runs.data && !runs.error && (
          <ListFreshness
            inFlight={runs.data.runs.some((run) => run.status === "running")}
            updatedAt={runs.dataUpdatedAt}
            fetching={runs.isFetching}
            refetch={runs.refetch}
            subject="Agent 執行"
            pollSeconds={AGENT_RUN_POLL_MS / 1000}
          />
        )}
        {runs.data && !runs.error && <AgentRunList runs={runs.data.runs} total={runs.data.total} />}
      </section>

      <section id="admin-agent-controls" aria-label="Agent 控制">
        <h2>Agent</h2>
        {agents.data && !agents.error && <AgentControls agents={agents.data.agents} />}
      </section>
    </>
  );
}

function AgentRunDetail({ id }: { id: string }) {
  const detail = usePlatformAgentRun(id);
  const opened = detail.data;
  const live = !detail.error && opened?.status === "running";
  return (
    <section aria-labelledby="admin-agent-run-heading">
      <h2 id="admin-agent-run-heading" tabIndex={-1}>
        這次執行
      </h2>
      <p>
        <Link to="/admin/agents" search={{}}>
          回到執行紀錄
        </Link>
      </p>
      {detail.isPending && <Loading what="執行紀錄" />}
      <ReadFailure error={detail.error} what="執行紀錄" />
      {live && opened && (
        <>
          <p role="status" className="notice">
            仍在執行；已記錄 {opened.steps} 步。平台會自行結束，可以離開這頁，回來查看結果。
          </p>
          <p className="note">
            狀態每 {AGENT_RUN_POLL_MS / 1000} 秒自動更新；上次取得於{" "}
            <Timestamp at={new Date(detail.dataUpdatedAt).toISOString()} relative />。{" "}
            <button
              type="button"
              disabled={detail.isFetching}
              onClick={() => void detail.refetch()}
            >
              {detail.isFetching ? "重新整理中…" : "重新整理"}
            </button>
          </p>
        </>
      )}
      {opened && !detail.error && (
        <>
          <p>
            <strong>{opened.agent}</strong>
          </p>
          <AgentRunFacts run={opened} />
          <DailyReport run={opened} />
        </>
      )}
      {opened && !detail.error && <AgentRunSteps run={id} live={live} />}
    </section>
  );
}

export function AdminAgents() {
  const { status, finding, proposal, run } = useSearch({ from: "/admin/agents" });
  const view = proposal ? "proposal" : finding ? "finding" : run ? "run" : undefined;
  const previousView = useRef(view);
  useEffect(() => {
    const previous = previousView.current;
    previousView.current = view;
    if (previous === view) return;
    const target = view
      ? {
          proposal: "admin-proposal-heading",
          finding: "admin-finding-heading",
          run: "admin-agent-run-heading",
        }[view]
      : previous
        ? {
            proposal: "admin-agent-proposals-heading",
            finding: "admin-agent-findings-heading",
            run: "admin-agent-runs-heading",
          }[previous]
        : undefined;
    if (target) document.getElementById(target)?.focus();
  }, [view]);
  return (
    <AdminPage
      heading="平台 Agent"
      lede="平台 Agent 只讀維運事實、不讀私人資料；日報的異常會進待辦，需要執行維運工作時必須由你核准提案。"
    >
      {proposal ? (
        <ProposalDetail key={proposal} id={proposal} />
      ) : finding ? (
        <FindingDetail id={finding} />
      ) : run ? (
        <AgentRunDetail id={run} />
      ) : (
        <AgentWorkbench status={status} />
      )}
    </AdminPage>
  );
}
