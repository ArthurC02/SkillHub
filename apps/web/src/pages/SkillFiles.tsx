import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useSkillFiles } from "../api/skills";
import type { SkillFileEntry } from "../api/types";

/**
 * DISC-007 advanced view: the full SKILL.md of the latest version plus the
 * package file tree. Reached from the detail page's 進階模式 link — the general
 * mode is that page, so this one only links back rather than carrying a toggle
 * that would just say "go there".
 */
export function SkillFiles() {
  const { skillId } = useParams({ from: "/skills/$skillId/files" });
  const { data, isLoading, error } = useSkillFiles(skillId);

  return (
    <article>
      <nav>
        <Link to="/skills/$skillId" params={{ skillId }}>
          ← 回到 Skill 詳情（一般模式）
        </Link>
      </nav>

      {isLoading && <p>載入中…</p>}
      {error instanceof ApiError && error.status === 410 && (
        <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>
      )}
      {error && !(error instanceof ApiError && error.status === 410) && (
        <p role="alert">載入檔案失敗。</p>
      )}

      {data && (
        <>
          <p className="note">
            版本 v{data.version_number}
            {data.embedded_script_note && (
              // SKILL-003: repeated here because the file tree is exactly what
              // cannot show code living inside the document.
              <span className="badge badge-risk">{data.embedded_script_note}</span>
            )}
          </p>

          <section>
            <h2>SKILL.md</h2>
            {data.skill_md_truncated && (
              <p className="notice" role="status">
                內容過長，以下只顯示前 1 MiB。
              </p>
            )}
            <pre className="skill-md">{data.skill_md}</pre>
          </section>

          <section>
            <h2>檔案樹</h2>
            <FileTree entries={data.tree} />
            <p className="note">{data.note}</p>
          </section>
        </>
      )}
    </article>
  );
}

function FileTree({ entries }: { entries: SkillFileEntry[] }) {
  if (entries.length === 0) return <p>這個版本沒有其他檔案。</p>;

  return (
    <ul className="file-tree">
      {entries.map((entry) => (
        <li key={entry.path} className={entry.is_script ? "file-script" : undefined}>
          <span className="file-path">{entry.path}</span>
          <span className="file-size">{entry.size} bytes</span>
          {entry.is_script && (
            <span className="script-tag" title="此檔案為可執行 Script">
              Script
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}
