import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { GenerateSkill } from "./GenerateSkill";
import { CreationSession } from "./CreationSession";

/**
 * 「建立一個 Skill」 — the three ways in, in one place, on `/workspace/skills`.
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
 *
 * ── 2026-09-07：三張卡不是三個同重量的東西，而版面一直假裝它們是 ──────────────
 *
 * 前兩張是**門**：一個標題、一句話、一個連結，按下去換頁。第三張是**工作台**——
 * `GenerateSkill` 592 行、`CreationSession` 945 行，各自帶著多輪狀態、費用、預算、
 * 素材與時間線，而 2026-09-05 的 ADR-066 又給它加了流程圖與參考 Skill 兩種輸入。
 *
 * 把工作台放進門的網格，量出來的結果是：`.create-cards` 的格軌等高，第三格撐到約
 * 1000px，於是前兩張卡各自帶著一個 **880px 的空白描邊框**；工作台自己被壓進約
 * 320px 的欄寬，`<dl>` 的標籤跑到值的左右兩側，中文一行只剩約 14 個字（設計 §4.5
 * 的 `40em` 量的是「一行 40 個中文字」，在 grid track 裡形同不存在）。
 *
 * 所以第三張卡收成一扇門，按下去**在原地展開成整列寬**（`grid-column: 1 / -1`）。
 * 這不是把它藏起來：**它變得更不顯眼，而那正是 `01` §10 邊界 1 要的方向**——旗標
 * 沒開時它整張不存在（不是 disabled、不是「即將推出」），旗標開了也只是一句話加一
 * 顆不填色的按鈕，而不是一整面表單。
 *
 * **展開之後不收回，這是刻意的。** `GenerateSkill` 的成功／失敗回饋住在一個
 * component-local 的 `useMutation` 裡（`api/generate.ts`），卸載就沒了——連同那個
 * 帶著 `skill_id` 的成功通知與逐條的驗證失敗。一顆會把使用者剛剛拿到的結果吃掉的
 * 收合鍵，不值得它省下的那幾百像素。
 */
export function CreateHub({
  generateExposed,
  creationExposed = false,
}: {
  generateExposed: boolean;
  creationExposed?: boolean;
}) {
  // 一路只從 false 走到 true，理由見檔頭最後一段。
  const [describing, setDescribing] = useState(false);

  // 門上與門後同一個名字（§3 第 14 條：同一件事一頁只有一個名字）。旗標決定門後
  // 是哪一個工作台，所以門上的字也跟著它，而不是寫死成其中一個。
  const doorway = creationExposed
    ? "和 Agent 一起創作 Skill"
    : "沒有夠接近的？讓平台依你的描述做一個";

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
          <p className="note" data-role="teaching">
            貼一個 GitHub URL，或上傳一個 zip。平台會做規格驗證與靜態掃描。
          </p>
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
            <span data-role="teaching">從目錄複製一份到你的工作區，再上傳改過的版本。</span>
            {/*
              §2.2 第三向: the restriction is real, it is stated before the user
              walks into it, and it names who enforces it. Fork is
              RequireInvited on the platform side (ADR-028 決策 1) — an
              uninvited user gets a 403, and finding that out by pressing the
              button is the shape 「強制但不顯示」 names as the second worst.

              **它不會因為卡片變小而縮短。** 這一句是 §2.2 的揭露義務，不是卡片的
              裝飾；`workspace.test.tsx` 逐字守著「Fork 需要封測邀請」與「由平台強制」
              兩段出現在這張卡上。
            */}
            Fork 需要封測邀請，這道限制由平台強制；還沒有邀請的話，那一步會被擋下來。
          </p>
          <p>
            {/*
              §4.6.3 的次要按鈕配方，這次連結也拿得到（`index.css` 的
              `.action-secondary`）。這三張卡本來是三種外觀——填色的 `.action`、一條
              純文字底線連結、一顆原生按鈕——而外部審查連續四輪把那個節奏讀成「瀏覽器
              預設樣式」。**填色仍然只有一個**（匯入，§4.6.3 一頁至多一個），變的是另外
              兩個入口從此同框。
            */}
            <Link className="action-secondary" to="/" search={{}}>
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
        */}
        {generateExposed && (
          <li className={describing ? "download-item create-workspace" : "download-item"}>
            {describing ? (
              /* 展開之後標題由工作台自己出（`GenerateSkill` 的 h2、`CreationSession`
                 的 h3），所以門上的 h3 不再存在——同一個名字在同一個時刻只出現一次。 */
              creationExposed ? (
                <CreationSession />
              ) : (
                <GenerateSkill />
              )
            ) : (
              <>
                <h3>{doorway}</h3>
                <p className="note" data-role="teaching">
                  描述你要完成的事，平台產生一個只屬於你的工作區的 Skill。
                </p>
                <p>
                  {/*
                    不是 `.action`：這一頁的填色主要動作只有一個，是「匯入 Skill」
                    （§4.6.3，`rendered.spec.ts` 全路由守著至多一個）。
                  */}
                  <button type="button" onClick={() => setDescribing(true)}>
                    開始描述
                  </button>
                </p>
              </>
            )}
          </li>
        )}
      </ul>
    </section>
  );
}
