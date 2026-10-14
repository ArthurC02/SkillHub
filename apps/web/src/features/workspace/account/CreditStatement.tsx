import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { canRetainLoadedPage } from "../../../core/api/client";
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
  const nextPageFailure = canRetainLoadedPage(statement.error, statement.isFetchNextPageError);
  const data = statement.error && !nextPageFailure ? undefined : statement.data;
  const pages = data?.pages ?? [];
  const entries = pages.flatMap((page) => page.entries);

  return (
    <>
      <section aria-label="點數餘額">
        {credits.isPending && <Loading what="點數餘額" />}
        <ReadFailure
          error={credits.error}
          what="點數餘額"
          retry={() => void credits.refetch()}
          retrying={credits.isFetching}
          focusTarget="account-credits-heading"
        />
        {credits.data && !credits.error && (
          <p>
            目前餘額 <strong>{credits.data.balance_credits}</strong> 點
          </p>
        )}
      </section>

      <section aria-label="點數紀錄">
        {statement.isPending && <Loading what="點數紀錄" />}
        <ReadFailure
          error={statement.error}
          what="點數紀錄"
          retry={() => void (nextPageFailure ? statement.fetchNextPage() : statement.refetch())}
          retrying={statement.isFetching}
          focusTarget="account-credits-heading"
        >
          {nextPageFailure && (
            <p role="alert">
              更早的點數紀錄讀取失敗；以下僅保留已載入的紀錄，尚未讀完。請再試一次。
            </p>
          )}
        </ReadFailure>
        {data &&
          (entries.length === 0 ? (
            <p>
              {statement.hasNextPage || nextPageFailure
                ? "已載入的紀錄中沒有點數進出；尚未讀完，無法確認全部紀錄。"
                : "還沒有任何點數進出。這裡是空的代表沒有發生過，不是紀錄被清掉了。"}
            </p>
          ) : (
            <ul className="download-list" data-role="evidence">
              {entries.map((entry) => (
                <StatementRow key={entry.id} entry={entry} />
              ))}
            </ul>
          ))}
        {pages[0]?.note && <p className="note">{pages[0].note}</p>}
        {statement.hasNextPage && !statement.error && (
          <button
            type="button"
            disabled={statement.isFetchingNextPage}
            onClick={() => statement.fetchNextPage()}
          >
            {statement.isFetchingNextPage ? "載入中…" : "載入更早的紀錄"}
          </button>
        )}
      </section>
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
