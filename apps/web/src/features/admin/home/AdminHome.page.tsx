import { Link } from "@tanstack/react-router";
import { AdminPage } from "../components/AdminPage";

export function AdminHome() {
  return (
    <AdminPage heading="營運後台">
      <ul className="download-list">
        <li className="download-item">
          <p>
            <Link to="/admin/accounts">
              <strong>帳號與點數</strong>
            </Link>
          </p>
          <p className="note">用 email 找帳號，看餘額與分錄，授予點數。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/skills" search={{}}>
              <strong>Skill 治理</strong>
            </Link>
          </p>
          <p className="note">找任何工作區的 Skill，設定受限展示、再散布判定或下架。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/dispatch">
              <strong>派送煞車</strong>
            </Link>
          </p>
          <p className="note">看平台有沒有在派送新的 Run，宣告或解除煞車。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/rosters">
              <strong>名冊</strong>
            </Link>
          </p>
          <p className="note">目前生效的 operator 名冊與封測名單，唯讀。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/audit-log">
              <strong>動作紀錄</strong>
            </Link>
          </p>
          <p className="note">全平台 operator 做過的事，包括每一次查帳號與查點數。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/cost-statistics">
              <strong>成本統計</strong>
            </Link>
          </p>
          <p className="note">每一種模型與沙箱呼叫最新的成本分布，不含使用者維度。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/trends" search={{}}>
              <strong>趨勢</strong>
            </Link>
          </p>
          <p className="note">成本、點數、Run 與 operator 動作的每日走勢，只有彙總。</p>
        </li>
      </ul>
    </AdminPage>
  );
}
