import { useState, type ReactNode } from "react";
import { WriteFailure } from "./WriteFailure";

export function ActionForm({
  id,
  submitLabel,
  pending,
  error,
  done,
  ready = true,
  onSubmit,
  children,
}: {
  id: string;
  submitLabel: string;
  pending: boolean;
  error: unknown;
  done?: ReactNode;
  ready?: boolean;
  onSubmit: (note: string) => void;
  children?: ReactNode;
}) {
  const [note, setNote] = useState("");
  const blocked = note.trim() === "" || !ready;
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!blocked) onSubmit(note.trim());
      }}
    >
      {children}
      <div className="field">
        <label htmlFor={`${id}-note`}>理由（必填，會寫進動作紀錄）</label>
        <textarea
          id={`${id}-note`}
          value={note}
          onChange={(event) => setNote(event.target.value)}
        />
      </div>
      <button
        type="submit"
        disabled={blocked || pending}
        aria-describedby={blocked ? `${id}-why` : undefined}
      >
        {pending ? "送出中…" : submitLabel}
      </button>
      {blocked && (
        <p id={`${id}-why`} className="note">
          「{submitLabel}」要等上面的欄位都填好。
        </p>
      )}
      {done && <p role="status">{done}</p>}
      <WriteFailure error={error} />
    </form>
  );
}
