import { useCleanMode } from "../api/me";

/**
 * PORT-003: 淨測試模式的畫面揭露.
 *
 * markup copied from GeneratedNotice.tsx on purpose — same two-part shape (a
 * `badge-unverified` headline, one `.note` per axis), because a second visual
 * language for "here is what this build is not" is how one of them starts
 * reading as the exception rather than the rule.
 *
 * No props. `useCleanMode` reads `GET /me`'s `features.clean_mode`
 * (ADR-052's flag mechanism, reused rather than a build-time constant, for the
 * same reason generate.ts gives: the flag is a deployment fact, not a page's).
 * Off, or not yet resolved, renders nothing — never a flash of the notice
 * before the flag is known.
 *
 * Mounted once, in router.tsx's RootLayout, inside `<main>` as the first
 * child: system.md §3 checklist 第 1 條 wants the headline to be the first
 * thing in the first screen, and a banner living outside `<main>` would queue
 * ahead of it on every page.
 *
 * KNOWN GAP, not something to fix here: `GET /me` requires a session, and `/`
 * and `/skills/$id` are reachable signed out. An anonymous visitor therefore
 * never sees this notice, so PORT-003's 「必須出現在使用者看得到的畫面上」 is
 * only met for signed-in users today. The real fix needs ADR-060 待決策 1 (a
 * merged entry point that serves its own build and can inject the flag for an
 * anonymous request) settled first — that binary does not exist yet, and
 * bolting an anonymous escape hatch onto `/me` or the invite gate ahead of
 * that decision is exactly the kind of local patch this repo keeps finding
 * out was the wrong place to put the knowledge.
 */
export function CleanModeNotice() {
  const cleanMode = useCleanMode();
  if (!cleanMode) return null;

  return (
    <>
      <p className="badge badge-unverified">
        淨測試模式：沒有隔離、不驗簽章、只有一條連線
      </p>
      <p className="note">
        沙箱沒有隔離——不是比較弱的隔離，是沒有邊界。這個模式只跑策展過的展示素材。
      </p>
      <p className="note">
        物件儲存不驗證 presigned URL。按下去有檔案出來，不構成任何授權上的證明。
      </p>
      <p className="note">
        資料庫只有一條連線，併發語意與生產不同。「在這個模式下沒重現」不是一個結論。
      </p>
    </>
  );
}
