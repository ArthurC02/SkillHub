import { Link } from "@tanstack/react-router";
import { useDispatchStatus, useExposureQueue } from "../admin.service";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { AdminPage } from "../components/AdminPage";
import "./AdminHome.page.css";

export function AdminHome() {
  return (
    <AdminPage
      heading="營運後台"
      lede="依工作目的進入平台治理與日常營運；每一區都只顯示它能採取的動作。"
    >
      <div className="admin-home-sections">
        <GovernanceSection />
        <OperationsSection />
      </div>
    </AdminPage>
  );
}

function GovernanceSection() {
  const queue = useExposureQueue();
  const pending = queue.data?.publications.length ?? 0;

  return (
    <section className="admin-home-section">
      <header>
        <p className="admin-home-eyebrow">Governing · Decisions</p>
        <h2>存取、內容與曝光</h2>
        <p className="note">處理成員資格、內容限制與公開目錄的准入。</p>
      </header>
      <ul className="admin-home-list">
        <li>
          <span className="admin-home-index" aria-hidden="true">
            01
          </span>
          <div>
            <Link to="/admin/accounts">
              <strong>帳號與點數</strong>
            </Link>
            <p className="note">用 email 找帳號，看餘額與分錄，授予點數。</p>
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            02
          </span>
          <div>
            <Link to="/admin/skills" search={{}}>
              <strong>小工具治理</strong>
            </Link>
            <p className="note">找任何工作區的小工具，設定受限展示、再散布判定或下架。</p>
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            03
          </span>
          <div>
            <Link to="/admin/rosters">
              <strong>名冊</strong>
            </Link>
            <p className="note">目前生效的 operator 名冊與封測名單，唯讀。</p>
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            04
          </span>
          <div>
            <Link to="/admin/exposure" search={{}}>
              <strong>曝光審核</strong>
            </Link>
            <p className="note">審核發佈物的最新 Release，決定要不要讓它出現在搜尋與目錄裡。</p>
            {queue.isFetching && <p role="status">正在讀取待審數…</p>}
            {!queue.isFetching && (
              <ReadFailure
                error={queue.error}
                what="曝光待審數"
                onRetry={() => void queue.refetch()}
                retrying={queue.isFetching}
              />
            )}
            {queue.data && !queue.error && !queue.isFetching && (
              <p className="badge-row">
                <span className={pending > 0 ? "badge badge-warning" : "badge"}>
                  待審 {pending} 筆
                </span>
              </p>
            )}
          </div>
        </li>
      </ul>
    </section>
  );
}

function OperationsSection() {
  const dispatch = useDispatchStatus();
  const status = dispatch.data;
  const haltCount = status?.halts.length ?? 0;

  return (
    <section className="admin-home-section">
      <header>
        <p className="admin-home-eyebrow">Governing · Operations</p>
        <h2>派送、稽核與成本</h2>
        <p className="note">監看平台執行狀態、操作紀錄與資源使用。</p>
      </header>
      <ul className="admin-home-list">
        <li>
          <span className="admin-home-index" aria-hidden="true">
            05
          </span>
          <div>
            <Link to="/admin/dispatch">
              <strong>派送煞車</strong>
            </Link>
            <p className="note">看平台有沒有在派送新的試跑紀錄，宣告或解除煞車。</p>
            {dispatch.isPending && dispatch.isFetching && <p role="status">正在讀取派送狀態…</p>}
            {status && dispatch.isFetching && <p className="note">正在確認最新派送狀態…</p>}
            {!dispatch.isFetching && (
              <ReadFailure
                error={dispatch.error}
                what="派送狀態"
                onRetry={() => void dispatch.refetch()}
                retrying={dispatch.isFetching}
              />
            )}
            {status && !dispatch.error && !dispatch.isFetching && (
              <p className="badge-row">
                <span
                  className={
                    !status.dispatching
                      ? "badge badge-danger"
                      : haltCount > 0
                        ? "badge badge-warning"
                        : "badge"
                  }
                >
                  {status.dispatching ? "派送中" : "已停止派送"} · {haltCount}{" "}
                  {status.dispatching && haltCount > 0 ? "個節點煞車" : "個煞車"}
                </span>
              </p>
            )}
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            06
          </span>
          <div>
            <Link to="/admin/audit-log">
              <strong>動作紀錄</strong>
            </Link>
            <p className="note">全平台 operator 做過的事，包括每一次查帳號與查點數。</p>
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            07
          </span>
          <div>
            <Link to="/admin/model-budgets">
              <strong>模型呼叫逾時</strong>
            </Link>
            <p className="note">每一種模型呼叫最多可以跑多久，上限由程式決定。</p>
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            08
          </span>
          <div>
            <Link to="/admin/cost-statistics">
              <strong>成本統計</strong>
            </Link>
            <p className="note">每一種模型與沙箱呼叫最新的成本分布，不含使用者維度。</p>
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            09
          </span>
          <div>
            <Link to="/admin/trends" search={{}}>
              <strong>趨勢</strong>
            </Link>
            <p className="note">成本、點數、試跑紀錄與 operator 動作的每日走勢，只有彙總。</p>
          </div>
        </li>
        <li>
          <span className="admin-home-index" aria-hidden="true">
            10
          </span>
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
