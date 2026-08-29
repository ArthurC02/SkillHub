import { useCleanMode } from "../api/me";

/**
 * PORT-003: 淨測試模式的畫面揭露.
 *
 * markup copied from GeneratedNotice.tsx on purpose — same two-part shape (a
 * `badge-unverified` headline, one `.note` per axis), because a second visual
 * language for "here is what this build is not" is how one of them starts
 * reading as the exception rather than the rule.
 *
 * No props. `useCleanMode` (api/me.ts) checks
 * `window.__SKILLHUB_CLEAN_MODE__` first and `GET /me`'s `features.clean_mode`
 * (ADR-052's flag mechanism) second. The first is injected by cmd/api's
 * clean-mode static handler straight into the served HTML, which is what
 * makes this reachable without a session — `GET /me` alone never was, and `/`
 * and `/skills/$id` are both reachable signed out. Off, or not yet resolved,
 * renders nothing — never a flash of the notice before either source answers.
 *
 * Mounted once, in router.tsx's RootLayout, inside `<main>` as the first
 * child: system.md §3 checklist 第 1 條 wants the headline to be the first
 * thing in the first screen, and a banner living outside `<main>` would queue
 * ahead of it on every page.
 */
export function CleanModeNotice() {
  const cleanMode = useCleanMode();
  if (!cleanMode) return null;

  return (
    <>
      <p className="badge badge-unverified">淨測試模式：沒有隔離、不驗簽章、只有一條連線</p>
      <p className="note">
        沙箱沒有隔離——不是比較弱的隔離，是沒有邊界。這個模式只跑策展過的展示素材。
      </p>
      <p className="note">
        物件儲存不驗證 presigned URL。按下去有檔案出來，不構成任何授權上的證明。
      </p>
      <p className="note">
        資料庫只有一條連線，併發語意與生產不同。「在這個模式下沒重現」不是一個結論。
      </p>
      {/*
        The fourth absence, and the one with no visible symptom at all: the
        object store is an in-process map, so a download that worked a minute
        ago works until the process exits and never afterwards. A reader who is
        not told this reads a gone artifact as a platform bug — and 設計 §2.9
        says an absence needs a word, not a blank.
      */}
      <p className="note">物件儲存只在記憶體裡，行程結束即消失。</p>
    </>
  );
}
