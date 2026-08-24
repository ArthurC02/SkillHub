import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  FEEDBACK_MAX_MESSAGE,
  feedbackPagePath,
  feedbackRunID,
  submitFeedback,
  type FeedbackKind,
} from "../api/feedback";

/**
 * 03:BETA-003/004 — the entry point for POST /feedback, in the layout so it is
 * reachable from every screen (beta-design §5「全站可及的入口」). Until this
 * existed the endpoint could only be reached with curl, which is not a channel a
 * beta tester has.
 *
 * The rules it exists to keep:
 *
 * 1. **Nothing is captured that the reporter cannot see.** The page path and the
 *    run id are rendered next to the message before it is sent — no screenshot,
 *    no console capture, no automatic context grab (beta-design §5). They are
 *    displayed as facts, not as fields to edit: they say where the report came
 *    from, and a reporter who could rewrite them would be filing about somewhere
 *    else.
 * 2. **A refused submit says what to fix.** 02:NFR-007 asks for clear validation
 *    messages, and a permanently disabled button with no stated cause reads as a
 *    bug — the same reasoning the DISC-003 filter bar already follows for its
 *    disabled controls.
 * 3. **The two kinds are the reporter's choice, never inferred.** 「我卡住了」and
 *    「這裡沒有我要的東西」 are different reports (BETA-004 vs BETA-005) and the
 *    platform cannot tell them apart from the outside.
 */

const KIND_LABEL: Record<FeedbackKind, string> = {
  blocking_issue: "有東西擋住我，做不下去",
  need_signal: "我想要的東西，這裡沒有",
};

/**
 * What the server counts. `String.length` is UTF-16 code units, `len([]rune(s))`
 * on the Go side is code points, and an emoji is two of the first and one of the
 * second — so the counter below used to tell a reporter they were over a limit
 * they were not near. The direction was safe; the sentence was not true.
 *
 * The `maxLength` attribute is gone for the same reason and not replaced: the
 * browser can only count the wrong unit, and GenerateSkill.tsx already refuses
 * it on this exact argument. The pre-check in `submit` is the one gate here,
 * and the server is the one that decides.
 */
const runes = (s: string) => [...s].length;

const KIND_NOTE: Record<FeedbackKind, string> = {
  blocking_issue: "例如：按了沒有反應、看不懂錯誤訊息、卡在某一步過不去。",
  need_signal: "例如：想用的功能不存在、額度不夠、還沒被邀請就想試。",
};

export function FeedbackEntry({ pathname }: { pathname: string }) {
  const [kind, setKind] = useState<FeedbackKind>("blocking_issue");
  const [message, setMessage] = useState("");
  const [invalid, setInvalid] = useState("");
  const [sent, setSent] = useState(false);

  const pagePath = feedbackPagePath(pathname);
  const runID = feedbackRunID(pathname);

  const send = useMutation({
    mutationFn: () =>
      submitFeedback({ kind, message: message.trim(), page_path: pagePath, run_id: runID }),
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
    // Code points, not UTF-16 code units: the server checks `len([]rune(message))`
    // and `String.length` counts an emoji as two. Same reasoning GenerateSkill.tsx
    // states for refusing a `maxLength` — one problem, and now one answer.
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
          。除此之外不會擷取任何東西——沒有截圖、沒有 console、沒有自動蒐集畫面內容。
        </p>

        <p>
          <button type="submit" disabled={send.isPending}>
            {send.isPending ? "送出中…" : "送出回報"}
          </button>
        </p>
      </form>

      {invalid && <p role="alert">{invalid}</p>}
      {send.error && (
        <p role="alert">
          送不出去：{send.error.message}
          。這份內容還留在上面，可以稍後再按一次送出；如果一直失敗，請直接寫信給我們。
        </p>
      )}
      {sent && (
        <p role="status">
          已收到，謝謝。這裡沒有回覆機制，也沒有查詢頁面——需要回覆的話，請在內容裡留下聯絡方式。
        </p>
      )}
    </details>
  );
}
