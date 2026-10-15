import { useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useSkillVersions } from "../../skills.service";
import { isVersionUploadResultUnknown, useSaveSkillVersion } from "../../versions.service";
import { VersionUploadFailure } from "./VersionUploadFailure";
import "./VersionUpload.css";

export function VersionUpload({
  skillId,
  versionId,
  contextConfirmed,
}: {
  skillId: string;
  versionId: string;
  contextConfirmed: boolean;
}) {
  const versions = useSkillVersions(skillId);
  const save = useSaveSkillVersion(skillId);
  const [file, setFile] = useState<File>();
  const resultLink = useRef<HTMLAnchorElement>(null);
  useEffect(() => {
    if (save.data) {
      resultLink.current?.focus();
      resultLink.current?.scrollIntoView({ block: "center" });
    }
  }, [save.data]);
  const confirmed = contextConfirmed && versions.isSuccess && versions.fetchStatus === "idle";
  const readStatusId = `version-upload-${skillId}-read-status`;

  if (
    !versions.data?.versions.length ||
    (!versions.data.versions.some((version) => version.version_id === versionId) &&
      save.data?.version_id !== versionId)
  )
    return null;

  return (
    <section>
      <h3>上傳新版本</h3>
      <p className="note" data-role="teaching">
        把你改過的套件上傳成這個小工具的新版本；舊版本原封不動留著。
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
          if (file && confirmed && !save.isPending) save.mutate(file);
        }}
      >
        <label htmlFor="skill-version-file">小工具 zip</label>
        <input
          id="skill-version-file"
          type="file"
          required
          accept=".zip,application/zip"
          onChange={(event) => setFile(event.target.files?.[0])}
        />
        <button
          type="submit"
          disabled={save.isPending || !confirmed}
          aria-describedby={!confirmed ? readStatusId : undefined}
        >
          {save.isPending
            ? "上傳中…"
            : save.isError && isVersionUploadResultUnknown(save.error)
              ? "重新送出上傳"
              : "上傳成新版本"}
        </button>
      </form>
      {!confirmed && (
        <p id={readStatusId} role="status">
          小工具與版本脈絡尚未確認；請先重新讀取，再上傳新版本。
        </p>
      )}
      {save.isError && (
        <VersionUploadFailure
          error={save.error}
          fileName={save.variables?.name}
          skillId={skillId}
          versionId={versionId}
        />
      )}
      {save.isSuccess && (
        <div role="status">
          <p className="note version-upload-result">這次上傳的檔案：{save.variables?.name}</p>
          <p>
            {save.data.duplicate
              ? `這份內容與現有的 v${save.data.version_number} 完全相同，沒有建立新版本。`
              : `已存成 v${save.data.version_number}。`}
          </p>
          <Link
            ref={resultLink}
            to="/skills/$skillId/versions/$versionId"
            params={{ skillId: save.data.skill_id, versionId: save.data.version_id }}
          >
            開啟 v{save.data.version_number} 並繼續驗證
          </Link>
        </div>
      )}
    </section>
  );
}
