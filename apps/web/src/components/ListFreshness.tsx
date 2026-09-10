import { Timestamp } from "./Timestamp";

export function ListFreshness({
  inFlight,
  updatedAt,
  fetching,
  refetch,
}: {
  inFlight: boolean;
  updatedAt: number;
  fetching: boolean;
  refetch: () => unknown;
}) {
  if (!inFlight) return null;
  return (
    <p className="note">
      有 Run 還在進行中；這份清單上次取得於{" "}
      <Timestamp at={new Date(updatedAt).toISOString()} relative />。{" "}
      <button type="button" disabled={fetching} onClick={() => void refetch()}>
        {fetching ? "重新整理中…" : "重新整理"}
      </button>
    </p>
  );
}
