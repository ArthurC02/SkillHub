import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import {
  useCreditStatement,
  useCredits,
  type CreditStatementEntry,
} from "../../../core/session/credits.service";

function signedCredits(delta: number): string {
  return delta > 0 ? `+${delta}` : String(delta);
}

export function CreditStatement() {
  const credits = useCredits();
  const statement = useCreditStatement();
  const pages = statement.data?.pages ?? [];
  const entries = pages.flatMap((page) => page.entries);

  return (
    <>
      <ReadFailure error={credits.error} what="點數餘額" />
      {credits.data && (
        <p>
          目前餘額 <strong>{credits.data.balance_credits}</strong> 點
        </p>
      )}

      {statement.isPending && <Loading what="點數紀錄" />}
      <ReadFailure error={statement.error} what="點數紀錄" />
      {statement.data &&
        (entries.length === 0 ? (
          <p>還沒有任何點數進出。這裡是空的代表沒有發生過，不是紀錄被清掉了。</p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {entries.map((entry) => (
              <StatementRow key={entry.id} entry={entry} />
            ))}
          </ul>
        ))}
      {pages[0]?.note && <p className="note">{pages[0].note}</p>}
      {statement.hasNextPage && (
        <button
          type="button"
          disabled={statement.isFetchingNextPage}
          onClick={() => statement.fetchNextPage()}
        >
          {statement.isFetchingNextPage ? "載入中…" : "載入更早的紀錄"}
        </button>
      )}
    </>
  );
}

function StatementRow({ entry }: { entry: CreditStatementEntry }) {
  return (
    <li className="download-item">
      <p>
        {entry.run_id ? (
          <Link to="/runs/$runId" params={{ runId: entry.run_id }}>
            {entry.label}
          </Link>
        ) : (
          entry.label
        )}{" "}
        <strong>{signedCredits(entry.delta_credits)} 點</strong>
        {entry.estimated && <span className="badge badge-unverified">估計</span>}
      </p>
      <p className="note">
        <Timestamp at={entry.created_at} />
      </p>
    </li>
  );
}
