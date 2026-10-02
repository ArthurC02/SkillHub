import { useEffect, useState, type FormEvent } from "react";
import { Link, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { FeedbackLauncher } from "./shell/FeedbackLauncher";
import { AuthControls } from "./shell/AuthControls";
import { CleanModeNotice } from "./shell/CleanModeNotice";
import { NavScrollCue } from "../shared/ui/NavScrollCue";
import { useCreationEntryPoint, useGenerateEntryPoint } from "../features/creation";
import { useMe } from "../core/session/me.service";
import { FeatureAvailabilityProvider } from "../shared/FeatureAvailabilityProvider";
import "./RootLayout.css";

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const navigate = useNavigate();
  const me = useMe();
  const generateExposed = useGenerateEntryPoint();
  const creationExposed = useCreationEntryPoint();
  const [query, setQuery] = useState("");

  useEffect(() => {
    const main = document.querySelector("#main-content");
    if (!main) return;
    const updateTitle = () => {
      const heading = main.querySelector("h1")?.textContent?.trim();
      document.title = heading ? `${heading} · Skill Hub` : "Skill Hub";
    };
    updateTitle();
    const observer = new MutationObserver(updateTitle);
    observer.observe(main, { childList: true, subtree: true, characterData: true });
    return () => observer.disconnect();
  }, [pathname]);

  const search = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const q = query.trim();
    void navigate({ to: "/", search: q ? { q } : {} });
  };

  return (
    <FeatureAvailabilityProvider creation={generateExposed && creationExposed}>
      <div
        className="app-shell platform-shell"
        data-chat={
          (pathname === "/workspace/creations" && generateExposed && creationExposed) || undefined
        }
      >
        <a className="skip-link" href="#main-content">
          跳到主要內容
        </a>
        <header className="app-header">
          <Link to="/" className="app-title">
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
            <div className="app-sidebar-context app-sidebar-label" aria-label="目前 Workspace">
              <span>Workspace</span>
              <span className="app-sidebar-context-name">
                {me.data?.display_name ?? "Skill Hub"}
              </span>
            </div>
            <nav className="app-nav" aria-label="主要導覽">
              <Link to="/workspace" activeOptions={{ exact: true }}>
                工作台
              </Link>
              <Link to="/" activeOptions={{ exact: true }}>
                探索
              </Link>
              <Link to="/library">資產庫</Link>
              {generateExposed && <Link to="/workspace/creations">Studio</Link>}
              <Link to="/activity">活動</Link>
              <Link to="/workspace/downloads">發佈</Link>
              <NavScrollCue />
            </nav>
          </aside>
          <div className="app-content">
            <main id="main-content" tabIndex={-1}>
              <CleanModeNotice admin={pathname === "/admin" || pathname.startsWith("/admin/")} />
              <Outlet />
            </main>
            <footer className="app-footer">
              <FeedbackLauncher pathname={pathname} />
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
    </FeatureAvailabilityProvider>
  );
}
