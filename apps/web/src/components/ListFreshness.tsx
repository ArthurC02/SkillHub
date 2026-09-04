import { Timestamp } from "./Timestamp";

/**
 * 設計 §2.12 第 6 條: a list that contains an in-flight row is an in-flight
 * screen, so it owes the reader the two things §2.12 第 3 條 asks of one — a
 * way to make it move (refresh) and a fact about how old it is (上次取得於).
 * Without them a row stuck on 「執行中」 cannot be told apart from a page that
 * has not been re-read in half an hour.
 *
 * Renders nothing when nothing on the list is still running: a finished list
 * does not age in a way that changes a decision, and a control that never
 * changes anything is noise (§2.13, E class that earns no place).
 *
 * `updatedAt` is TanStack Query's `dataUpdatedAt` — the moment this browser
 * last received the list, which is the honest "as of"; it is not a server
 * field, so it is not an `_at` value and the RAW_TIMESTAMP rule does not
 * apply. 「重新整理中…」 is on a11y.test.tsx's SELF_EXPLAINING list already
 * (RunTrace uses the same words), so the disabled state names its own cause.
 */
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
