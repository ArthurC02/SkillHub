import { useId, useState, type KeyboardEvent, type ReactNode } from "react";

export function Tip({ anchor, children }: { anchor: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const id = useId();

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key === "Escape" && open) {
      e.stopPropagation();
      setOpen(false);
    }
  };

  return (
    <span className="tip" data-tip onKeyDown={onKeyDown}>
      <button
        type="button"
        className="tip-trigger"
        aria-expanded={open}
        aria-controls={id}
        onClick={(e) => {
          // WebKit doesn't focus a button on click, so without this the Escape
          // handler above never sees the keydown once the tip is open.
          e.currentTarget.focus();
          setOpen((v) => !v);
        }}
      >
        <svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12" focusable="false">
          <circle cx="6" cy="6" r="5" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <path d="M6 5.2v3.3M6 3.4v.3" stroke="currentColor" strokeWidth="1.5" fill="none" />
        </svg>
        {anchor}
      </button>
      <p className="tip-content" id={id} hidden={!open} data-role="teaching">
        {children}
      </p>
    </span>
  );
}
