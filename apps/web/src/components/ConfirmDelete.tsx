import { useEffect, useRef, useState, type ReactNode } from "react";

export function ConfirmDelete({
  scopeId,
  scope,
  pending,
  onAsk,
  onConfirm,
  label = "刪除",
  confirmLabel = "確認刪除",
}: {
  scopeId: string;
  scope: ReactNode;
  pending: boolean;
  onAsk?: () => void;
  onConfirm: () => void;
  label?: string;
  confirmLabel?: string;
}) {
  const [confirming, setConfirming] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  useEffect(() => {
    if (!confirming && restoreFocus.current) {
      restoreFocus.current = false;
      trigger.current?.focus();
    }
  }, [confirming]);

  if (!confirming) {
    return (
      <button
        ref={trigger}
        type="button"
        onClick={() => {
          onAsk?.();
          setConfirming(true);
        }}
      >
        {label}
      </button>
    );
  }

  return (
    <>
      <span className="note confirm-scope" id={scopeId}>
        {scope}
      </span>{" "}
      <button
        key="confirm"
        type="button"
        className="destructive"
        autoFocus
        aria-describedby={scopeId}
        disabled={pending}
        onClick={onConfirm}
      >
        {pending ? "送出中…" : confirmLabel}
      </button>{" "}
      <button
        type="button"
        disabled={pending}
        onClick={() => {
          restoreFocus.current = true;
          setConfirming(false);
        }}
      >
        {pending ? "已送出，無法取消" : "取消"}
      </button>
    </>
  );
}
