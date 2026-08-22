import { Loading } from "../components/Loading";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useSkillFiles } from "../api/skills";
import type { SkillFileEntry } from "../api/types";

/**
 * DISC-007 advanced view: the full SKILL.md of the latest version plus the
 * package file tree. Reached from the detail page's 進階模式 link — the general
 * mode is that page, so this one only links back rather than carrying a toggle
 * that would just say "go there".
 *
 * Three things this page owes its reader (system.md):
 *
 * 1. **It names itself.** The page had no `h1` at all: a back link, a version
 *    note and then `h2 SKILL.md`. Nothing on screen said which page this was,
 *    and no gate saw it — axe fails a *skipped* level, never a missing top one,
 *    and page-level rules do not run against an element context (§6). The two
 *    sections below stay at `h2`: they are the direct children of this `h1`,
 *    so demoting them would be the mirror of the defect §3 item 6 names.
 * 2. **The bytes on screen are tied to the immutable version they came from**
 *    (§1.1). This is the view someone opens to check another person's work, and
 *    it was rendering neither `version_id` nor `skill_id` although the API sends
 *    both. Folded, not printed above the answer (§2.6).
 * 3. **The Script marker's meaning is visible text, not a `title`** (§3 item 4).
 *    A tooltip does not exist on a touch device, and this is the sentence that
 *    stops 「Script」 reading as a file-type label.
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

      <h1>SKILL.md 與檔案樹</h1>

      {isLoading && <Loading what="套件檔案清單" />}
      {error instanceof ApiError && error.status === 410 && (
        <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>
      )}
      {/*
        0023: the API's own sentence is shown rather than a local paraphrase.
        There is one place that decides what a hold means, and a second wording
        here is a second thing to keep in step.
      */}
      {error instanceof ApiError && error.status === 403 && <p role="status">{error.message}</p>}
      {error && !(error instanceof ApiError && (error.status === 410 || error.status === 403)) && (
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

          {/*
            §2.6: the identifiers are what makes this page checkable — without
            them the reader cannot tie what is rendered to a version anyone else
            can name — but they are not the answer, so they fold.
          */}
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
            <p className="note">
              這一頁顯示的內容屬於上面這一個不可變版本。版本不會被原地修改，所以同一個版本 ID
              永遠對應同一份內容。
            </p>
          </details>

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
      {/*
        §3 item 4: the marker's meaning was only in a `title`. Stated for both
        answers — 「沒有 Script」 is a fact about this package and printing it is
        the difference between a scanned package and an unstated one (§2.1).
      */}
      <p className="note">
        {scripts > 0
          ? `標成 Script 的 ${scripts} 個檔案是可執行 Script：它們會在你自己的環境裡執行。Skill Hub 的匯入與掃描階段不執行套件內的任何程式碼。`
          : "這個清單裡沒有可執行 Script 檔案。這只說明檔案樹，不包括 SKILL.md 內嵌的程式碼——那由上面的揭露負責。"}
      </p>
    </>
  );
}
