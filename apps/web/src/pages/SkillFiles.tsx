import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useSkillFiles } from "../api/skills";
import type { SkillFileEntry } from "../api/types";
import { Reveal } from "../components/Reveal";

export function SkillFiles() {
  const { skillId } = useParams({ from: "/skills/$skillId/files" });
  const { data, isLoading, error } = useSkillFiles(skillId);
  const skillMdBytes = data?.tree.find((entry) => entry.path === "SKILL.md")?.size;

  return (
    <article>
      <nav>
        <Link to="/skills/$skillId" params={{ skillId }}>
          ← 回到 Skill 詳情（一般模式）
        </Link>
      </nav>

      <h1>SKILL.md 與檔案樹</h1>

      {isLoading && <Loading what="套件檔案清單" />}
      {error instanceof ApiError && error.status === 410 && (
        <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>
      )}
      {error instanceof ApiError && error.status === 403 && <p role="status">{error.message}</p>}
      {!(error instanceof ApiError && (error.status === 410 || error.status === 403)) && (
        <ReadFailure error={error} what="套件檔案清單">
          {error instanceof ApiError && error.status === 404 ? (
            <p role="alert">找不到這個 Skill 的檔案清單，它可能還沒有保存的版本。</p>
          ) : error instanceof ApiError && error.status === 503 ? (
            <p role="alert">儲存的套件目前讀不到，稍後再試一次。</p>
          ) : undefined}
        </ReadFailure>
      )}

      {data && (
        <>
          <p className="note">
            版本 v{data.version_number}
            {data.embedded_script_note ? (
              <span className="badge badge-risk">{data.embedded_script_note}</span>
            ) : (
              <span className="note">靜態掃描沒有在 SKILL.md 裡找到內嵌的程式碼。</span>
            )}
          </p>

          <details>
            <summary>進階資訊（版本與識別碼）</summary>
            <ul className="note">
              <li>版本編號：v{data.version_number}</li>
              <li>
                版本 ID：<code>{data.version_id}</code>
              </li>
              <li>
                Skill ID：<code>{data.skill_id}</code>
              </li>
            </ul>
            <p className="note">這一頁顯示的內容屬於上面這一個不可變版本。</p>
          </details>

          <section>
            <h2>SKILL.md</h2>
            {data.skill_md_truncated && (
              <p className="notice" role="status">
                {skillMdBytes === undefined
                  ? "內容過長，以下只顯示前 1 MiB。"
                  : `共 ${skillMdBytes} bytes，這裡只顯示前 1 MiB，因為這個端點的單次上限是 1 MiB。`}
              </p>
            )}
            <pre className="skill-md">
              <Reveal text={data.skill_md} />
            </pre>
          </section>

          <section>
            <h2>檔案樹</h2>
            <FileTree entries={data.tree} />
          </section>
        </>
      )}
    </article>
  );
}

function FileTree({ entries }: { entries: SkillFileEntry[] }) {
  if (entries.length === 0) return <p>這個版本沒有其他檔案。</p>;

  const scripts = entries.filter((entry) => entry.is_script).length;

  return (
    <>
      <ul className="file-tree">
        {entries.map((entry) => (
          <li key={entry.path} className={entry.is_script ? "file-script" : undefined}>
            <span>{entry.path}</span>
            <span className="file-size">{entry.size} bytes</span>
            {entry.is_script && (
              <span className="script-tag" title="此檔案為可執行 Script">
                Script
              </span>
            )}
          </li>
        ))}
      </ul>
      <p className="note">
        {scripts > 0
          ? `標成 Script 的 ${scripts} 個檔案是可執行 Script：它們會在你自己的環境裡執行。Skill Hub 的匯入與掃描階段不執行套件內的任何程式碼。`
          : "這個清單裡沒有可執行 Script 檔案。這只說明檔案樹，不包括 SKILL.md 內嵌的程式碼。"}
      </p>
    </>
  );
}
