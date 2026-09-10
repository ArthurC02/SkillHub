import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { LoginRequired, ReadFailure, unauthenticated } from "./LoginRequired";
import { useMe } from "../api/me";
import {
  BUILD_ID,
  FEEDBACK_MAX_MESSAGE,
  feedbackPagePath,
  feedbackRunID,
  submitFeedback,
  type FeedbackKind,
} from "../api/feedback";

export const KIND_LABEL: Record<FeedbackKind, string> = {
  blocking_issue: "有東西擋住我，做不下去",
  need_signal: "我想要的東西，這裡沒有",
};

const runes = (s: string) => [...s].length;

export const KIND_NOTE: Record<FeedbackKind, string> = {
  blocking_issue: "例如：按了沒有反應、看不懂錯誤訊息、卡在某一步過不去。",
  need_signal: "例如：想用的功能不存在、額度不夠、還沒被邀請就想試。",
};

export function FeedbackEntry({ pathname }: { pathname: string }) {
  const me = useMe();
  const [kind, setKind] = useState<FeedbackKind>("blocking_issue");
  const [message, setMessage] = useState("");
  const [invalid, setInvalid] = useState("");
  const [sent, setSent] = useState(false);

  const pagePath = feedbackPagePath(pathname);
  const runID = feedbackRunID(pathname);

  const send = useMutation({
    mutationFn: () =>
      submitFeedback({
        kind,
        message: message.trim(),
        page_path: pagePath,
        run_id: runID,
        build_id: BUILD_ID,
      }),
    onSuccess: () => {
      setSent(true);
      setMessage("");
    },
  });

  function submit() {
    setSent(false);
    const trimmed = message.trim();
    if (trimmed === "") {
      setInvalid("請先寫下發生了什麼事。內容不能空白——只有這一段是你的話，其餘欄位都只是位置。");
      return;
    }
    if (runes(trimmed) > FEEDBACK_MAX_MESSAGE) {
      setInvalid(
        `內容最多 ${FEEDBACK_MAX_MESSAGE} 字，目前 ${runes(trimmed)} 字。` +
          "請刪掉一些再送出，這樣才不會有一半被丟掉。",
      );
      return;
    }
    setInvalid("");
    send.mutate();
  }

  return (
    <details className="feedback-entry">
      <summary>回報問題</summary>
      {unauthenticated(me.error) ? (
        <LoginRequired what="回報問題" />
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <fieldset>
            <legend>這是哪一種？</legend>
            {(Object.keys(KIND_LABEL) as FeedbackKind[]).map((k) => (
              <p key={k}>
                <label>
                  <input
                    type="radio"
                    name="feedback-kind"
                    value={k}
                    checked={kind === k}
                    onChange={() => setKind(k)}
                  />{" "}
                  {KIND_LABEL[k]}
                </label>{" "}
                <span className="note">{KIND_NOTE[k]}</span>
              </p>
            ))}
          </fieldset>

          <p>
            <label htmlFor="feedback-message">發生了什麼事</label>
            <br />
            <textarea
              id="feedback-message"
              rows={4}
              cols={60}
              aria-describedby="feedback-context"
              value={message}
              onChange={(e) => {
                setMessage(e.target.value);
                setInvalid("");
              }}
            />
            <br />
            <span className="note">
              {runes(message)}／{FEEDBACK_MAX_MESSAGE} 字
            </span>
          </p>

          <p className="note" id="feedback-context">
            會跟著送出的只有這些：目前頁面 <code>{pagePath}</code>
            {runID ? (
              <>
                、你正在看的 Run <code>{runID}</code>
              </>
            ) : (
              ""
            )}
            {BUILD_ID ? (
              <>
                、這一頁的 Build 識別碼 <code>{BUILD_ID}</code>
              </>
            ) : (
              ""
            )}
            。除此之外不會擷取任何東西——沒有截圖、沒有 console、沒有自動蒐集畫面內容。
          </p>

          <p>
            <button type="submit" disabled={send.isPending}>
              {send.isPending ? "送出中…" : "送出回報"}
            </button>
          </p>
        </form>
      )}

      {invalid && <p role="alert">{invalid}</p>}
      {send.error && (
        <ReadFailure error={send.error} what="回報">
          <p role="alert">
            送不出去。這份內容還留在上面，可以稍後再按一次送出；目前沒有第二條回報管道。
          </p>
        </ReadFailure>
      )}
      {sent && (
        <p role="status">
          已收到，謝謝。這裡沒有回覆機制，也沒有查詢頁面——需要回覆的話，請在內容裡留下聯絡方式。
        </p>
      )}
    </details>
  );
}
