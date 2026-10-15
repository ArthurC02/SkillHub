import { useEffect, useRef } from "react";
import { Link } from "@tanstack/react-router";
import { serverReason } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { isContinuationResultUnknown } from "../../runs.service";

export function ContinuationFailure({ error }: { error: unknown }) {
  const unknown = Boolean(error) && isContinuationResultUnknown(error);
  const failure = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (unknown) {
      failure.current?.focus();
      failure.current?.scrollIntoView({ block: "center" });
    }
  }, [error, unknown]);
  return (
    <ReadFailure error={error} what="送出回答">
      <div ref={failure} role="alert" tabIndex={-1} aria-label="回答送出結果">
        {unknown ? (
          <>
            <p>無法確認回答是否已送出並開始下一次試跑。請先核對試跑紀錄，再決定是否重新送出。</p>
            <p>
              <Link to="/workspace/runs" target="_blank" rel="noopener noreferrer">
                另開試跑紀錄
              </Link>
            </p>
            <p className="note">
              原始問題仍然顯示，不代表尚未收到回答；重新送出會嘗試開始試跑並使用額度。
            </p>
          </>
        ) : (
          <p>回答未被接受。{serverReason(error)}</p>
        )}
      </div>
    </ReadFailure>
  );
}
