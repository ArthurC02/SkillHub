import { Link } from "@tanstack/react-router";

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
 * 320px 的欄寬，`<dl>` 的標籤跑到值的左右兩側，中文一行只剩約 14 個字。
 *
 * 當時的處置是：第三張卡收成一扇門，按下去**在原地展開成整列寬**。
 *
 * ── 2026-09-09：那扇門真的換頁了（負責人指示）───────────────────────────────
 *
 * 「最右邊的卡片應該要像另外兩張一樣，有著獨立的頁面，直接呈現 Chat UI」。工作台
 * 搬到 `/workspace/creations`（`pages/CreateSkill.tsx`，位址為什麼是這一個寫在那裡），
 * 這一張卡自此與左邊兩張**逐位元組同一種外觀**：一個標題、一句話、一個 `.action-secondary`
 * 連結。`describing` 這個 state 沒有了，就地展開的那一段規則也沒有了。
 *
 * **上一段那兩個理由沒有被推翻，是被換頁一起解決了**：欄寬的問題消失（那一頁有整個
 * `main` 的寬度），而「展開後不收回，否則會吃掉剛拿到的結果」也不再需要——離開再回來
 * 時，會話是由 `/creation-sessions` 重新讀出來的，不是由某個元件的 `useMutation` 記著。
 *
 * ⛔ `01` §10 邊界 1 不變：旗標關著時這張卡整張不存在（不是 disabled、不是「即將
 * 推出」），而三扇門同一種外觀之後，生成入口**沒有變得更顯眼**——變的是它通往哪裡。
 */
export function CreateHub({
  generateExposed,
  creationExposed = false,
}: {
  generateExposed: boolean;
  creationExposed?: boolean;
}) {
  // 門上與門後同一個名字（§3 第 14 條：同一件事一頁只有一個名字）。旗標決定門後
  // 是哪一個工作台，所以門上的字也跟著它，而不是寫死成其中一個。
  const doorway = creationExposed ? "和 Agent 一起創作 Skill" : "讓平台依你的描述做一個";

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
            ── 2026-09-08：這一顆從填色降成描邊，而 §4.6.3 的表跟著改 ────────────────
            它 2026-09-03 落地時是這一頁唯一的填色動作，當時的理由是「這一頁本來零個」。
            §4.6.3 的判準不是「有沒有空位」，是**「完成這一頁的工作的那一個」**——而這
            一頁的工作是看自己的清單，不是匯入。這三張卡是三扇並列的門，三個同重量的
            導流，其中一個填色只是說「平台希望你走這扇」，那不是這條規則要表達的事。

            §4.6.3 的表本來就有「零個」那一列，逐字寫著「沒有『完成這一頁的工作』的
            動作時，零個是合法的」（`/compare`、`/runs/$id`、`/policy` 三頁在那一列）。
            `/workspace/skills` 同批移過去。`rendered.spec.ts` 守的是「至多一個」與
            「全站至少一個」，兩者都還成立。
          */}
          <p>
            <Link className="action-secondary" to="/workspace/import">
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
              裝飾；`workspace.test.tsx` 逐字守著限制與強制者兩段出現在這張卡上。

              ── 2026-09-08：語氣換了，兩件該說的事一件都沒少 ──────────────────
              原句尾巴是「還沒有邀請的話，那一步會被擋下來」。§2.2 第三向要的是
              **限制是真的、撞上之前先說、說出誰強制**——那一句只是把後果再講一次，
              而外部審查讀成「工程師在嗆使用者」。全 app 四處同型句同批改（這裡、
              `SkillDetail` 的 Fork、`RunPreflight` 的開始 Run、`Packaging` 的建立
              下載套件），因為同一個揭露有兩種語氣比語氣本身更糟。
            */}
            平台目前只讓有封測邀請的帳號 Fork。
          </p>
          <p>
            {/*
              §4.6.3 的次要按鈕配方，這次連結也拿得到（`index.css` 的
              `.action-secondary`）。這三張卡本來是三種外觀——填色的 `.action`、一條
              純文字底線連結、一顆原生按鈕——而外部審查連續四輪把那個節奏讀成「瀏覽器
              預設樣式」。三個現在同框，而且**一個填色都沒有**：理由在上一張卡的註解裡
              （§4.6.3 的判準是「完成這一頁的工作的那一個」，這一頁的工作是看清單）。
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
          <li className="download-item">
            <h3>{doorway}</h3>
            <p className="note" data-role="teaching">
              描述你要完成的事，平台產生一個只屬於你的工作區的 Skill。
            </p>
            <p>
              {/*
                ── 2026-09-09：它終於是一扇門，和左邊兩張一樣 ────────────────────
                在這之前這一顆按鈕把工作台**就地展開**在卡片裡（`grid-column: 1 / -1`，
                展開後不收回，因為收回會吃掉剛拿到的結果）。負責人的話：「最右邊的卡片
                應該要像另外兩張一樣，有著獨立的頁面，直接呈現 Chat UI」。

                那一整套理由跟著搬走了，不是被推翻：工作台在 320px 的欄寬裡站不住是
                真的，收回會吃掉結果也是真的——**換頁把兩件事一起解決**，因為那一頁
                本來就有整個 `main` 的寬度，而且離開再回來時會話由 `/creation-sessions`
                重新讀出來，不是由某個元件的 `useMutation` 記著。

                ⛔ `01` §10 邊界 1 不變，而且方向是對的：這扇門與另外兩扇**逐位元組同
                一種外觀**，旗標關著時整張卡仍然不存在。生成入口沒有變得更顯眼，變的
                是它按下去之後去哪裡。
              */}
              <Link className="action-secondary" to="/workspace/creations">
                開始描述
              </Link>
            </p>
          </li>
        )}
      </ul>
    </section>
  );
}
