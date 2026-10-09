import { useEffect, useRef } from "react";
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
  against,
}: {
  start: ReturnType<typeof useConfirmAndStartRun>;
  hash: string;
  blocked: PreflightResponse["blocked"];
  against?: string;
}) {
  const runId = start.data?.run_id ?? "";
  const message = startFailureSentence(start.error);
  const resultLink = useRef<HTMLAnchorElement>(null);

  useEffect(() => {
    if (runId) resultLink.current?.focus();
  }, [runId]);

  return (
    <>
      {unauthenticated(start.error) && <ReadFailure error={start.error} what="試跑紀錄" />}
      {message && <p role="alert">{message}</p>}

      {runId ? (
        <>
          <p role="status">已開始試跑。</p>
          <p>
            <Link ref={resultLink} to="/runs/$runId" params={{ runId }}>
              查看這次試跑的結果
            </Link>
            <span className="note">
              試跑紀錄 ID：<code>{runId}</code>
            </span>
          </p>
          {against && (
            <p>
              <Link to="/runs/$runId/compare" params={{ runId }} search={{ against }}>
                和改版前的那次試跑逐條比較
              </Link>
              ：評估完成後，兩次試跑的每條驗收條件會並排列出。
            </p>
          )}
        </>
      ) : blocked ? (
        <p role="alert" className="notice">
          {BLOCKED_SENTENCE[blocked]}
        </p>
      ) : (
        <>
          <p className="note">平台目前只讓有封測邀請的帳號開始試跑。</p>
          <button
            type="button"
            className="action"
            disabled={start.isPending}
            onClick={() => start.mutate(hash)}
          >
            {start.isPending ? "開始中…" : "我確認以上權限,開始試跑"}
          </button>
        </>
      )}
      <p>不同意就不要按下按鈕:未確認的試跑紀錄不會被建立。</p>
    </>
  );
}
