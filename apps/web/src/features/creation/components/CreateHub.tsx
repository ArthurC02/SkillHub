import { Link } from "@tanstack/react-router";
import "./CreateHub.css";

export function CreateHub({
  generateExposed,
  creationExposed = false,
  explain = true,
  compact = false,
}: {
  generateExposed: boolean;
  creationExposed?: boolean;
  explain?: boolean;
  compact?: boolean;
}) {
  const doorway = creationExposed ? "和 Agent 一起創作小工具" : "讓平台依你的描述做一個";

  if (compact) {
    return (
      <section
        className="create-hub create-hub-compact"
        id="create"
        aria-labelledby="create-heading"
      >
        <h2 id="create-heading">新增到資產庫</h2>
        <ul className="create-links">
          <li>
            <Link to="/workspace/import">匯入套件</Link>
          </li>
          <li>
            <Link to="/" search={{}}>
              從 Catalog 複製一份
            </Link>
            <span className="note">平台目前只讓有封測邀請的帳號複製一份。</span>
          </li>
          {generateExposed && (
            <li>
              <Link to="/workspace/creations">開始描述</Link>
            </li>
          )}
        </ul>
      </section>
    );
  }

  return (
    <section className="create-hub" id="create" aria-labelledby="create-heading">
      <h2 id="create-heading">新增到資產庫</h2>

      <ul className="create-cards">
        <li className="surface-card" data-tone="0">
          <span className="door-mono" aria-hidden="true">
            ↑
          </span>
          <h3>匯入現成的套件</h3>
          {explain && (
            <p className="note" data-role="teaching">
              貼一個 GitHub URL，或上傳一個 zip。平台會做規格驗證與靜態掃描。
            </p>
          )}
          <p>
            <Link className="action-secondary" to="/workspace/import">
              匯入小工具
            </Link>
          </p>
        </li>

        <li className="surface-card" data-tone="1">
          <span className="door-mono" aria-hidden="true">
            ✎
          </span>
          <h3>從目錄挑一個來改</h3>
          <p className="note">
            {explain && (
              <span data-role="teaching">從目錄複製一份到你的工作區，再上傳改過的版本。</span>
            )}
            平台目前只讓有封測邀請的帳號複製一份。
          </p>
          <p>
            <Link className="action-secondary" to="/" search={{}}>
              到目錄挑一個
            </Link>
          </p>
        </li>

        {generateExposed && (
          <li className="surface-card" data-tone="2">
            <span className="door-mono" aria-hidden="true">
              ✦
            </span>
            <h3>{doorway}</h3>
            {explain && (
              <p className="note" data-role="teaching">
                描述你要完成的事，平台產生一個只屬於你的工作區的小工具。
              </p>
            )}
            <p>
              <Link className="action-secondary" to="/workspace/creations">
                開始描述
              </Link>
            </p>
          </li>
        )}
      </ul>
    </section>
  );
}
