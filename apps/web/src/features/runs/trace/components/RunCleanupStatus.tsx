import { useRun } from "../../runs.service";
import { CLEANUP_BADGE } from "../../runs.model";

export function RunCleanupStatus({ runId }: { runId: string }) {
  const run = useRun(runId);
  if (!run.data?.cleanup_status) return null;
  const { cleanup_status } = run.data;
  return (
    <p className="badge-row">
      <span className={CLEANUP_BADGE[cleanup_status.value] ?? "badge badge-unverified"}>
        清理狀態：{cleanup_status.label}
      </span>{" "}
      <span className="note">{cleanup_status.note}</span>
    </p>
  );
}
