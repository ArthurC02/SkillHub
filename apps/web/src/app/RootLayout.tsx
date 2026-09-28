import { useState, type FormEvent } from "react";
import { Link, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { FeedbackEntry } from "./shell/FeedbackEntry";
import { AuthControls } from "./shell/AuthControls";
import { CleanModeNotice } from "./shell/CleanModeNotice";
import { NavScrollCue } from "../shared/ui/NavScrollCue";
import { useGenerateEntryPoint } from "../features/creation";
import { useMe } from "../core/session/me.service";
import "./RootLayout.css";

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const navigate = useNavigate();
  const me = useMe();
  const generateExposed = useGenerateEntryPoint();
  const [query, setQuery] = useState("");

  const search = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const q = query.trim();
    void navigate({ to: "/", search: q ? { q } : {} });
  };

  return (
    <div
      className="app-shell platform-shell"
      data-chat={pathname === "/workspace/creations" || undefined}
    >
      <header className="app-header">
        <Link to={me.data ? "/workspace" : "/"} className="app-title">
          Skill Hub
        </Link>
        <AuthControls />
        {pathname !== "/" && (
          <form className="app-search" role="search" onSubmit={search}>
            <input
              type="search"
              aria-label="搜尋 Catalog"
              placeholder="搜尋小工具或描述任務"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
            <button type="submit">搜尋</button>
          </form>
        )}
      </header>
      <div className="app-frame">
        <aside className="app-sidebar">
          <p className="app-sidebar-label">Workspace</p>
          <nav className="app-nav" aria-label="主要導覽">
            <Link to="/workspace" activeOptions={{ exact: true }}>
              首頁
            </Link>
            <Link to="/" activeOptions={{ exact: true }}>
              Catalog
            </Link>
            <Link to="/workspace/skills">資產庫</Link>
            {generateExposed && <Link to="/workspace/creations">Studio</Link>}
            <Link to="/workspace/runs">活動</Link>
            <Link to="/workspace/downloads">發佈</Link>
            <NavScrollCue />
          </nav>
        </aside>
        <div className="app-content">
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
      </div>
    </div>
  );
}
