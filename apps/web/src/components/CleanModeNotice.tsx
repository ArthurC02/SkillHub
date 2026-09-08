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
 *
 * ── 2026-09-07：五句話從平鋪改成一個可點開的標籤 ────────────────────────────
 *
 * **搬動的是揭露的形狀，不是它的內容**，而形狀本身是 §0 明文允許讓步的那一半
 * （「讓步的是形式」）。促成它的是一次外部審查：非技術讀者看到頁首六行關於沙箱、
 * presigned URL 與併發語意的灰字，得到的不是知情，是「系統是不是壞了、我有沒有
 * 權限用」——**一段沒有人讀的揭露，其實現度是零**，而它同時把 checklist 第 1 條
 * 要的頭條推到第二屏（這個元件自己上一段就在講那條規則，然後違反了它）。
 *
 * 五句一個字都沒有改、一句都沒有刪，全部仍在 DOM 裡、仍由 `clean-mode.test.tsx`
 * 逐句斷言。改的是預設是否展開。
 *
 * **為什麼這不是把 §2.10「永不折疊」偷偷繞過去**：那份封閉清單的第 9 項是
 * 「平台的降級自述」，而它在文件裡的具名實例是 `IncompleteNotice`（system.md:68），
 * 從來沒有一份文件把這個橫幅掛在那一項底下。真正管這個形狀的是
 * [ADR-065](../../../docs/adr/ADR-065-hot-path-text-budget-and-the-fourth-disclosure-mechanism.md)
 * §3 規則 3，而它**逐字拿「淨測試模式：5 項在這個模式下不成立」當合格錨點的範例**
 * ——管這件事的那份決策，早就把這個橫幅想過一遍了。
 *
 * **錨點自己要成立**：`<summary>` 上的那一句話說出「有幾項」與「不成立」，所以
 * 不點開的人也知道自己錯過了什麼，而不是看到一個沒有內容的「詳情」。
 */
export function CleanModeNotice() {
  const cleanMode = useCleanMode();
  if (!cleanMode) return null;

  return (
    <details className="clean-mode-notice">
      {/* 徽章的 class 在 `<summary>` 自己身上，不在裡面的 `<span>` 上。2026-09-08
          之前它在 `<span>` 上，而 `summary::before` 的那個 `›` 是 summary 的第一個
          行內盒——**於是箭頭落在藥丸外面**，畫面上看起來像一個孤零零的大於符號旁邊
          擺著一顆徽章。第一次真的看畫面才看到（前面幾輪都在量數字）。 */}
      <summary className="badge badge-unverified">
        淨測試模式：5 項在這個模式下不成立（沒有隔離、不驗簽章、只有一條連線）
      </summary>
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
      {/*
        The fifth, added 2026-08-30 with the capability field that made runs
        possible here at all (04 丙-98). It is the one absence a user can be
        actively misled about rather than merely uninformed of: 試跑前的權限摘要
        prints a specific egress allow list and asks them to agree to it, and in
        this mode nothing holds the run to it. 設計 §2.2 ranks 「顯示但不強制」
        as the worst of the four states — so it gets a sentence naming the screen
        it contradicts, not a general note about networking.
      */}
      <p className="note">
        試跑前那份「可連往哪裡」的清單，在這個模式下不被強制——沒有任何東西擋著沙箱連別的地方。
      </p>
    </details>
  );
}
