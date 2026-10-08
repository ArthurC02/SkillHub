import { Timestamp } from "./Timestamp";

export function ListFreshness({
  inFlight,
  showWhenIdle = false,
  updatedAt,
  fetching,
  refetch,
  subject = "試跑",
  pollSeconds,
}: {
  inFlight: boolean;
  showWhenIdle?: boolean;
  updatedAt: number;
  fetching: boolean;
  refetch: () => unknown;
  subject?: string;
  pollSeconds?: number;
}) {
  if (!inFlight && !showWhenIdle) return null;
  return (
    <p className="note">
      {inFlight
        ? `有${subject}還在進行中；${pollSeconds ? `每 ${pollSeconds} 秒自動更新；` : ""}`
        : ""}
      這份{subject}清單上次取得於 <Timestamp at={new Date(updatedAt).toISOString()} relative />。{" "}
      <button type="button" disabled={fetching} onClick={() => void refetch()}>
        {fetching ? "重新整理中…" : "重新整理"}
      </button>
    </p>
  );
}
