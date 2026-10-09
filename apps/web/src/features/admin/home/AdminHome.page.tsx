import { Link } from "@tanstack/react-router";
import {
  canKeepLoadedFindings,
  useDispatchStatus,
  useExposureQueue,
  usePlatformAgentFindings,
  usePlatformAgentProposals,
  type DispatchStatus,
} from "../admin.service";
import { dispatchState } from "../admin.model";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import "./AdminHome.page.css";

function priorityText(value: string | undefined, error: Error | null): string {
  if (error) return "無法取得";
  return value ?? "讀取中";
}

function dispatchPriority(status: DispatchStatus | undefined, error: Error | null) {
  if (!status || error) return undefined;
  const state = dispatchState(status);
  if (state === "stopped") return { label: "停止派送", tone: "halt" };
  if (state === "partial") {
    return { label: `仍在派送；${status.halts.length} 個煞車`, tone: "pending" };
  }
  return { label: "正在派送", tone: undefined };
}

function findingPriorityRead(findings: ReturnType<typeof usePlatformAgentFindings>) {
  const error = canKeepLoadedFindings(findings.error, findings.isFetchNextPageError)
    ? null
    : findings.error;
  return { error, data: error ? undefined : findings.data };
}

function Priorities() {
  const dispatch = useDispatchStatus();
  const proposals = usePlatformAgentProposals();
  const findings = usePlatformAgentFindings();
  const exposure = useExposureQueue();
  const dispatchSummary = dispatchPriority(dispatch.data, dispatch.error);
  const { error: findingReadError, data: readableFindings } = findingPriorityRead(findings);
  const fetching =
    dispatch.isFetching || proposals.isFetching || findings.isFetching || exposure.isFetching;
  const complete =
    readableFindings && [dispatch, proposals, exposure].every((read) => read.data && !read.error);
  const oldestAt = Math.min(
    dispatch.dataUpdatedAt,
    proposals.dataUpdatedAt,
    findings.dataUpdatedAt,
    exposure.dataUpdatedAt,
  );

  return (
    <section className="admin-home-priorities" aria-label="目前需留意">
      <header>
        <h2>目前需留意</h2>
        <button
          type="button"
          disabled={fetching}
          onClick={() =>
            void Promise.all([
              dispatch.refetch(),
              proposals.refetch(),
              findings.refetch(),
              exposure.refetch(),
            ])
          }
        >
          {fetching ? "更新中…" : "重新整理狀態"}
        </button>
      </header>
      {complete && (
        <p className="note">
          四項狀態最早取得於 <Timestamp at={new Date(oldestAt).toISOString()} relative />。
        </p>
      )}
      <div className="admin-home-priority-list">
        <Link
          to="/admin/dispatch"
          className="admin-home-priority"
          data-state={dispatchSummary?.tone}
        >
          <span>派送狀態</span>
          <strong>{priorityText(dispatchSummary?.label, dispatch.error)}</strong>
        </Link>
        <Link
          to="/admin/agents"
          search={{}}
          hash="admin-agent-proposals"
          className="admin-home-priority"
          data-state={
            !proposals.error && proposals.data && proposals.data.total > 0 ? "pending" : undefined
          }
        >
          <span>平台 Agent 提案</span>
          <strong>
            {priorityText(proposals.data && `${proposals.data.total} 件待核准`, proposals.error)}
          </strong>
        </Link>
        <Link
          to="/admin/agents"
          search={{}}
          hash="admin-agent-findings"
          className="admin-home-priority"
          data-state={
            readableFindings &&
            readableFindings.pages[0].counts.open + readableFindings.pages[0].counts.acknowledged >
              0
              ? "pending"
              : undefined
          }
        >
          <span>平台 Agent 待辦</span>
          <strong>
            {priorityText(
              readableFindings &&
                `${readableFindings.pages[0].counts.open + readableFindings.pages[0].counts.acknowledged} 件待辦`,
              findingReadError,
            )}
          </strong>
        </Link>
        <Link
          to="/admin/exposure"
          search={{}}
          className="admin-home-priority"
          data-state={
            !exposure.error && exposure.data && exposure.data.publications.length > 0
              ? "pending"
              : undefined
          }
        >
          <span>曝光審核</span>
          <strong>
            {priorityText(
              exposure.data && `${exposure.data.publications.length} 件待審`,
              exposure.error,
            )}
          </strong>
        </Link>
      </div>
    </section>
  );
}

export function AdminHome() {
  return (
    <AdminPage
      heading="營運後台"
      lede="先確認派送與待處理事項，再依工作目的進入平台治理或日常營運。"
    >
      <Priorities />
      <div className="admin-home-sections">
        <GovernanceSection />
        <OperationsSection />
      </div>
    </AdminPage>
  );
}

function GovernanceSection() {
  return (
    <section className="admin-home-section">
      <header>
        <p className="admin-home-eyebrow">Governing · Decisions</p>
        <h2>存取、內容與曝光</h2>
        <p className="note">處理成員資格、內容限制與公開目錄的准入。</p>
      </header>
      <ul className="admin-home-list">
        <li>
          <div>
            <Link to="/admin/accounts">
              <strong>帳號與點數</strong>
            </Link>
            <p className="note">用 email 找帳號，看餘額與分錄，授予點數。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/skills" search={{}}>
              <strong>小工具治理</strong>
            </Link>
            <p className="note">找任何工作區的小工具，設定受限展示、再散布判定或下架。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/rosters">
              <strong>名冊</strong>
            </Link>
            <p className="note">目前生效的 operator 名冊與封測名單，唯讀。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/exposure" search={{}}>
              <strong>曝光審核</strong>
            </Link>
            <p className="note">審核發佈物的最新 Release，決定要不要讓它出現在搜尋與目錄裡。</p>
          </div>
        </li>
      </ul>
    </section>
  );
}

function OperationsSection() {
  return (
    <section className="admin-home-section">
      <header>
        <p className="admin-home-eyebrow">Conducting · Operations</p>
        <h2>派送、稽核與成本</h2>
        <p className="note">監看平台執行狀態、操作紀錄與資源使用。</p>
      </header>
      <ul className="admin-home-list">
        <li>
          <div>
            <Link to="/admin/dispatch">
              <strong>派送煞車</strong>
            </Link>
            <p className="note">看平台有沒有在派送新的試跑紀錄，宣告或解除煞車。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/audit-log">
              <strong>動作紀錄</strong>
            </Link>
            <p className="note">人員、平台 Agent 與平台自動執行的動作，包括查詢帳號與點數。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/model-budgets">
              <strong>模型呼叫逾時</strong>
            </Link>
            <p className="note">每一種模型呼叫最多可以跑多久，上限由程式決定。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/cost-statistics">
              <strong>成本統計</strong>
            </Link>
            <p className="note">每一種模型與沙箱呼叫最新的成本分布，不含使用者維度。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/trends" search={{}}>
              <strong>趨勢</strong>
            </Link>
            <p className="note">成本、點數、試跑紀錄與 operator 動作的每日走勢，只有彙總。</p>
          </div>
        </li>
        <li>
          <div>
            <Link to="/admin/agents" search={{}}>
              <strong>平台 Agent</strong>
            </Link>
            <p className="note">
              最近的維運日報、每次執行的步驟與花費，以及每個 Agent 的啟停與全域煞車。
            </p>
          </div>
        </li>
      </ul>
    </section>
  );
}
