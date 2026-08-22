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
 * 4. **The status word is derived from `status` AND expiry** (system.md §2.2,
 *    02:NFR-001). `available` alone is not servable: the platform requires
 *    `available && !purged && expires_at > now` (delivery/download.go) before it
 *    hands over bytes, so printing 可下載 beside 已過期 and 目前不提供下載 stated
 *    three things about one artifact, two of them false. Displaying a state the
 *    platform enforces against is the 顯示但不強制 shape §2.2 names as the worst
 *    of the options. Both surfaces read this one derivation, which is the point
 *    of the shared component — they must not be able to disagree.
 */

export function isExpired(artifact: DownloadArtifact, now = Date.now()): boolean {
  const at = Date.parse(artifact.expires_at);
  return Number.isFinite(at) && at <= now;
}

/**
 * The status word is the server's (04 丙-29 ⑤).
 *
 * It was derived here, from `status` and expiry, and the derivation was right as
 * far as it could see — but it could not see the purge, which is the third input
 * the platform actually checks (skill/delivery/download.go). So this file was
 * computing a *different predicate* from the one that decides whether bytes come
 * out, and printing its answer beside the server's expiry date. `serve_state`
 * and `servable` come from the one function that combines all three.
 *
 * What that rule preserved and the server now preserves: expiry outranks
 * `available` and **only that one** — `quarantined` and `rejected` already say
 * the bytes are not on offer, and overwriting them with 已過期 would lose why.
 */

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
      {/* 04 丙-42: this row used to name its version only as a uuid, folded inside
          a disclosure. 「我下載的是不是最新調整好的那一版」 is the question WS-002
          第 1 條 asks, and no arrangement of a uuid answers it. The wording and
          both numbers are the server's (設計系統 §4.4) — the client is not the
          place to decide what 「舊」 means. */}
      <p className="note">
        版本：{artifact.version_state.label}
        {artifact.version_state.note && <> — {artifact.version_state.note}</>}
      </p>
      <p className="note">
        {bytes(artifact.size_bytes)}｜狀態：{artifact.serve_state.label}｜建立於{" "}
        {artifact.created_at}｜已下載 {artifact.download_count} 次
      </p>
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
            Skill Version ID：<code>{artifact.skill_version_id}</code>（v
            {artifact.version_number}）
          </li>
        </ul>
      </details>
    </>
  );
}
