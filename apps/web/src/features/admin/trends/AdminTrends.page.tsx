import { RUN_STATUS_LABEL } from "../../runs";
import { Link, useSearch } from "@tanstack/react-router";
import {
  usd,
  useTrend,
  daysOf,
  TREND_DAYS,
  type CreditTrend,
  type DailyAmount,
  type DailyCount,
  type Trend,
} from "../admin.service";
import { AdminPage } from "../components/AdminPage";
import { ENTRY_KIND, ACTION_LABEL, COST_KIND } from "../admin.model";
import { TrendSection } from "./components/TrendSection";

const usdAmount = (micros: number) => usd(micros);

const creditAmount = (credits: number) => `${credits} 點`;

const countOf = (bucket: DailyCount) => bucket.count;

const totalOf = (bucket: DailyAmount) => bucket.total;

export function AdminTrends() {
  const { days = 30 } = useSearch({ from: "/admin/trends" });
  const cost = useTrend<Trend<DailyAmount>>("cost", days);
  const credits = useTrend<CreditTrend>("credits", days);
  const runs = useTrend<Trend>("runs", days);
  const actions = useTrend<Trend>("operator-actions", days);
  const range = [cost, credits, runs, actions].find((query) => query.data)?.data;

  return (
    <AdminPage
      heading="趨勢"
      lede="依 UTC 日期分組，台灣時間早上八點換日；只有彙總，不指向任何帳號。"
    >
      <nav aria-label="時間範圍" className="category-nav">
        {TREND_DAYS.map((n) => (
          <Link key={n} to="/admin/trends" search={{ days: n }} className="chip">
            {n} 天
          </Link>
        ))}
      </nav>
      {range && (
        <p>
          {range.from} 到 {range.to}（UTC），共 {daysOf(range.from, range.to).length} 天。
        </p>
      )}
      <TrendSection
        heading="每日成本（美元，含估計值）"
        query={cost}
        value={totalOf}
        format={usdAmount}
        labels={COST_KIND}
        valueHeading="美元"
      />
      <TrendSection
        heading="每日點數異動（淨額）"
        query={credits}
        value={totalOf}
        format={creditAmount}
        labels={ENTRY_KIND}
        valueHeading="點數"
      >
        {credits.data && <p>全平台目前餘額總和：{credits.data.balance_total} 點。</p>}
      </TrendSection>
      <TrendSection
        heading="每天建立的 Run（依目前狀態）"
        query={runs}
        value={countOf}
        format={String}
        labels={RUN_STATUS_LABEL}
      />
      <TrendSection
        heading="每日 operator 動作"
        query={actions}
        value={countOf}
        format={String}
        labels={ACTION_LABEL}
      />
    </AdminPage>
  );
}
