import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import {
  usePlatformAgentProposals,
  type PlatformAgentProposal,
  type ProposalQueueView,
} from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ListFreshness } from "../../../../shared/ui/ListFreshness";
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
        <Link
          to="/admin/agents"
          search={(prev) => ({
            ...prev,
            proposal: proposal.id,
            finding: undefined,
            run: undefined,
          })}
        >
          打開這個提案
        </Link>
      </p>
    </li>
  );
}

export function ProposalList() {
  const { proposal_view, proposal_offset } = useSearch({ from: "/admin/agents" });
  const navigate = useNavigate();
  const view = proposal_view ?? "proposed";
  const offset = proposal_offset ?? 0;
  const proposals = usePlatformAgentProposals(view, offset);
  const page = proposals.error ? undefined : proposals.data;
  const rows = page?.proposals ?? [];
  const label = { proposed: "待核准", processing: "核准後處理中", closed: "最近七天結案" }[view];
  const nextOffset = offset + rows.length;
  const canNext = !!page && rows.length > 0 && nextOffset < page.total;
  const changeOffset = (next: number | undefined) =>
    navigate({
      to: "/admin/agents",
      search: (prev) => ({ ...prev, proposal_offset: next }),
    });
  return (
    <>
      <div className="field">
        <label htmlFor="admin-proposal-view">查看提案清單</label>
        <select
          id="admin-proposal-view"
          value={view}
          onChange={(event) => {
            const selected = event.target.value as ProposalQueueView;
            void navigate({
              to: "/admin/agents",
              search: (prev) => ({
                ...prev,
                proposal_view: selected === "proposed" ? undefined : selected,
                proposal_offset: undefined,
              }),
            });
          }}
        >
          <option value="proposed">待核准</option>
          <option value="processing">核准後處理中</option>
          <option value="closed">最近七天結案</option>
        </select>
      </div>
      {proposals.isPending && <Loading what="提案" />}
      <ReadFailure error={proposals.error} what="提案" />
      {page && (
        <ListFreshness
          inFlight={rows.some((proposal) => ["approved", "running"].includes(proposal.status))}
          showWhenIdle
          updatedAt={proposals.dataUpdatedAt}
          fetching={proposals.isFetching}
          refetch={proposals.refetch}
          subject="提案"
        />
      )}
      {page && rows.length === 0 && (
        <p>{offset > 0 ? "這一頁沒有提案，清單可能已更新。請返回上一頁。" : `${label}：0 件。`}</p>
      )}
      {page && rows.length > 0 && (
        <>
          <p className="note">
            {label}：共 {page.total} 件；目前顯示第 {offset + 1}–{nextOffset} 件。
          </p>
          <ul className="download-list">
            {rows.map((proposal) => (
              <ProposalRow proposal={proposal} key={proposal.id} />
            ))}
          </ul>
        </>
      )}
      {(offset > 0 || canNext) && (
        <nav aria-label="提案分頁">
          {offset > 0 && (
            <button type="button" onClick={() => void changeOffset(Math.max(0, offset - 20))}>
              上一頁
            </button>
          )}
          {canNext && (
            <button type="button" onClick={() => void changeOffset(nextOffset)}>
              下一頁
            </button>
          )}
        </nav>
      )}
    </>
  );
}
