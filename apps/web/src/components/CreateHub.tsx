import { Link } from "@tanstack/react-router";
import { GenerateSkill } from "./GenerateSkill";

/**
 * 「建立一個 Skill」 — the three ways in, in one place, at the top of
 * `/workspace/skills`.
 *
 * WHY HERE AND NOT AT `/create` OR ON THE HOME PAGE. Three separate rules land
 * on the same address:
 *
 *   - 資訊架構 §0.1 R7: `/workspace/skills` is already a nav item and already a
 *     list address in R2's sense. A new `/create` would be a verb in a list
 *     position and would cost a row in §0.2's ledger — and that ledger may only
 *     get shorter.
 *   - 02:GEN-004 / ADR-046 決策 7: the home page may not carry a generation
 *     entry of equal weight to search. Sitting here avoids that clause entirely
 *     rather than arguing with it.
 *   - 資訊架構 §2.4 does not change by one word: `generate_skill` already lists
 *     `/workspace/skills` as somewhere it appears.
 *
 * WHAT IS DELIBERATELY NOT HERE. A fourth card for 「從零寫一個」: there is no
 * blank-skill endpoint and no editor (`/skills/$id/files` is a read-only
 * `<pre>`), so the card would be a door onto nothing.
 *
 * `exposed` IS A PROP, NOT A HOOK CALL, and that is not a style choice.
 * `ia.test.ts`'s FLAG_OFF_ASSERTED roster is keyed by the file that CALLS
 * `useGenerateEntryPoint`, it is shrink-only at three entries, and
 * `pages/WorkspaceSkills.tsx` is one of them. Reading the flag here would move
 * the mount off the roster and put a fourth name on a list that cannot grow —
 * so the page keeps the read, and the boundary keeps the test that guards it.
 */
export function CreateHub({ generateExposed }: { generateExposed: boolean }) {
  return (
    /* `id="create"` is a link target, not decoration: the home page's hero
       points at `/workspace/skills#create`. */
    <section className="create-hub" id="create" aria-labelledby="create-heading">
      <h2 id="create-heading">建立一個 Skill</h2>
      {/* 設計 §2.13,D 類:這一段說「三條路的結果一樣」,而三張卡的標題已經說了三條路
          各是什麼,標題本身也已經在 h2「建立一個 Skill」底下。第二次來看的人不會因為
          這一段而改按哪一張卡,所以它整段走。 */}

      {/*
        §4.3: `.download-item` is this app's universal card family — 「修法一律是
        套用既有的卡片族，不是發明第五種樣式」 — so the hub paints no card of its
        own and the grid is the only new thing.
      */}
      <ul className="create-cards">
        <li className="download-item">
          <h3>匯入現成的套件</h3>
          {/* 「不會執行套件裡的任何 Script」 走了,不是因為它不重要,而是因為它在
              /workspace/import 上有一份逐字的同義句（「套件只會做靜態檢查；匯入期間
              不執行其中的 Script。」）,而那一頁才是撞上這件事的地方——這張卡是通往
              那一頁的門,門上與門後講同一句話,讀者只讀得到第二次（§2.13 第 2 條）。 */}
          <p className="note">貼一個 GitHub URL，或上傳一個 zip。平台會做規格驗證與靜態掃描。</p>
          {/*
            The one filled action on this page (§4.6.3 一頁至多一個). It was zero
            before, so this adds a row to that table rather than competing with
            an existing primary action.
          */}
          <p>
            <Link className="action" to="/workspace/import">
              匯入 Skill
            </Link>
          </p>
        </li>

        <li className="download-item">
          <h3>從目錄挑一個來改</h3>
          <p className="note">
            {/* §2.13,D 類:三步驟的教學縮成一句。按鈕上的字（「以這個 Skill 為起點
                建立我自己的」）在目錄那一頁就看得到,不必在門上先背一次。 */}
            從目錄複製一份到你的工作區，再上傳改過的版本。
            {/*
              §2.2 第三向: the restriction is real, it is stated before the user
              walks into it, and it names who enforces it. Fork is
              RequireInvited on the platform side (ADR-028 決策 1) — an
              uninvited user gets a 403, and finding that out by pressing the
              button is the shape 「強制但不顯示」 names as the second worst.
            */}
            Fork 需要封測邀請，這道限制由平台強制；還沒有邀請的話，那一步會被擋下來。
          </p>
          <p>
            <Link to="/" search={{}}>
              到目錄挑一個
            </Link>
          </p>
        </li>

        {/*
          ⛔ 01 §10 邊界 1 / ADR-052. Not rendered at all when the flag is off —
          not disabled, not 「即將推出」. §2.4 governs disabled controls that
          exist and says why; a beta participant who can see that this exists
          has already had the funnel's first segment changed for them, and that
          number has one chance and twelve people.

          No `<h3>` of its own: `GenerateSkill` opens with 「沒有夠接近的？讓平台
          依你的描述做一個」, and a card title above it would be §3 第 14 條 —
          the same fact, on one screen, worded twice.
        */}
        {generateExposed && (
          <li className="download-item">
            <GenerateSkill />
          </li>
        )}
      </ul>
    </section>
  );
}
