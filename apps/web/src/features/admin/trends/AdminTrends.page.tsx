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
  type FunnelTrend,
  type Trend,
} from "../admin.service";
import { AdminPage } from "../components/AdminPage";
import { ENTRY_KIND, ACTION_LABEL, COST_KIND } from "../admin.model";
import { Timestamp } from "../../../shared/ui/Timestamp";
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
  const funnel = useTrend<FunnelTrend>("funnel", days);
  const reads = [cost, credits, runs, actions, funnel];
  const available = reads.filter((query) => query.data && !query.error);
  const fetching = reads.some((query) => query.isFetching);
  const stages = funnel.data?.stages ?? [];
  const range = available[0]?.data;

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
      {reads.some((query) => query.data || query.error) && (
        <p className="note">
          {available.length > 0 ? (
            <>
              已取得 {available.length}/5 組趨勢；最早取得於{" "}
              <Timestamp
                at={new Date(
                  Math.min(...available.map((query) => query.dataUpdatedAt)),
                ).toISOString()}
                relative
              />
              。{" "}
            </>
          ) : (
            "目前沒有可用趨勢。 "
          )}
          <button
            type="button"
            disabled={fetching}
            onClick={() => void Promise.all(reads.map((query) => query.refetch()))}
          >
            {fetching ? "重新整理中…" : "重新整理五組趨勢"}
          </button>
        </p>
      )}
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
        heading="每天建立的試跑紀錄（依目前狀態）"
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
      <TrendSection
        heading="漏斗各段每天到達的數量"
        query={funnel}
        value={countOf}
        format={String}
        labels={Object.fromEntries(stages.map((stage) => [stage.key, stage.label]))}
      >
        {stages.length > 0 && (
          <dl>
            {stages.map((stage) => (
              <div key={stage.key}>
                <dt>{stage.label}</dt>
                <dd>{stage.grain}</dd>
              </div>
            ))}
          </dl>
        )}
      </TrendSection>
    </AdminPage>
  );
}
