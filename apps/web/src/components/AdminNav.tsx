import { Link } from "@tanstack/react-router";

export function AdminNav() {
  return (
    <nav aria-label="後台" className="category-nav">
      <Link to="/admin" className="chip" activeOptions={{ exact: true }}>
        後台首頁
      </Link>
      <Link to="/admin/accounts" className="chip">
        帳號與點數
      </Link>
      <Link to="/admin/skills" search={{}} className="chip">
        Skill 治理
      </Link>
      <Link to="/admin/dispatch" className="chip">
        派送煞車
      </Link>
      <Link to="/admin/rosters" className="chip">
        名冊
      </Link>
      <Link to="/admin/audit-log" className="chip">
        動作紀錄
      </Link>
      <Link to="/admin/cost-statistics" className="chip">
        成本統計
      </Link>
      <Link to="/admin/trends" search={{}} className="chip">
        趨勢
      </Link>
    </nav>
  );
}
