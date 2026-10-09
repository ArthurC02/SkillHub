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
  type TrendDays,
} from "../admin.service";
import { AdminPage } from "../components/AdminPage";
import { ENTRY_KIND, ACTION_LABEL, COST_KIND } from "../admin.model";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { TrendSection } from "./components/TrendSection";

const usdAmount = (micros: number) => usd(micros);

const creditAmount = (credits: number) => `${credits} 點`;

const countOf = (bucket: DailyCount) => bucket.count;

const totalOf = (bucket: DailyAmount) => bucket.total;

function TrendTopics() {
  return (
    <nav aria-label="趨勢主題" className="category-nav">
      <a className="chip" href="#admin-trend-cost">
        成本
      </a>
      <a className="chip" href="#admin-trend-credits">
        點數
      </a>
      <a className="chip" href="#admin-trend-runs">
        試跑
      </a>
      <a className="chip" href="#admin-trend-actions">
        後台動作
      </a>
      <a className="chip" href="#admin-trend-funnel">
        漏斗
      </a>
    </nav>
  );
}

function TrendRangeStatus({ ranges, days }: { ranges: Array<Trend | undefined>; days: TrendDays }) {
  const range = ranges[0];
  if (!range) return null;
  const mismatch = ranges.some(
    (item) =>
      item &&
      (item.from !== range.from ||
        item.to !== range.to ||
        daysOf(item.from, item.to).length !== days),
  );
  return mismatch ? (
    <p role="alert" className="notice notice-warning">
      取得的趨勢日期範圍與選擇不一致，請重新整理後再比較。
    </p>
  ) : (
    <p>
      {range.from} 到 {range.to}（UTC），共 {daysOf(range.from, range.to).length} 天。
    </p>
  );
}

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

  return (
    <AdminPage
      heading="趨勢"
      lede="依 UTC 日期分組，台灣時間早上八點換日；只有彙總，不指向任何帳號。"
    >
      <nav aria-label="時間範圍" className="category-nav">
        {TREND_DAYS.map((n) => (
          <Link
            key={n}
            to="/admin/trends"
            search={{ days: n }}
            className="chip"
            aria-current={n === days ? "page" : undefined}
          >
            {n} 天
          </Link>
        ))}
      </nav>
      <TrendRangeStatus ranges={available.map((query) => query.data)} days={days} />
      {reads.some((query) => query.data || query.error) && (
        <p className="note">
          {available.length > 0 ? (
            <>
              已取得 {available.length}/{reads.length} 組 ·{" "}
              <Timestamp
                at={new Date(
                  Math.min(...available.map((query) => query.dataUpdatedAt)),
                ).toISOString()}
              />
              。{" "}
            </>
          ) : (
            "目前沒有可用趨勢。 "
          )}
          <button
            type="button"
            disabled={fetching}
            aria-label={fetching ? undefined : "重新整理五組趨勢"}
            onClick={() => void Promise.all(reads.map((query) => query.refetch()))}
          >
            {fetching ? "重新整理中…" : "重新整理"}
          </button>
        </p>
      )}
      <TrendTopics />
      <TrendSection
        id="admin-trend-cost"
        heading="每日成本（美元，含估計值）"
        query={cost}
        value={totalOf}
        format={usdAmount}
        labels={COST_KIND}
        valueHeading="美元"
      />
      <TrendSection
        id="admin-trend-credits"
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
        id="admin-trend-runs"
        heading="每天建立的試跑紀錄（依目前狀態）"
        query={runs}
        value={countOf}
        format={String}
        labels={RUN_STATUS_LABEL}
      />
      <TrendSection
        id="admin-trend-actions"
        heading="每日 operator 動作"
        query={actions}
        value={countOf}
        format={String}
        labels={ACTION_LABEL}
      />
      <TrendSection
        id="admin-trend-funnel"
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
