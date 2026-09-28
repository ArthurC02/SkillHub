import { Link } from "@tanstack/react-router";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../../shared/ui/LoginRequired.model";
import type { useConfirmAndStartRun } from "../../lab.service";
import type { PreflightResponse } from "../../lab.service";
import { BLOCKED_SENTENCE, startFailureSentence } from "../preflight.model";

export function RunStartControl({
  start,
  hash,
  blocked,
}: {
  start: ReturnType<typeof useConfirmAndStartRun>;
  hash: string;
  blocked: PreflightResponse["blocked"];
}) {
  const runId = start.data?.run_id ?? "";
  const message = startFailureSentence(start.error);

  return (
    <>
      {unauthenticated(start.error) && <ReadFailure error={start.error} what="Run" />}
      {message && <p role="alert">{message}</p>}

      {runId ? (
        <p>
          已開始 Run。{" "}
          <Link to="/runs/$runId" params={{ runId }}>
            查看這次 Run 的結果
          </Link>
          <span className="note">
            Run ID：<code>{runId}</code>
          </span>
        </p>
      ) : blocked ? (
        <p role="alert" className="notice">
          {BLOCKED_SENTENCE[blocked]}
        </p>
      ) : (
        <>
          <p className="note">平台目前只讓有封測邀請的帳號開始 Run。</p>
          <button
            type="button"
            className="action"
            disabled={start.isPending}
            onClick={() => start.mutate(hash)}
          >
            {start.isPending ? "開始中…" : "我確認以上權限,開始 Run"}
          </button>
        </>
      )}
      <p>不同意就不要按下按鈕:未確認的 Run 不會被建立。</p>
    </>
  );
}
