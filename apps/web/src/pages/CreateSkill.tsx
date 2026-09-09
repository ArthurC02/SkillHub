import { Link } from "@tanstack/react-router";
import { useGenerateEntryPoint } from "../api/generate";
import { useCreationEntryPoint } from "../api/creation";
import { GenerateSkill } from "../components/GenerateSkill";
import { CreationSession } from "../components/CreationSession";

/**
 * `/workspace/creations` — 和 Agent 一起創作 Skill，一整頁。
 *
 * ── 為什麼它終於是一個位址（2026-09-09，負責人指示）────────────────────────────
 *
 * 在這之前，「建立一個 Skill」第三張卡按下去是**就地展開**：工作台被塞進一張卡裡，
 * 佔滿整列（`grid-column: 1 / -1`），旁邊兩張門卡縮在左邊。負責人的話是「最右邊的
 * 卡片應該要像另外兩張一樣，有著獨立的頁面，直接呈現 Chat UI」——而另外兩張確實
 * 是門：`/workspace/import` 與 `/`。三扇並列的門，其中兩扇換頁、一扇原地長出一間
 * 房間，那個不一致就是外部審查一路讀到的「拼裝感」。
 *
 * ── 這個位址為什麼是這一個 ────────────────────────────────────────────────────
 *
 * 資訊架構 §0.1 R2：清單位址掛在提問者的位置，`/workspace/*` 是「我的東西」，而且
 * 要是**名詞的複數**。`/workspace/creations` 兩條都合——它就是「我的創作會話」那份
 * 清單（元件自己的「恢復創作」下拉就是那份清單），所以 `ia.test.ts` 的 R2 檢查判它
 * 合規，**§0.2 那份只能縮短的偏離帳一列都沒有多**。
 *
 * 曾經被檔案註解寫成不能做的 `/create` 是**另一個形狀**：動詞當清單位址（§0.2 裡
 * `/lab/run` 那一列就是同一個毛病，記著「待修」）。換成名詞之後那個理由就不成立了。
 *
 * ── 旗標讀在這裡，而這件事有代價 ──────────────────────────────────────────────
 *
 * ⛔ `01` §10 邊界 1：M5 的生成入口不得對封測使用者出現。所以這一頁**自己**要判斷，
 * 不能只靠「沒有人連過來」——一個猜到網址的封測參與者，看到的必須與其他人一樣是
 * 一句「這一頁現在不存在」，不是一個他不該知道存在的工作台。
 *
 * 代價是 `ia.test.ts` 的 `FLAG_OFF_ASSERTED` 名冊：它宣告只能縮短，而且上限本來是
 * 三。這一頁是第四個。上限與理由同一批改在那支測試裡，並且**先寫了旗標關閉的斷言**
 * 才加名字（`create-skill.test.tsx`），順序就是那份名冊要求的順序。
 */
export function CreateSkill() {
  const generateExposed = useGenerateEntryPoint();
  const creationExposed = useCreationEntryPoint();

  if (!generateExposed) {
    return (
      <>
        <h1>這一頁現在不存在</h1>
        <p>
          你可以回到 <Link to="/workspace/skills">我的 Skill</Link>，或到{" "}
          <Link to="/" search={{}}>
            目錄
          </Link>{" "}
          看看已經有的。
        </p>
      </>
    );
  }

  return creationExposed ? <CreationSession /> : <GenerateSkill />;
}
