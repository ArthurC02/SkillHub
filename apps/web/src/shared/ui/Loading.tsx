import "./Loading.css";

export function Loading({ what, className }: { what: string; className?: string }) {
  return (
    <div
      role="status"
      data-loading=""
      className={["loading-state", className].filter(Boolean).join(" ")}
    >
      <span className="loading-label">載入{what}中…</span>
      <span className="loading-skeleton" aria-hidden="true">
        <span />
        <span />
        <span />
      </span>
    </div>
  );
}
