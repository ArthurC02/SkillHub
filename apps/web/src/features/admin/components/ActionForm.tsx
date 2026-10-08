import { useState, type ReactNode } from "react";
import { operatorNoteBytes } from "../admin.model";
import { WriteFailure } from "./WriteFailure";

function noteLimitLabel(maxNoteBytes?: number): string {
  return maxNoteBytes === undefined ? "" : `，最多 ${maxNoteBytes} 位元組`;
}

function blockReason({
  tooLong,
  noteBytes,
  maxNoteBytes,
  ready,
  unavailableReason,
  submitLabel,
}: {
  tooLong: boolean;
  noteBytes: number;
  maxNoteBytes: number | undefined;
  ready: boolean;
  unavailableReason: string | undefined;
  submitLabel: string;
}): string {
  if (tooLong) {
    return `理由太長：目前 ${noteBytes} 位元組，上限 ${maxNoteBytes} 位元組。請縮短後再送出。`;
  }
  if (!ready && unavailableReason) {
    return unavailableReason;
  }
  return `「${submitLabel}」要等上面的欄位都填好。`;
}

export function ActionForm({
  id,
  submitLabel,
  pending,
  error,
  done,
  contextKey = "",
  tone,
  ready = true,
  readOnly = false,
  maxNoteBytes,
  unavailableReason,
  onSubmit,
  children,
}: {
  id: string;
  submitLabel: string;
  pending: boolean;
  error: unknown;
  done?: ReactNode;
  contextKey?: string;
  tone?: "caution";
  ready?: boolean;
  readOnly?: boolean;
  maxNoteBytes?: number;
  unavailableReason?: string;
  onSubmit: (note: string) => void;
  children?: ReactNode;
}) {
  const [note, setNote] = useState("");
  const [submitted, setSubmitted] = useState<{ note: string; contextKey: string } | null>(null);
  const noteBytes = operatorNoteBytes(note);
  const tooLong = maxNoteBytes !== undefined && noteBytes > maxNoteBytes;
  const blocked = note.trim() === "" || !ready || tooLong;
  const resultMatches = submitted?.note === note.trim() && submitted.contextKey === contextKey;

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!blocked && !pending) {
          setSubmitted({ note: note.trim(), contextKey });
          onSubmit(note.trim());
        }
      }}
    >
      {children}
      <div className="field">
        <label htmlFor={`${id}-note`}>
          理由（必填{noteLimitLabel(maxNoteBytes)}，會寫進動作紀錄）
        </label>
        <textarea
          id={`${id}-note`}
          value={note}
          onChange={(event) => {
            setNote(event.target.value);
            setSubmitted(null);
          }}
          readOnly={pending || readOnly}
          aria-invalid={tooLong}
          aria-describedby={tooLong ? `${id}-why` : undefined}
        />
      </div>
      <button
        type="submit"
        className={tone}
        disabled={blocked || pending}
        aria-describedby={blocked ? `${id}-why` : undefined}
      >
        {pending ? "送出中…" : submitLabel}
      </button>
      {blocked && (
        <p id={`${id}-why`} className="note">
          {blockReason({ tooLong, noteBytes, maxNoteBytes, ready, unavailableReason, submitLabel })}
        </p>
      )}
      {done && resultMatches && (
        <p className="notice notice-success" role="status">
          {done}
        </p>
      )}
      <WriteFailure error={resultMatches && !pending ? error : undefined} />
    </form>
  );
}
