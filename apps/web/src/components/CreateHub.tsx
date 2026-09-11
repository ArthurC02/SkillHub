import { Link } from "@tanstack/react-router";

export function CreateHub({
  generateExposed,
  creationExposed = false,
  explain = true,
}: {
  generateExposed: boolean;
  creationExposed?: boolean;
  explain?: boolean;
}) {
  const doorway = creationExposed ? "和 Agent 一起創作 Skill" : "讓平台依你的描述做一個";

  return (
    <section className="create-hub" id="create" aria-labelledby="create-heading">
      <h2 id="create-heading">建立一個 Skill</h2>

      <ul className="create-cards">
        <li className="download-item">
          <h3>匯入現成的套件</h3>
          {explain && (
            <p className="note" data-role="teaching">
              貼一個 GitHub URL，或上傳一個 zip。平台會做規格驗證與靜態掃描。
            </p>
          )}
          <p>
            <Link className="action-secondary" to="/workspace/import">
              匯入 Skill
            </Link>
          </p>
        </li>

        <li className="download-item">
          <h3>從目錄挑一個來改</h3>
          <p className="note">
            {explain && (
              <span data-role="teaching">從目錄複製一份到你的工作區，再上傳改過的版本。</span>
            )}
            平台目前只讓有封測邀請的帳號 Fork。
          </p>
          <p>
            <Link className="action-secondary" to="/" search={{}}>
              到目錄挑一個
            </Link>
          </p>
        </li>

        {generateExposed && (
          <li className="download-item">
            <h3>{doorway}</h3>
            {explain && (
              <p className="note" data-role="teaching">
                描述你要完成的事，平台產生一個只屬於你的工作區的 Skill。
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
