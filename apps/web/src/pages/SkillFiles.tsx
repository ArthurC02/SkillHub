import { Link, useParams } from "@tanstack/react-router";
import { useState } from "react";
import { useSkillFiles } from "../api/skills";
import type { SkillFileEntry } from "../api/types";

// DISC-007: SKILL.md + file tree advanced view, with a toggle back to
// general mode (DISC-006's detail page).
export function SkillFiles() {
  const { skillId } = useParams({ from: "/skills/$skillId/files" });
  const { data, isLoading, isError } = useSkillFiles(skillId);
  const [mode, setMode] = useState<"general" | "advanced">("advanced");

  return (
    <article>
      <nav>
        <Link to="/skills/$skillId" params={{ skillId }}>
          ← 回到 Skill 詳情
        </Link>
      </nav>

      <div className="mode-toggle" role="group" aria-label="檢視模式">
        <button type="button" aria-pressed={mode === "general"} onClick={() => setMode("general")}>
          一般模式
        </button>
        <button type="button" aria-pressed={mode === "advanced"} onClick={() => setMode("advanced")}>
          進階模式
        </button>
      </div>

      {isLoading && <p>載入中…</p>}
      {isError && <p role="alert">載入檔案失敗。</p>}

      {data && mode === "general" && (
        <p>
          一般模式請見{" "}
          <Link to="/skills/$skillId" params={{ skillId }}>
            Skill 詳情頁
          </Link>
          （功能、限制、輸入輸出、依賴、權限、來源與相容性摘要）。
        </p>
      )}

      {data && mode === "advanced" && (
        <>
          <section>
            <h2>SKILL.md</h2>
            <pre className="skill-md">{data.skill_md}</pre>
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
  return (
    <ul className="file-tree">
      {entries.map((entry) => (
        <li key={entry.path} className={entry.is_script ? "file-script" : undefined}>
          <span className="file-path">{entry.path}</span>
          {entry.is_script && (
            <span className="script-tag" title="此檔案包含可執行 Script">
              Script
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}
