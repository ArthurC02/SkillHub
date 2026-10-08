import { Link } from "@tanstack/react-router";
import {
  useDecideProposal,
  usePlatformAgentProposal,
  type PlatformAgentProposalDetail,
} from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ActionForm } from "../../components/ActionForm";
import { PROPOSAL_STATUS, PROPOSAL_TIER, actionLabel, previewLine } from "./proposalLabels";
import "./ProposalDetail.css";

const DECISIONS = [
  { decision: "approve", label: "核准並執行", done: "已核准，維運程序會在幾分鐘內執行。" },
  { decision: "reject", label: "駁回", done: "已駁回。" },
] as const;

function Decide({ proposal }: { proposal: PlatformAgentProposalDetail }) {
  const decide = useDecideProposal();
  return (
    <>
      {DECISIONS.map(({ decision, label, done }) => (
        <ActionForm
          key={decision}
          id={`admin-proposal-${decision}`}
          submitLabel={label}
          tone={decision === "approve" && proposal.tier === "destructive" ? "caution" : undefined}
          pending={decide.isPending}
          error={decide.error}
          done={decide.isSuccess && done}
          contextKey={`${proposal.id}:${proposal.status}:${decision}`}
          onSubmit={(note) => decide.mutate({ id: proposal.id, decision, note })}
        />
      ))}
    </>
  );
}

function Outcome({ proposal }: { proposal: PlatformAgentProposalDetail }) {
  return (
    <>
      {proposal.decided_at && (
        <p>
          <Timestamp at={proposal.decided_at} /> 營運者
          {proposal.status === "rejected" ? "駁回" : "核准"}：{proposal.decision_note}
        </p>
      )}
      {proposal.started_at && (
        <p>
          <Timestamp at={proposal.started_at} /> 開始執行
        </p>
      )}
      {proposal.finished_at && proposal.status !== "rejected" && (
        <p>
          <Timestamp at={proposal.finished_at} /> {PROPOSAL_STATUS[proposal.status]}
          {proposal.outcome && `：${proposal.outcome}`}
        </p>
      )}
    </>
  );
}

export function ProposalDetail({ id }: { id: string }) {
  const detail = usePlatformAgentProposal(id);
  const proposal = detail.data;
  return (
    <section aria-labelledby="admin-proposal-heading">
      <h2 id="admin-proposal-heading">這個提案</h2>
      <p>
        <Link to="/admin/agents" search={(prev) => ({ ...prev, proposal: undefined })}>
          回到提案
        </Link>
      </p>
      {detail.isPending && <Loading what="這個提案" />}
      <ReadFailure error={detail.error} what="這個提案" />
      {proposal && (
        <>
          <p>
            <strong>{actionLabel(proposal.action)}</strong>
          </p>
          <p className="badge-row">
            <span className="badge">{PROPOSAL_STATUS[proposal.status]}</span>
            <span className={proposal.tier === "destructive" ? "badge badge-danger" : "badge"}>
              {PROPOSAL_TIER[proposal.tier]}
            </span>
          </p>
          <p>{proposal.reason}</p>
          <h3>會發生什麼</h3>
          <ul>
            {proposal.preview.counts.map((item) => (
              <li key={item.key}>{previewLine(item)}</li>
            ))}
          </ul>
          {proposal.preview.batch_limit && (
            <p className="note">這個工作一次最多處理 {proposal.preview.batch_limit} 筆。</p>
          )}
          <h3>依據</h3>
          <ul>
            {proposal.cites.map((cite) => (
              <li key={cite}>
                <code className="proposal-cite">{cite}</code>
              </li>
            ))}
          </ul>
          <h3>經過</h3>
          <p>
            <Timestamp at={proposal.proposed_at} /> 由 {proposal.agent} 提出
          </p>
          <Outcome proposal={proposal} />
          {proposal.status === "proposed" && (
            <>
              <h3>決定</h3>
              <p className="note">
                <Timestamp at={proposal.expires_at} /> 前沒有核准就作廢。
              </p>
              <Decide proposal={proposal} />
            </>
          )}
        </>
      )}
    </section>
  );
}
