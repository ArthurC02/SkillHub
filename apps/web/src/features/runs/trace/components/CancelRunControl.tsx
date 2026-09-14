import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { ApiError } from "../../../../core/api/client";
import { useState } from "react";
import { useCancelRun } from "../../runs.service";
import { IN_FLIGHT_RUN_STATUSES } from "../../trace.service";

export const CANCELLABLE = IN_FLIGHT_RUN_STATUSES;

function cancelFailureSentence(error: unknown): string {
  if (error instanceof ApiError && error.status === 409)
    return "這個 Run 已經結束，沒有東西可以取消。";
  if (error instanceof ApiError && error.status === 404) return "找不到這個 Run。";
  return "取消要求沒有送出，可以再按一次。";
}

export function CancelRunControl({ runId, status }: { runId: string; status?: string }) {
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState("");
  const cancel = useCancelRun(runId);
  const confirmCancel = () =>
    cancel.mutate(undefined, {
      onSuccess: (result) => setMessage(result.note ?? "已送出取消要求。"),
      onSettled: () => setConfirming(false),
    });
  const failure = cancel.error ? (
    <ReadFailure error={cancel.error} what="取消這個 Run">
      <p role="alert">{cancelFailureSentence(cancel.error)}</p>
    </ReadFailure>
  ) : null;

  if (!status || !CANCELLABLE.has(status))
    return (
      <>
        {failure}
        {message && <p role="status">{message}</p>}
      </>
    );
  if (!confirming) {
    return (
      <p>
        <button type="button" onClick={() => setConfirming(true)}>
          取消這個 Run
        </button>
        {message && <span role="status"> {message}</span>}
        {failure}
      </p>
    );
  }
  return (
    <div>
      <p className="note">確定要取消？已開始的 Sandbox 仍要等平台完成停止與清理。</p>
      <button type="button" disabled={cancel.isPending} onClick={confirmCancel}>
        確認取消
      </button>{" "}
      <button type="button" disabled={cancel.isPending} onClick={() => setConfirming(false)}>
        返回
      </button>
      {failure}
    </div>
  );
}
