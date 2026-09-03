import type { DownloadArtifact } from "../api/packaging";
import { Timestamp } from "./Timestamp";

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
 * 5. **「已過期」 and 「檔案遺失」 are different answers too** (04 丙-91). Expiry is
 *    the promise being kept and the file comes back by packaging again; loss is
 *    the platform having dropped something inside the period it promised. This
 *    file used to have one branch for both, so a lost package was shown its own
 *    future expiry date — 「到期後檔案刪除」 about bytes that were already gone.
 *    The server decides which sentence (`serve_state`); this only stops the
 *    expiry copy being printed over it.
 */

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
  // The server's word, not this browser's clock. The status sentence beside it
  // was already the server's, so deriving the badge from Date.now() put two
  // clocks on one line: a viewer whose machine is set wrong, or in a timezone
  // this page never sees, got 已過期 next to 可下載. Worse, `quarantined` plus a
  // past expiry printed 已過期 over 檢查中 — breaking the priority rule stated at
  // the top of this very file (M4 audit, 2026-08-24).
  const expired = artifact.serve_state.value === "expired";
  // Not folded into `expired`: everything below that used the flag would then
  // tell a lost package the retention story, which is the whole of 04 丙-91.
  const lost = artifact.serve_state.value === "lost";

  return (
    <>
      <p>
        <strong>{artifact.file_name}</strong>{" "}
        <span className="badge" title={TARGET_NOTE}>
          {artifact.target}
        </span>{" "}
        <span className="note">{TARGET_NOTE}</span>{" "}
        <span className="badge">
          {artifact.includes_test_cases ? "含 Test Case" : "不含 Test Case"}
        </span>{" "}
        {(expired || lost) && (
          <span className="badge badge-expired">{lost ? "檔案遺失" : "已過期"}</span>
        )}
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
        <Timestamp at={artifact.created_at} />
        ｜已下載 {artifact.download_count} 次
      </p>
      <p className="note">
        {lost ? (
          // The server's own sentence, not a copy — the wording of a fault is
          // one place's job (設計系統 §4.4), and this surface has no way to know
          // it was a fault other than by being told.
          artifact.serve_state.note
        ) : expired ? (
          <>
            已於 <Timestamp at={artifact.expires_at} /> 到期，檔案已刪除，這筆紀錄保留。
            「已過期」與「沒有這一筆」不是同一件事。要再拿到同樣的內容，回到該版本重新打包一次即可。
          </>
        ) : (
          <>
            到期時間：
            <Timestamp at={artifact.expires_at} />
            （到期後檔案刪除，同一版本隨時可以再打包一次）
          </>
        )}
      </p>
      <details>
        <summary>雜湊與打包器版本</summary>
        <ul className="note">
          {/*
            ADR-027 決策 1 把 `content_hash` 定義為「給使用者自己比對用的事實」，
            並要求把「manifest_hash 不含自身」寫進 schema description，理由逐字是
            「它是**使用者側重算時**的必要前提」——整個設計預設了有人會自己重算。
            在此之前這兩列印的是兩串 64 個十六進位字元，**沒說是哪一種雜湊、對什麼
            算的**，也就是給了一個做不到的動作。同時 ADR-027 決策 3 要求下載面
            「不得暗示套件帶有平台背書或完整性保證」，而兩串看起來很像簽章的東西
            旁邊一句話都沒有，讀者會把它讀得比它主張的大（設計 §2.11(c)）。
            那句「不簽也不驗」是 components/Findings.tsx 已經寫好的同一句。
          */}
          <li>
            內容雜湊（SHA-256，對整包 zip 的位元組算的；下載回去對檔案算一次 SHA-256
            應該得到同一串）：<code>{artifact.content_hash}</code>
          </li>
          <li>
            Manifest 雜湊（SHA-256，對套件清單算的，不含 zip 本身的 metadata、也不含 manifest
            自己；用來判斷內容和上次一不一樣）：<code>{artifact.manifest_hash}</code>
          </li>
          <li>打包器版本：{artifact.packager_version ?? "未測量"}</li>
          <li>
            這兩串是雜湊，不是簽章。<strong>MVP 的套件不帶數位簽章，平台也不驗簽</strong>
            （ADR-027 決策 3 是明文的「不做」）——它們證明得了「位元組沒有被改過」，
            證明不了「這份東西是誰做的」。
          </li>
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
