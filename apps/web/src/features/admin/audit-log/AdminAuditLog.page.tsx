import { useOperatorAuditLog } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ACTION_LABEL } from "../admin.model";
import type { OperatorAuditEvent } from "../admin.service";
import { MetadataCell } from "./components/MetadataCell";

const RESOURCE_LABEL: Record<string, string> = {
  account: "帳號",
  credit_account: "點數帳戶",
  credit_entry: "點數分錄",
  dispatch: "派送",
  skill: "小工具",
};

function ActorCell({ event }: { event: OperatorAuditEvent }) {
  if (event.actor_kind === "agent") {
    return (
      <>
        平台 Agent <code>{event.actor_agent_id}</code>
      </>
    );
  }
  if (event.actor_kind === "person") return <code>{event.actor_user_id}</code>;
  return <>平台自動</>;
}

export function AdminAuditLog() {
  const log = useOperatorAuditLog();
  const rows = log.data?.pages.flatMap((page) => page.events) ?? [];

  return (
    <AdminPage heading="動作紀錄">
      {log.isPending && <Loading what="動作紀錄" />}
      <ReadFailure error={log.error} what="動作紀錄">
        <p role="alert">
          暫時無法讀取動作紀錄。{log.data ? "先前載入的內容已隱藏。" : "請稍後再試。"}
        </p>
        <button type="button" disabled={log.isFetching} onClick={() => void log.refetch()}>
          {log.isFetching ? "重新讀取中…" : "再試一次"}
        </button>
      </ReadFailure>
      {log.data && !log.error && (
        <ListFreshness
          inFlight={false}
          showWhenIdle
          updatedAt={log.dataUpdatedAt}
          fetching={log.isFetching}
          refetch={log.refetch}
          subject="動作紀錄"
        />
      )}
      {log.data &&
        !log.error &&
        (rows.length === 0 ? (
          <p>動作紀錄：0 筆。</p>
        ) : (
          <>
            <div className="table-scroll">
              <table className="responsive-table">
                <caption>全平台動作紀錄，新的在上面</caption>
                <thead>
                  <tr>
                    <th scope="col">時間</th>
                    <th scope="col">動作</th>
                    <th scope="col">行為者</th>
                    <th scope="col">對象</th>
                    <th scope="col">內容</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((event, index) => (
                    <tr key={`${event.action}-${index}`}>
                      <td data-label="時間">
                        <Timestamp at={event.occurred_at} />
                      </td>
                      <th scope="row" data-label="動作">
                        {ACTION_LABEL[event.action] ?? event.action}
                      </th>
                      <td data-label="行為者">
                        <ActorCell event={event} />
                      </td>
                      <td data-label="對象">
                        {RESOURCE_LABEL[event.resource_type] ?? event.resource_type}{" "}
                        <code>{event.resource_id ?? "不適用"}</code>
                        <p className="note">
                          工作區：
                          {event.workspace_id === null ? (
                            "不適用"
                          ) : (
                            <code>{event.workspace_id}</code>
                          )}
                        </p>
                      </td>
                      <td data-label="內容">
                        <MetadataCell metadata={event.metadata} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p className="note" role="status">
              已載入 {rows.length} 筆動作紀錄{log.hasNextPage ? "；還有更多。" : "。"}
            </p>
          </>
        ))}
      {log.hasNextPage && !log.error && (
        <button type="button" disabled={log.isFetchingNextPage} onClick={() => log.fetchNextPage()}>
          {log.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}
    </AdminPage>
  );
}
