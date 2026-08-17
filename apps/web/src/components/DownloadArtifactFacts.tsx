import type { DownloadArtifact } from "../api/packaging";

/**
 * What one built package is, shared by the page that just built it and the
 * download history (WS-002 第 1 條). One component so the two surfaces cannot
 * describe the same row differently.
 *
 * The rules it exists to keep:
 *
 * 1. **Expiry is an absolute date, never 「N 天後」** (PDM-006 risk table), and
 *    it is stated together with the fact that the same version can be packaged
 *    again — which is true because packaging is idempotent.
 * 2. **「已過期」 and 「沒有這一筆」 are different answers.** An expired row stays
 *    in the list saying it expired; a row that never existed is the empty state
 *    on the list page. Neither is rendered as the other.
 * 3. **`status` is three states, not a ready flag.** `quarantined` (still being
 *    checked) and `rejected` (over) are not the same thing to the person waiting
 *    for a download, so they are not folded together.
 */

export function isExpired(artifact: DownloadArtifact, now = Date.now()): boolean {
  const at = Date.parse(artifact.expires_at);
  return Number.isFinite(at) && at <= now;
}

const STATUS_LABEL: Record<DownloadArtifact["status"], string> = {
  quarantined: "檢查中（尚未可下載）",
  available: "可下載",
  rejected: "已拒絕（打包後未通過驗證）",
};

const TARGET_NOTE = "打包目標；安裝說明在套件內的 INSTALL.md。";

function bytes(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

export function DownloadArtifactFacts({ artifact }: { artifact: DownloadArtifact }) {
  const expired = isExpired(artifact);

  return (
    <>
      <p>
        <strong>{artifact.file_name}</strong>{" "}
        <span className="badge" title={TARGET_NOTE}>
          {artifact.target}
        </span>{" "}
        <span className="badge">
          {artifact.includes_test_cases ? "含 Test Case" : "不含 Test Case"}
        </span>{" "}
        {expired && <span className="badge badge-expired">已過期</span>}
      </p>
      <p className="note">
        {bytes(artifact.size_bytes)}｜狀態：{STATUS_LABEL[artifact.status]}｜建立於{" "}
        {artifact.created_at}｜已下載 {artifact.download_count} 次
      </p>
      {artifact.status_reason && <p className="note">拒絕原因：{artifact.status_reason}</p>}
      <p className="note">
        {expired ? (
          <>
            已於 {artifact.expires_at} 到期，檔案已刪除，這筆紀錄保留。
            「已過期」與「沒有這一筆」不是同一件事。要再拿到同樣的內容，回到該版本重新打包一次即可。
          </>
        ) : (
          <>到期時間：{artifact.expires_at}（到期後檔案刪除，同一版本隨時可以再打包一次）</>
        )}
      </p>
      <details>
        <summary>雜湊與打包器版本</summary>
        <ul className="note">
          <li>
            內容雜湊（這是不是同一個檔）：<code>{artifact.content_hash}</code>
          </li>
          <li>
            Manifest 雜湊（內容和上次一不一樣）：<code>{artifact.manifest_hash}</code>
          </li>
          <li>打包器版本：{artifact.packager_version ?? "未回報"}</li>
          <li>Profile 版本：{artifact.profile_version ?? "無（標準套件沒有 Profile）"}</li>
          <li>
            Skill Version ID：<code>{artifact.skill_version_id}</code>
          </li>
        </ul>
      </details>
    </>
  );
}
