import "./Loading.css";

export function Loading({
  what,
  className,
  paused = false,
}: {
  what: string;
  className?: string;
  paused?: boolean;
}) {
  return (
    <div
      role="status"
      data-loading=""
      className={["loading-state", className].filter(Boolean).join(" ")}
    >
      <span className="loading-label">
        {paused ? `等待連線以讀取${what}；恢復連線後會繼續讀取。` : `載入${what}中…`}
      </span>
      {!paused && (
        <span className="loading-skeleton" aria-hidden="true">
          <span />
          <span />
          <span />
        </span>
      )}
    </div>
  );
}
