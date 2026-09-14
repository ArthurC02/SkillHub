import { useOperatorAuditLog } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ACTION_LABEL } from "../admin.model";
import { MetadataCell } from "./components/MetadataCell";

const RESOURCE_LABEL: Record<string, string> = {
  account: "帳號",
  credit_account: "點數帳戶",
  credit_entry: "點數分錄",
  dispatch: "派送",
  skill: "Skill",
};

export function AdminAuditLog() {
  const log = useOperatorAuditLog();
  const rows = log.data?.pages.flatMap((page) => page.events) ?? [];

  return (
    <AdminPage heading="動作紀錄">
      {log.isPending && <Loading what="動作紀錄" />}
      <ReadFailure error={log.error} what="動作紀錄" />
      {log.data &&
        (rows.length === 0 ? (
          <p>operator 動作：0 筆。</p>
        ) : (
          <div className="table-scroll">
            <table>
              <caption>operator 動作，新的在上面</caption>
              <thead>
                <tr>
                  <th scope="col">時間</th>
                  <th scope="col">動作</th>
                  <th scope="col">operator</th>
                  <th scope="col">對象</th>
                  <th scope="col">內容</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((event, index) => (
                  <tr key={`${event.action}-${index}`}>
                    <td>
                      <Timestamp at={event.occurred_at} />
                    </td>
                    <td>{ACTION_LABEL[event.action] ?? event.action}</td>
                    <td>{event.actor_user_id ? <code>{event.actor_user_id}</code> : "平台自動"}</td>
                    <td>
                      {RESOURCE_LABEL[event.resource_type] ?? event.resource_type}{" "}
                      <code>{event.resource_id ?? "不適用"}</code>
                    </td>
                    <td>
                      <MetadataCell metadata={event.metadata} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      {log.hasNextPage && (
        <button type="button" disabled={log.isFetchingNextPage} onClick={() => log.fetchNextPage()}>
          {log.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}
    </AdminPage>
  );
}
