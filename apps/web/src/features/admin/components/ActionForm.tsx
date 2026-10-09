import { useState, type ReactNode } from "react";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
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
  readOnlyNote = pending,
  blockedReason,
  confirmationScope,
  confirmationLabel,
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
  readOnlyNote?: boolean;
  blockedReason?: string;
  confirmationScope?: ReactNode;
  confirmationLabel?: string;
  onSubmit: (note: string) => void;
  children?: ReactNode;
}) {
  const [note, setNote] = useState("");
  const [submitted, setSubmitted] = useState<{ note: string; contextKey: string } | null>(null);
  const resultMatches = submitted?.note === note.trim() && submitted.contextKey === contextKey;
  const completed = Boolean(done && resultMatches);
  const blocked = note.trim() === "" || !ready || completed;
  const send = () => {
    if (blocked || pending) return;
    setSubmitted({ note: note.trim(), contextKey });
    onSubmit(note.trim());
  };

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!confirmationScope) send();
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
          readOnly={readOnlyNote}
        />
      </div>
      {confirmationScope ? (
        <ConfirmDelete
          scopeId={`${id}-scope`}
          scope={confirmationScope}
          pending={pending}
          disabled={blocked}
          disabledReasonId={`${id}-why`}
          label={submitLabel}
          confirmLabel={confirmationLabel}
          onConfirm={send}
        />
      ) : (
        <button
          type="submit"
          className={tone}
          disabled={blocked || pending}
          aria-describedby={blocked ? `${id}-why` : undefined}
        >
          {pending ? "送出中…" : submitLabel}
        </button>
      )}
      {blocked && (
        <p id={`${id}-why`} className="note">
          {completed
            ? "已完成這筆操作；修改上方欄位或理由後再送出，會建立另一筆操作。"
            : (blockedReason ?? `「${submitLabel}」要等上面的欄位都填好。`)}
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
