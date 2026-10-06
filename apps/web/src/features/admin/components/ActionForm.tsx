import { useState, type ReactNode } from "react";
import { WriteFailure } from "./WriteFailure";

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
  unavailableReason?: string;
  onSubmit: (note: string) => void;
  children?: ReactNode;
}) {
  const [note, setNote] = useState("");
  const [submitted, setSubmitted] = useState<{ note: string; contextKey: string } | null>(null);
  const blocked = note.trim() === "" || !ready;
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
        <label htmlFor={`${id}-note`}>理由（必填，會寫進動作紀錄）</label>
        <textarea
          id={`${id}-note`}
          value={note}
          onChange={(event) => {
            setNote(event.target.value);
            setSubmitted(null);
          }}
          readOnly={pending || readOnly}
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
          {!ready && unavailableReason
            ? unavailableReason
            : `「${submitLabel}」要等上面的欄位都填好。`}
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
