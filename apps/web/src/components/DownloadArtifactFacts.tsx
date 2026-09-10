import type { DownloadArtifact } from "../api/packaging";
import { Timestamp } from "./Timestamp";

function bytes(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

export function DownloadArtifactFacts({ artifact }: { artifact: DownloadArtifact }) {
  const expired = artifact.serve_state.value === "expired";
  const lost = artifact.serve_state.value === "lost";
  const purged = artifact.serve_state.value === "purged";

  return (
    <>
      <p>
        <strong>{artifact.file_name}</strong> <span className="badge">{artifact.target}</span>{" "}
        <span className="badge">
          {artifact.includes_test_cases ? "含 Test Case" : "不含 Test Case"}
        </span>{" "}
        {(expired || lost || purged) && (
          <span className="badge badge-expired">
            {lost ? "檔案遺失" : purged ? "檔案不存在" : "已過期"}
          </span>
        )}
      </p>
      <p className="note">
        版本：{artifact.version_state.label}
        {artifact.version_state.note && <> — {artifact.version_state.note}</>}
      </p>
      <p className="note">
        {bytes(artifact.size_bytes)}｜狀態：{artifact.serve_state.label}｜建立於{" "}
        <Timestamp at={artifact.created_at} />
        ｜已下載 {artifact.download_count} 次
      </p>
      <p className="note">
        {lost || purged ? (
          artifact.serve_state.note
        ) : expired ? (
          <>
            已於 <Timestamp at={artifact.expires_at} /> 到期，檔案已刪除，這筆紀錄保留。
          </>
        ) : (
          <>
            到期時間：
            <Timestamp at={artifact.expires_at} />
          </>
        )}
      </p>
      <details>
        <summary>雜湊與打包器版本</summary>
        <ul className="note">
          <li>
            內容雜湊（SHA-256，對整包 zip 的位元組算的；下載回去對檔案算一次 SHA-256
            應該得到同一串）：<code>{artifact.content_hash}</code>
          </li>
          <li>
            Manifest 雜湊（SHA-256，對套件清單算的，不含 zip 本身的 metadata、也不含 manifest
            自己；用來判斷內容和上次一不一樣）：<code>{artifact.manifest_hash}</code>
          </li>
          <li>打包器版本：{artifact.packager_version ?? "未測量"}</li>
          <li>Profile 版本：{artifact.profile_version ?? "無（標準套件沒有 Profile）"}</li>
          <li>
            Skill Version ID：<code>{artifact.skill_version_id}</code>（v
            {artifact.version_number}）
          </li>
        </ul>
      </details>
    </>
  );
}
