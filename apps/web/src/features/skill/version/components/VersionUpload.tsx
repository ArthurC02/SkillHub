import { useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMe } from "../../../../core/session/me.service";
import { SessionNotReadyError } from "../../../../core/api/client";
import type { UploadResult } from "../../../../core/api/types";
import { bytes } from "../../../../shared/format";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { importRefusal, useSkillImportLimits } from "../../../creation";
import { useSkillVersions } from "../../skills.service";
import { isVersionUploadResultUnknown, useSaveSkillVersion } from "../../versions.service";
import { VersionUploadFailure } from "./VersionUploadFailure";
import "./VersionUpload.css";

type UploadFailure = { error: unknown; fileName?: string; blockedError?: SessionNotReadyError };

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
  const me = useMe();
  const save = useSaveSkillVersion(skillId);
  const limits = useSkillImportLimits();
  const [file, setFile] = useState<File>();
  const [unknownUpload, setUnknownUpload] = useState<UploadFailure>();
  const identityConfirmed = me.isSuccess && me.fetchStatus === "idle";
  const confirmed =
    contextConfirmed &&
    [me, versions, limits].every((query) => query.isSuccess && query.fetchStatus === "idle");
  const failedUpload = readUploadFailure(save, unknownUpload);
  const readStatusId = `version-upload-${skillId}-read-status`;
  const titleId = `version-upload-${skillId}-title`;
  const oversizeId = `version-upload-${skillId}-oversize`;
  const oversize = file && limits.data ? importRefusal("upload", "", file, limits.data) : null;

  if (
    !versions.data?.versions.length ||
    (!versions.data.versions.some((version) => version.version_id === versionId) &&
      save.data?.version_id !== versionId)
  )
    return null;

  return (
    <section>
      <h3 id={titleId} tabIndex={-1}>
        上傳新版本
      </h3>
      <p className="note" data-role="teaching">
        把你改過的套件上傳成這個小工具的新版本；舊版本原封不動留著。
      </p>
      <VersionUploadRules limits={limits} titleId={titleId} />
      <form
        className="version-upload"
        onSubmit={(event) => {
          event.preventDefault();
          if (!file || !confirmed || oversize || save.isPending) return;
          save.mutate(file, {
            onError: (error) => {
              if (error instanceof SessionNotReadyError) return;
              setUnknownUpload(
                isVersionUploadResultUnknown(error) ? { error, fileName: file.name } : undefined,
              );
            },
            onSuccess: () => setUnknownUpload(undefined),
          });
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
          disabled={save.isPending || !confirmed || Boolean(oversize)}
          aria-describedby={!confirmed ? readStatusId : oversize ? oversizeId : undefined}
        >
          {uploadButtonLabel(save.isPending, save.isError, failedUpload.error)}
        </button>
      </form>
      {oversize && (
        <p id={oversizeId} role="alert">
          {oversize}
        </p>
      )}
      {!confirmed && (
        <p id={readStatusId} role="status">
          {whyUploadWaits(identityConfirmed, limits.isSuccess && limits.fetchStatus === "idle")}
        </p>
      )}
      {save.isError && (
        <VersionUploadFailure
          error={failedUpload.error}
          fileName={failedUpload.fileName}
          skillId={skillId}
          versionId={versionId}
          blockedError={failedUpload.blockedError}
        />
      )}
      {save.isSuccess && (
        <VersionUploadSuccess result={save.data} fileName={save.variables?.name} />
      )}
    </section>
  );
}

function uploadButtonLabel(pending: boolean, failed: boolean, error: unknown): string {
  if (pending) return "上傳中…";
  return failed && isVersionUploadResultUnknown(error) ? "重新送出上傳" : "上傳成新版本";
}

function whyUploadWaits(identityConfirmed: boolean, limitsConfirmed: boolean): string {
  if (!identityConfirmed) return "登入狀態尚未確認；請先在頁首確認，再上傳新版本。";
  if (!limitsConfirmed) return "上傳限制尚未確認；請先重新讀取，再上傳新版本。";
  return "小工具與版本脈絡尚未確認；請先重新讀取，再上傳新版本。";
}

function VersionUploadRules({
  limits,
  titleId,
}: {
  limits: ReturnType<typeof useSkillImportLimits>;
  titleId: string;
}) {
  const rules = limits.isSuccess && limits.fetchStatus === "idle" ? limits.data : undefined;
  return (
    <>
      {limits.fetchStatus !== "idle" && (
        <p role="status">
          {limits.fetchStatus === "paused" ? "等待連線以讀取上傳限制…" : "正在讀取上傳限制…"}
        </p>
      )}
      <ReadFailure
        error={limits.error}
        what="上傳限制"
        retry={() => void limits.refetch()}
        retrying={limits.fetchStatus !== "idle"}
        focusTarget={titleId}
      />
      <ul className="note">
        {rules && (
          <li>
            平台限制：zip 最大 {bytes(rules.max_zip_bytes)}，解壓後總量最大{" "}
            {bytes(rules.max_unpacked_bytes)}；最多 {rules.max_files} 個檔案、單一檔案最大{" "}
            {bytes(rules.max_file_bytes)}、路徑最深 {rules.max_path_depth} 層。
          </li>
        )}
        <li>
          zip 的最上層（或單一頂層資料夾）要有 <code>SKILL.md</code>，而且它的 frontmatter 要有{" "}
          <code>name</code> 與 <code>description</code>——名稱、描述與 License 都從那裡讀，
          不必在這一頁手打。
        </li>
      </ul>
    </>
  );
}

function readUploadFailure(
  save: ReturnType<typeof useSaveSkillVersion>,
  previous?: UploadFailure,
): UploadFailure {
  return save.error instanceof SessionNotReadyError && previous
    ? { ...previous, blockedError: save.error }
    : { error: save.error, fileName: save.variables?.name };
}

function VersionUploadSuccess({ result, fileName }: { result: UploadResult; fileName?: string }) {
  const link = useRef<HTMLAnchorElement>(null);
  useEffect(() => {
    link.current?.focus();
    link.current?.scrollIntoView({ block: "center" });
  }, [result]);
  return (
    <div role="status">
      <p className="note version-upload-result">這次上傳的檔案：{fileName}</p>
      <p>
        {result.duplicate
          ? `這份內容與現有的 v${result.version_number} 完全相同，沒有建立新版本。`
          : `已存成 v${result.version_number}。`}
      </p>
      <Link
        ref={link}
        to="/skills/$skillId/versions/$versionId"
        params={{ skillId: result.skill_id, versionId: result.version_id }}
      >
        開啟 v{result.version_number} 並繼續驗證
      </Link>
    </div>
  );
}
