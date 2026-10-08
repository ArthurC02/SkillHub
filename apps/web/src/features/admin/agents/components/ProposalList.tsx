import { Link } from "@tanstack/react-router";
import { usePlatformAgentProposals, type PlatformAgentProposal } from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { PROPOSAL_STATUS, PROPOSAL_TIER, actionLabel } from "./proposalLabels";

function ProposalRow({ proposal }: { proposal: PlatformAgentProposal }) {
  return (
    <li className="download-item">
      <p>
        <strong>{actionLabel(proposal.action)}</strong>
      </p>
      <p className="badge-row">
        <span className={proposal.status === "proposed" ? "badge badge-danger" : "badge"}>
          {PROPOSAL_STATUS[proposal.status]}
        </span>
        <span className="badge">{PROPOSAL_TIER[proposal.tier]}</span>
      </p>
      <p>{proposal.reason}</p>
      {proposal.status === "proposed" && (
        <p className="note">
          <Timestamp at={proposal.expires_at} /> 前沒有核准就作廢
        </p>
      )}
      <p>
        <Link to="/admin/agents" search={(prev) => ({ ...prev, proposal: proposal.id })}>
          打開這個提案
        </Link>
      </p>
    </li>
  );
}

export function ProposalList() {
  const proposals = usePlatformAgentProposals();
  return (
    <>
      {proposals.isPending && <Loading what="提案" />}
      <ReadFailure error={proposals.error} what="提案" />
      {proposals.data && proposals.data.proposals.length === 0 && (
        <p>待核准與最近七天的提案：0 件。</p>
      )}
      {proposals.data && proposals.data.proposals.length > 0 && (
        <ul className="download-list">
          {proposals.data.proposals.map((proposal) => (
            <ProposalRow proposal={proposal} key={proposal.id} />
          ))}
        </ul>
      )}
    </>
  );
}
