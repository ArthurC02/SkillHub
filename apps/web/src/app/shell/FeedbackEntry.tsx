import { useRef, useState, type RefObject } from "react";
import { LoginRequired, ReadFailure } from "../../shared/ui/LoginRequired";
import { unauthenticated } from "../../shared/ui/LoginRequired.model";
import { useMe } from "../../core/session/me.service";
import {
  BUILD_ID,
  FEEDBACK_MAX_MESSAGE,
  feedbackPagePath,
  feedbackRunID,
  useSubmitFeedback,
  type FeedbackKind,
} from "./feedback.service";
import { KIND_LABEL, KIND_NOTE } from "./FeedbackEntry.model";

const runes = (s: string) => [...s].length;

function FeedbackMessageField({
  message,
  invalid,
  inputRef,
  pagePath,
  runID,
  onChange,
}: {
  message: string;
  invalid: string;
  inputRef: RefObject<HTMLTextAreaElement | null>;
  pagePath: string;
  runID: string | undefined;
  onChange: (value: string) => void;
}) {
  return (
    <>
      <p>
        <label htmlFor="feedback-message">發生了什麼事</label>
        <br />
        <textarea
          ref={inputRef}
          id="feedback-message"
          rows={4}
          cols={60}
          aria-invalid={invalid ? true : undefined}
          aria-describedby={
            invalid ? "feedback-context feedback-message-error" : "feedback-context"
          }
          value={message}
          onChange={(event) => onChange(event.target.value)}
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
            、你正在看的試跑紀錄 <code>{runID}</code>
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
    </>
  );
}

function FeedbackResult({
  invalid,
  send,
  message,
  submittedMessage,
}: {
  invalid: string;
  send: ReturnType<typeof useSubmitFeedback>;
  message: string;
  submittedMessage: string;
}) {
  return (
    <>
      {invalid && (
        <p role="alert" id="feedback-message-error">
          {invalid}
        </p>
      )}
      {send.error && (
        <ReadFailure error={send.error} what="回報">
          <p role="alert">
            {message === submittedMessage
              ? "送不出去。這份內容還留在上面，可以稍後再按一次送出；目前沒有第二條回報管道。"
              : "上一份回報未送出；目前內容也尚未送出。請確認內容後再試一次。"}
          </p>
        </ReadFailure>
      )}
      {send.isSuccess && (
        <p role="status">
          {message && message !== submittedMessage
            ? "上一份回報已收到；目前內容尚未送出。平台沒有回覆機制或查詢頁面。"
            : "已收到回報。平台沒有回覆機制或查詢頁面。"}
        </p>
      )}
    </>
  );
}

export function FeedbackEntry({
  pathname,
  embedded = false,
}: {
  pathname: string;
  embedded?: boolean;
}) {
  const me = useMe();
  const [kind, setKind] = useState<FeedbackKind>("blocking_issue");
  const [message, setMessage] = useState("");
  const [invalid, setInvalid] = useState("");
  const [submittedMessage, setSubmittedMessage] = useState("");
  const messageInput = useRef<HTMLTextAreaElement>(null);

  const pagePath = feedbackPagePath(pathname);
  const runID = feedbackRunID(pathname);
  const send = useSubmitFeedback();

  function submit() {
    send.reset();
    const trimmed = message.trim();
    if (trimmed === "") {
      setInvalid("請先寫下發生了什麼事。內容不能空白——只有這一段是你的話，其餘欄位都只是位置。");
      messageInput.current?.focus();
      return;
    }
    if (runes(trimmed) > FEEDBACK_MAX_MESSAGE) {
      setInvalid(
        `內容最多 ${FEEDBACK_MAX_MESSAGE} 字，目前 ${runes(trimmed)} 字。` +
          "請刪掉一些再送出，這樣才不會有一半被丟掉。",
      );
      messageInput.current?.focus();
      return;
    }
    setInvalid("");
    setSubmittedMessage(trimmed);
    send.mutate(
      { kind, message: trimmed, page_path: pagePath, run_id: runID, build_id: BUILD_ID },
      { onSuccess: () => setMessage((current) => (current === message ? "" : current)) },
    );
  }

  const content = (
    <>
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
                    onChange={() => {
                      setKind(k);
                      if (!send.isPending) send.reset();
                    }}
                  />{" "}
                  {KIND_LABEL[k]}
                </label>{" "}
                <span className="note">{KIND_NOTE[k]}</span>
              </p>
            ))}
          </fieldset>

          <FeedbackMessageField
            message={message}
            invalid={invalid}
            inputRef={messageInput}
            pagePath={pagePath}
            runID={runID}
            onChange={(value) => {
              setMessage(value);
              setInvalid("");
              if (!send.isPending) send.reset();
            }}
          />

          <p className="note">
            平台沒有回覆機制或查詢頁面；若希望有人聯絡，請在送出前於內容中留下聯絡方式。
          </p>

          <p>
            <button type="submit" disabled={send.isPending}>
              {send.isPending ? "送出中…" : "送出回報"}
            </button>
          </p>
        </form>
      )}

      <FeedbackResult
        invalid={invalid}
        send={send}
        message={message}
        submittedMessage={submittedMessage}
      />
    </>
  );

  if (embedded) return content;

  return (
    <details className="feedback-entry">
      <summary>回報問題</summary>
      {content}
    </details>
  );
}
