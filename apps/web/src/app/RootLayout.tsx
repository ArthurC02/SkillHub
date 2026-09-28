import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { FeedbackEntry } from "./shell/FeedbackEntry";
import { AuthControls } from "./shell/AuthControls";
import { CleanModeNotice } from "./shell/CleanModeNotice";

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  return (
    <div className="app-shell" data-chat={pathname === "/workspace/creations" || undefined}>
      <header className="app-header">
        <Link to="/" className="app-title">
          Skill Hub
        </Link>
        <nav className="app-nav" aria-label="主要導覽">
          <Link to="/workspace/skills">我的 Skill</Link>
          <Link to="/workspace/runs">Run 歷史</Link>
          <Link to="/workspace/import">匯入 Skill</Link>
          <Link to="/lab/test-cases">Test Case</Link>
          <Link to="/workspace/downloads">下載紀錄</Link>
        </nav>
        <AuthControls />
      </header>
      <main>
        <CleanModeNotice admin={pathname === "/admin" || pathname.startsWith("/admin/")} />
        <Outlet />
      </main>
      <footer className="app-footer">
        <FeedbackEntry pathname={pathname} />
        <p className="note">
          <Link to="/policy">資料保存政策</Link>
          {" ｜ "}
          <Link to="/workspace/account">帳號與刪除</Link>
        </p>
        <details className="note">
          <summary>Build 識別碼</summary>
          <code>{import.meta.env.VITE_BUILD_ID ?? "未由建置注入（本機開發）"}</code>
        </details>
      </footer>
    </div>
  );
}
