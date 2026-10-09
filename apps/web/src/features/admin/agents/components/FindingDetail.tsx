import { useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  useMoveFinding,
  usePlatformAgentFinding,
  type PlatformAgentFinding,
  type PlatformAgentFindingEvent,
  type FindingStatus,
} from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ActionForm } from "../../components/ActionForm";
import { FINDING_EVENT, FINDING_MOVES, FINDING_STATUS } from "./findingLabels";
import "./FindingDetail.css";

function latestEvidence(events: PlatformAgentFindingEvent[]): Record<string, unknown> {
  return [...events].reverse().find((event) => event.evidence)?.evidence ?? {};
}

function Moves({ finding }: { finding: PlatformAgentFinding }) {
  const move = useMoveFinding();
  const [selected, setSelected] = useState<FindingStatus>();
  const choices = FINDING_MOVES[finding.status];
  const choice = choices.find(({ to }) => to === selected) ?? choices[0];
  if (!choice) return null;

  return (
    <div onInput={() => move.reset()}>
      {choices.length > 1 && (
        <fieldset>
          <legend>處理方式</legend>
          {choices.map(({ to, label }) => (
            <label key={to}>
              <input
                type="radio"
                name="admin-finding-move"
                value={to}
                checked={choice.to === to}
                disabled={move.isPending}
                onChange={() => {
                  setSelected(to);
                  move.reset();
                }}
              />
              {label}
            </label>
          ))}
        </fieldset>
      )}
      <ActionForm
        key={`${finding.id}:${finding.status}:${choice.to}`}
        id="admin-finding"
        submitLabel={choice.label}
        tone={choice.to === "dismissed" ? "caution" : undefined}
        pending={move.isPending}
        error={move.error}
        contextKey={`${finding.id}:${finding.status}:${choice.to}`}
        onSubmit={(note) => move.mutate({ id: finding.id, status: choice.to, note })}
      />
      {move.isSuccess && (
        <p className="notice notice-success" role="status">
          已改成「{FINDING_STATUS[move.variables.status]}」。
        </p>
      )}
    </div>
  );
}

export function FindingDetail({ id }: { id: string }) {
  const detail = usePlatformAgentFinding(id);
  return (
    <section aria-labelledby="admin-finding-heading">
      <h2 id="admin-finding-heading" tabIndex={-1}>
        這件事
      </h2>
      <p>
        <Link to="/admin/agents" search={(prev) => ({ status: prev.status })}>
          回到待辦
        </Link>
      </p>
      {detail.isPending && <Loading what="這件事" />}
      <ReadFailure error={detail.error} what="這件事" />
      {detail.data && !detail.error && (
        <>
          <p>
            <strong>{detail.data.finding.title}</strong>
          </p>
          <p className="badge-row">
            <span className="badge">{FINDING_STATUS[detail.data.finding.status]}</span>
          </p>
          <h3>依據</h3>
          <ul className="finding-evidence">
            {Object.entries(latestEvidence(detail.data.events)).map(([cite, value]) => (
              <li key={cite}>
                <code className="finding-cite">{cite}</code> ＝ {JSON.stringify(value)}
              </li>
            ))}
          </ul>
          <h3>經過</h3>
          <ol>
            {detail.data.events.map((event) => (
              <li key={event.seq}>
                <Timestamp at={event.occurred_at} /> {FINDING_EVENT[event.kind]}
                {event.note && `：${event.note}`}
              </li>
            ))}
          </ol>
          <h3>處理</h3>
          <Moves finding={detail.data.finding} />
        </>
      )}
    </section>
  );
}
