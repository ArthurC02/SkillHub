import { Link } from "@tanstack/react-router";
import {
  usePlatformAgentFindings,
  type FindingStatus,
  type PlatformAgentFinding,
} from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ListFreshness } from "../../../../shared/ui/ListFreshness";
import { FINDING_STATUS } from "./findingLabels";

type ClosedView = "resolved" | "dismissed" | "recovered";

const VIEWS: { status?: ClosedView; label: string; counts: FindingStatus[] }[] = [
  { label: "待辦", counts: ["open", "acknowledged"] },
  { status: "resolved", label: FINDING_STATUS.resolved, counts: ["resolved"] },
  { status: "recovered", label: FINDING_STATUS.recovered, counts: ["recovered"] },
  { status: "dismissed", label: FINDING_STATUS.dismissed, counts: ["dismissed"] },
];

function FindingRow({ finding }: { finding: PlatformAgentFinding }) {
  const live = finding.status === "open" || finding.status === "acknowledged";
  return (
    <li className="download-item">
      <p>
        <strong>{finding.title}</strong>
      </p>
      <p className="badge-row">
        <span className={finding.status === "open" ? "badge badge-danger" : "badge"}>
          {FINDING_STATUS[finding.status]}
        </span>
      </p>
      <p className="note">
        回報 {finding.seen_count} 次；最近一次 <Timestamp at={finding.last_seen_at} />
        {live && finding.seen_count > 1 && (
          <>
            ；首次 <Timestamp at={finding.first_seen_at} />
          </>
        )}
      </p>
      <p>
        <Link
          to="/admin/agents"
          search={(prev) => ({ ...prev, finding: finding.id, proposal: undefined, run: undefined })}
        >
          打開這件事
        </Link>
      </p>
    </li>
  );
}

export function FindingInbox({ status }: { status?: ClosedView }) {
  const findings = usePlatformAgentFindings(status);
  const counts = !findings.error && findings.data?.counts;
  const current = VIEWS.find((view) => view.status === status) ?? VIEWS[0];
  return (
    <>
      <nav aria-label="待辦的狀態" className="badge-row">
        {VIEWS.map((view) => (
          <Link
            key={view.label}
            to="/admin/agents"
            search={view.status ? { status: view.status } : {}}
            className="chip"
            aria-current={view === current ? "page" : undefined}
          >
            {view.label}
            {counts && `：${view.counts.reduce((sum, s) => sum + counts[s], 0)}`}
          </Link>
        ))}
      </nav>
      {findings.isPending && <Loading what="待辦" />}
      <ReadFailure error={findings.error} what="待辦" />
      {findings.data && !findings.error && (
        <ListFreshness
          inFlight={false}
          showWhenIdle
          updatedAt={findings.dataUpdatedAt}
          fetching={findings.isFetching}
          refetch={findings.refetch}
          subject="待辦"
        />
      )}
      {findings.data && !findings.error && findings.data.findings.length === 0 && (
        <p>{current.label}：0 件。</p>
      )}
      {findings.data && !findings.error && findings.data.findings.length > 0 && (
        <ul className="download-list">
          {findings.data.findings.map((finding) => (
            <FindingRow finding={finding} key={finding.id} />
          ))}
        </ul>
      )}
    </>
  );
}
