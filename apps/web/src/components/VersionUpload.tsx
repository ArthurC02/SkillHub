import { useState } from "react";
import { ApiError } from "../api/client";
import { isCategorizedFindings } from "../api/import";
import { Findings } from "./Findings";
import { ReadFailure } from "./LoginRequired";
import { useSkillVersions } from "../api/skills";
import { useSaveSkillVersion } from "../api/versions";

function versionUploadErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 413) return "檔案超過上限，請縮小套件再上傳。";
    if (error.status === 404) return "找不到這個 Skill，它可能已被刪除。";
  }
  return "上傳沒有成功，可以再按一次。";
}

export function VersionUpload({ skillId }: { skillId: string }) {
  const versions = useSkillVersions(skillId);
  const save = useSaveSkillVersion(skillId);
  const [file, setFile] = useState<File>();

  if ((versions.data?.versions.length ?? 0) === 0) return null;

  return (
    <section>
      <h3>上傳新版本</h3>
      <p className="note" data-role="teaching">
        把你改過的套件上傳成這個 Skill 的新版本；舊版本原封不動留著（ADR-003）。
      </p>
      <ul className="note">
        <li>
          大小上限見拒絕訊息——平台強制 zip 與解壓後的兩個上限，但這一頁還讀不到它們的值，
          所以這裡不印一個沒有來源的數字。
        </li>
        <li>
          zip 的最上層（或單一頂層資料夾）要有 <code>SKILL.md</code>，而且它的 frontmatter 要有{" "}
          <code>name</code> 與 <code>description</code>——名稱、描述與 License 都從那裡讀，
          不必在這一頁手打。
        </li>
      </ul>
      <form
        className="version-upload"
        onSubmit={(event) => {
          event.preventDefault();
          if (file) save.mutate(file);
        }}
      >
        <label htmlFor="skill-version-file">Skill zip</label>
        <input
          id="skill-version-file"
          type="file"
          required
          accept=".zip,application/zip"
          onChange={(event) => setFile(event.target.files?.[0])}
        />
        <button type="submit" disabled={save.isPending}>
          {save.isPending ? "上傳中…" : "上傳成新版本"}
        </button>
      </form>
      {save.isError &&
        (save.error instanceof ApiError && isCategorizedFindings(save.error.body) ? (
          <section role="alert">
            <p>上傳被擋下：套件沒有通過驗證。</p>
            <Findings findings={save.error.body} level={4} />
          </section>
        ) : (
          <ReadFailure error={save.error} what="上傳新版本">
            <p role="alert">{versionUploadErrorMessage(save.error)}</p>
          </ReadFailure>
        ))}
      {save.isSuccess && (
        <p role="status">
          {save.data.duplicate
            ? `這份內容與現有的 v${save.data.version_number} 完全相同，沒有建立新版本。`
            : `已存成 v${save.data.version_number}。`}
        </p>
      )}
    </section>
  );
}
