import { Link } from "@tanstack/react-router";
import { useFeatureAvailability } from "../../shared/featureAvailability";
import { ListFreshness } from "../../shared/ui/ListFreshness";
import { Loading } from "../../shared/ui/Loading";
import { ReadFailure } from "../../shared/ui/LoginRequired";
import { Timestamp } from "../../shared/ui/Timestamp";
import {
  activityUnavailable,
  useActivity,
  type ActivityClassification,
  type ActivityContinuation,
  type ActivityItem,
  type ActivitySource,
} from "./activity.service";
import "./Activity.page.css";

const GROUPS: Array<{
  key: ActivityClassification;
  title: string;
  note: string;
}> = [
  { key: "needs_attention", title: "需要你處理", note: "先看清楚狀態，再回到原物件做決定。" },
  {
    key: "in_progress",
    title: "平台處理中",
    note: "可以安全離開；重新整理會讀取各 owner 的最新事實。",
  },
  { key: "recent", title: "最近完成", note: "已結束或已確認的工作，依權威活動時間排列。" },
  { key: "neutral", title: "其他活動", note: "目前不需要動作，但仍保留可回到原物件的脈絡。" },
];

const SOURCE_LABELS: Record<ActivitySource, string> = {
  run: "Run",
  evaluation: "Evaluation",
  creation: "Creation",
  packaging: "Packaging",
  publishing: "Publishing",
};

const KIND_LABELS: Record<string, string> = {
  run: "試跑與評估",
  creation_session: "Studio 創作",
  packaging_artifact: "交付套件",
  skill_publication: "Skill 發佈",
};

export function Activity() {
  const activity = useActivity();
  const availability = useFeatureAvailability();
  const items = activity.data?.pages.flatMap((page) => page.items) ?? [];
  const unavailable = activityUnavailable(activity.error);
  const inProgress = items.some((item) => item.classification === "in_progress");

  return (
    <section className="activity-page">
      <header className="activity-hero">
        <div>
          <p className="activity-eyebrow">Workspace activity</p>
          <h1>活動</h1>
          <p>把正在執行、等待決定與最近完成的工作放在同一條可信時間線。</p>
        </div>
        {activity.data && <ActivitySummary items={items} />}
      </header>

      <p className="note" data-role="caveat">
        這裡合併五個來源，但不取代各物件的完整證據；狀態與時間都由原本的 owner 提供。
      </p>

      {activity.isPending && <Loading what="Workspace 活動" />}
      <ReadFailure error={activity.error} what="Workspace 活動">
        {unavailable && <UnavailableSources sources={unavailable.unavailable_sources} />}
      </ReadFailure>
      {activity.data && (
        <ListFreshness
          inFlight={inProgress}
          updatedAt={activity.dataUpdatedAt}
          fetching={activity.isFetching && !activity.isFetchingNextPage}
          refetch={activity.refetch}
          subject="工作"
        />
      )}

      {activity.data &&
        (items.length === 0 ? (
          <div className="activity-empty">
            <h2>目前沒有活動</h2>
            <p>五個來源都已讀取，這代表目前沒有可列出的工作，不是資料載入不完整。</p>
            <Link className="action-secondary" to="/library">
              前往資產庫
            </Link>
          </div>
        ) : (
          <div className="activity-groups">
            {GROUPS.map((group) => {
              const groupItems = items.filter((item) => item.classification === group.key);
              if (groupItems.length === 0) return null;
              return (
                <section className="activity-group" key={group.key} data-classification={group.key}>
                  <header>
                    <div>
                      <h2>{group.title}</h2>
                      <p className="note">{group.note}</p>
                    </div>
                    <span className="activity-count">{groupItems.length}</span>
                  </header>
                  <ol className="activity-list">
                    {groupItems.map((item) => (
                      <ActivityRow
                        key={`${item.kind}:${item.source_id}`}
                        item={item}
                        creationExposed={availability.creation}
                      />
                    ))}
                  </ol>
                </section>
              );
            })}
          </div>
        ))}

      {activity.hasNextPage && (
        <button
          type="button"
          disabled={activity.isFetchingNextPage}
          onClick={() => void activity.fetchNextPage()}
        >
          {activity.isFetchingNextPage ? "載入中…" : "載入更多活動"}
        </button>
      )}
    </section>
  );
}

function ActivitySummary({ items }: { items: ActivityItem[] }) {
  const count = (classification: ActivityClassification) =>
    items.filter((item) => item.classification === classification).length;
  return (
    <dl className="activity-summary" aria-label="活動摘要">
      <div>
        <dt>完整來源</dt>
        <dd>5 / 5</dd>
      </div>
      <div>
        <dt>需要處理</dt>
        <dd>{count("needs_attention")}</dd>
      </div>
      <div>
        <dt>進行中</dt>
        <dd>{count("in_progress")}</dd>
      </div>
    </dl>
  );
}

function UnavailableSources({ sources }: { sources: ActivitySource[] }) {
  return (
    <div className="activity-unavailable" role="alert">
      <strong>暫時無法讀取完整活動。</strong>
      <p>
        未能讀取：{sources.map((source) => SOURCE_LABELS[source]).join("、")}。沒有顯示部分結果。
      </p>
    </div>
  );
}

function ActivityRow({ item, creationExposed }: { item: ActivityItem; creationExposed: boolean }) {
  return (
    <li>
      <div className="activity-kind">{KIND_LABELS[item.kind] ?? "其他活動"}</div>
      <div className="activity-body">
        <strong>{item.summary}</strong>
        <p className="activity-status">{item.status.label}</p>
        {item.status.note && <p className="note">{item.status.note}</p>}
        <p className="note">
          更新於 <Timestamp at={item.activity_at} relative />
        </p>
      </div>
      <ActivityAction continuation={item.continuation} creationExposed={creationExposed} />
    </li>
  );
}

function ActivityAction({
  continuation,
  creationExposed,
}: {
  continuation: ActivityContinuation;
  creationExposed: boolean;
}) {
  if (continuation.kind === "run" && continuation.run_id) {
    return (
      <Link className="action-secondary" to="/runs/$runId" params={{ runId: continuation.run_id }}>
        查看 Run
      </Link>
    );
  }
  if (continuation.kind === "creation_session" && continuation.session_id) {
    return creationExposed ? (
      <Link
        className="action-secondary"
        to="/workspace/creations"
        search={{ session: continuation.session_id }}
      >
        繼續創作
      </Link>
    ) : (
      <span className="note">Studio 目前未開放</span>
    );
  }
  if (continuation.kind === "packaging_artifact" && continuation.artifact_id) {
    return (
      <Link
        className="action-secondary"
        to="/workspace/downloads"
        search={{ artifact: continuation.artifact_id }}
      >
        查看套件
      </Link>
    );
  }
  if (
    continuation.kind === "skill_publication" &&
    continuation.publisher &&
    continuation.publication_name
  ) {
    return (
      <Link
        className="action-secondary"
        to="/workspace/downloads"
        search={{ publication: `${continuation.publisher}/${continuation.publication_name}` }}
      >
        查看發佈
      </Link>
    );
  }
  return <span className="note">目前沒有可用的續作入口</span>;
}
