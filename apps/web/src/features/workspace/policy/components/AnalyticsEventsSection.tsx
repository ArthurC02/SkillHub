import { Loading } from "../../../../shared/ui/Loading";
import type { DataRetentionPolicy } from "../../../../core/api/types";

export function AnalyticsEventsSection({
  isPending,
  error,
  data,
}: {
  isPending: boolean;
  error: Error | null;
  data: DataRetentionPolicy | undefined;
}) {
  return (
    <>
      {isPending && <Loading what="分析事件政策" />}
      {error && (
        <p role="alert">
          無法讀取分析事件政策：{error.message}
          。讀不到不等於沒有收集，這一頁不會替伺服器回答這個問題。
        </p>
      )}

      {data && (
        <>
          {data.collecting ? (
            <p>
              這個部署<strong>有</strong>在收集下面 {data.events.length} 個事件，保存
              <strong>{data.retention_days} 天</strong>
              ，到期後刪除。分析事件用的 cookie 也是同一個期限。
            </p>
          ) : (
            <p>
              這個部署<strong>目前不收集</strong>
              使用行為分析事件：沒有設定保存期限，所以一列都不寫，cookie 也不發。
              保存期限定案之前不會開始收集——這是規則，不是還沒做完。下面仍然列出
              {data.events.length} 個事件，因為「現在不收」本身就是需要說清楚的一件事。
            </p>
          )}

          <p className="note">{data.note}</p>

          <div className="table-scroll">
            <table className="compare-table responsive-table" data-role="evidence">
              <caption>
                全部只有這 {data.events.length} 個事件。要再加一個，得先說明既有的資料表
                為什麼答不出那個問題。
              </caption>
              <thead>
                <tr>
                  <th scope="col">事件</th>
                  <th scope="col">什麼時候產生</th>
                  <th scope="col">記了哪些欄位</th>
                  <th scope="col">沒有記什麼</th>
                </tr>
              </thead>
              <tbody>
                {data.events.map((event) => (
                  <tr key={event.name}>
                    <th scope="row" data-label="事件">
                      <code>{event.name}</code>
                    </th>
                    <td data-label="什麼時候產生">{event.when}</td>
                    <td data-label="記了哪些欄位">
                      <ul className="risk-list">
                        {event.attributes.map((attribute) => (
                          <li key={attribute}>
                            <code>{attribute}</code>
                          </li>
                        ))}
                      </ul>
                    </td>
                    <td data-label="沒有記什麼">{event.not_recorded}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <h2>回報問題的資料</h2>
          <p>{data.feedback.what}</p>
          <ul className="risk-list">
            {data.feedback.collected.map((column) => (
              <li key={column}>
                <code>{column}</code>
              </li>
            ))}
          </ul>
          <p>{data.feedback.free_text}</p>
          <p>{data.feedback.page_path}</p>
          <p>{data.feedback.run_id}</p>
          <p>{data.feedback.on_account_deletion}</p>
          {data.feedback.retention_days !== null ? (
            <p>
              保存 <strong>{data.feedback.retention_days} 天</strong>，到期後刪除。
            </p>
          ) : (
            <p className="note">保存期限：尚未定值。{data.feedback.note}</p>
          )}
        </>
      )}
    </>
  );
}
