import { useCostStatistics, usd } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { COST_KIND } from "../admin.model";

export function AdminCostStatistics() {
  const stats = useCostStatistics();
  const rows = stats.data?.statistics ?? [];

  return (
    <AdminPage heading="成本統計" lede="與開始前檢查、會話估價讀的是同一組數字；不含使用者維度。">
      {stats.isPending && <Loading what="成本統計" />}
      <ReadFailure error={stats.error} what="成本統計" />
      {stats.data &&
        (rows.length === 0 ? (
          <p>統計窗：0 個。每日統計跑過之後才會有。</p>
        ) : (
          <div className="table-scroll">
            <table>
              <caption>每一種呼叫最新的統計窗（美元）</caption>
              <thead>
                <tr>
                  <th scope="col">種類</th>
                  <th scope="col">統計窗結束</th>
                  <th scope="col">樣本數</th>
                  <th scope="col">p50</th>
                  <th scope="col">p90</th>
                  <th scope="col">p95</th>
                  <th scope="col">最大</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.kind}>
                    <th scope="row">{COST_KIND[row.kind] ?? row.kind}</th>
                    <td>
                      <Timestamp at={row.window_end} />
                    </td>
                    <td>{row.sample_count}</td>
                    <td>{usd(row.p50_usd_micros)}</td>
                    <td>{usd(row.p90_usd_micros)}</td>
                    <td>{usd(row.p95_usd_micros)}</td>
                    <td>{usd(row.max_usd_micros)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
    </AdminPage>
  );
}
