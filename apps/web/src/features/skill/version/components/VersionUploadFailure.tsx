import { useEffect, useRef } from "react";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../../../../core/api/client";
import { Findings } from "../../../../shared/ui/Findings";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { isVersionUploadResultUnknown, readVersionUploadFindings } from "../../versions.service";

function uploadRefusalMessage(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 413) return "檔案超過上限，請縮小套件再上傳。";
    if (error.status === 404) return "找不到這個小工具，它可能已被刪除。";
    if (error.status === 422) return "套件未通過驗證，但無法讀取檢查細節。請檢查套件後再上傳。";
  }
  return "這次上傳未被接受。請確認檔案與目前權限後再上傳。";
}

export function VersionUploadFailure({
  error,
  fileName,
  skillId,
  versionId,
}: {
  error: unknown;
  fileName?: string;
  skillId: string;
  versionId: string;
}) {
  const failure = useRef<HTMLDivElement>(null);
  const unknown = isVersionUploadResultUnknown(error);
  const findings = readVersionUploadFindings(error);
  useEffect(() => {
    failure.current?.focus();
    failure.current?.scrollIntoView({ block: "center" });
  }, [error]);
  return (
    <ReadFailure error={error} what="上傳新版本">
      <div ref={failure} role="alert" tabIndex={-1} aria-label="上傳新版本結果">
        <p className="note version-upload-result">這次送出的檔案：{fileName}</p>
        {unknown ? (
          <>
            <p>無法確認這份檔案是否已存成新版本。請先核對版本與發佈，再決定是否重新送出。</p>
            <p>
              <Link
                to="/skills/$skillId/versions/$versionId"
                params={{ skillId, versionId }}
                target="_blank"
                rel="noopener noreferrer"
              >
                另開版本與發佈
              </Link>
            </p>
            <p className="note">重新送出會再次嘗試上傳目前選取的檔案。</p>
          </>
        ) : findings ? (
          <>
            <p>上傳被擋下：套件沒有通過驗證。</p>
            <Findings findings={findings} level={4} />
          </>
        ) : (
          <p>{uploadRefusalMessage(error)}</p>
        )}
      </div>
    </ReadFailure>
  );
}
