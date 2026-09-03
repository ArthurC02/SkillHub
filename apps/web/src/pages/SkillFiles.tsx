import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
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
  // 設計 §4.3 的硬規則：被截斷的清單（這裡是一份被截斷的**文件**，同一條）要說出
  // 「共 N，這裡顯示 M，因為 X」。伺服器沒有回一個 `skill_md_bytes` 欄位，但它回了
  // 檔案樹，而 SKILL.md 是樹裡的一列——`fileTree` 走的是套件內每一個檔案，`Size`
  // 取自 zip entry 的未壓縮大小（discovery/detail.go）。所以總量是**伺服器送的**，
  // 不是這裡估的；讀者也看得到同一個數字長在下面的檔案樹上。
  // 找不到那一列時不編一個數字：退回原本只說理由的那句話。
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
      {/*
        0023: the API's own sentence is shown rather than a local paraphrase.
        There is one place that decides what a hold means, and a second wording
        here is a second thing to keep in step.
      */}
      {error instanceof ApiError && error.status === 403 && <p role="status">{error.message}</p>}
      {/* 410 and 403 above keep their own sentences — they are facts about this
          listing, not read failures. Everything else goes through the shared
          component so a 401 says to log in and a 500 still says what broke;
          「載入檔案失敗。」 said neither. */}
      {!(error instanceof ApiError && (error.status === 410 || error.status === 403)) && (
        <ReadFailure error={error} what="套件檔案清單" />
      )}

      {data && (
        <>
          <p className="note">
            版本 v{data.version_number}
            {data.embedded_script_note ? (
              // SKILL-003: repeated here because the file tree is exactly what
              // cannot show code living inside the document.
              <span className="badge badge-risk">{data.embedded_script_note}</span>
            ) : (
              /*
                設計 §2.1／§2.9. 這一格以前是「有才印」，而底下那句「內嵌的程式碼
                由上面的揭露負責」是**無條件**印的——沒有 finding 時，上面什麼都
                沒有，那句話指向空氣，整頁讀起來就是「這個套件不含程式碼」。
                而 SKILL-003 記著的事故正是這個形狀：5 個 seed 套件夾帶約 180 行
                Python，卻回報沒有 script。掃描一定跑過（不然這一頁讀不到檔案樹），
                所以「掃了、沒掃到」是一句有根據的肯定敘述，不是推定。
              */
              <span className="note">靜態掃描沒有在 SKILL.md 裡找到內嵌的程式碼。</span>
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
            {/* 設計 §2.13（丙-142）：後半句「版本不會被原地修改，所以同一個版本 ID
                永遠對應同一份內容」是**全站規則**（ADR-003），不是這一頁的事實，而且
                `SkillDetail` 的版本清單也講了一次同樣的規則。留下來的是這一頁自己的
                那一句：螢幕上這些位元組屬於上面那一個版本。 */}
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
            <pre className="skill-md">{data.skill_md}</pre>
          </section>

          <section>
            <h2>檔案樹</h2>
            <FileTree entries={data.tree} />
            {/* 伺服器的 `note`（「tree 為套件內檔案清單與大小；…」）在這裡只是把上面
                那個 h2 用一句話再講一次——它描述的是這一區塊**是什麼**，而標題已經
                說了，每一列自己也寫著大小。設計 §2.13（丙-142）。它的後半句（單檔內容
                讀取端點尚未實作）講的是一個這一頁沒有提供、讀者也沒有在找的控制項。 */}
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

        2026-09-03（丙-142）：句尾那半句「那由版本號旁邊的那一句負責，它兩種答案都會
        說出來」刪掉了。它是**指路**——它要讀者去看同一頁上方的另一段文字，而那段文字
        本來就無條件渲染（有 finding 印徽章，沒有就印「靜態掃描沒有在 SKILL.md 裡找到
        內嵌的程式碼」）。範圍限定那一句（「不包括 SKILL.md 內嵌的程式碼」）是 §2.11(c)
        的但書，**留著**：它說的是這一句話不涵蓋什麼。
      */}
      <p className="note">
        {scripts > 0
          ? `標成 Script 的 ${scripts} 個檔案是可執行 Script：它們會在你自己的環境裡執行。Skill Hub 的匯入與掃描階段不執行套件內的任何程式碼。`
          : "這個清單裡沒有可執行 Script 檔案。這只說明檔案樹，不包括 SKILL.md 內嵌的程式碼。"}
      </p>
    </>
  );
}
