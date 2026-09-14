import { useRun } from "../../runs.service";

export function FailureClass({ runId, status }: { runId: string; status: string }) {
  const run = useRun(runId);
  if (status !== "failed") return null;
  if (run.error) return null;
  return (
    <>
      <p>
        失敗類別：
        <strong>{run.data?.failure_class?.label ?? (run.isPending ? "讀取中…" : "未記錄")}</strong>
      </p>
      {run.data?.failure_class && <p className="note">{run.data.failure_class.note}</p>}
    </>
  );
}
