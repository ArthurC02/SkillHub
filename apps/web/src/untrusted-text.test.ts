import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

/**
 * `04` 丙-210 的顯示半邊，寫成名冊而不是三個逐頁測試。
 *
 * # 為什麼是這個形狀
 *
 * 這條規則的失效方式不是「現有的某一處寫錯了」，是**下一個人再開一處**。第一版
 * 只把 `Reveal` 接到互動創作的六處，而同一個論證在別的地方更成立：`SkillFiles`
 * 把**匯入套件的 SKILL.md** 攤給人看，`RunEvaluation` 的建議差異是「採用＝建立
 * 新版本」（鐵律 4）那一步在讀的東西，`RunCompare` 的版本差異是判斷「這個 Skill
 * 可不可信」的證據。三處當時都沒有標記，而漏掉它們的原因就是逐頁思考。
 *
 * 所以測的是類別：**任何把不受信任的文字倒進 `<pre>` 的地方，都要經過
 * `Reveal`**。判準是那兩個 class——`.skill-md`（Skill 本文與模型的程式碼區塊）
 * 與 `.diff`（版本或建議差異）——因為它們就是這個 app 用來顯示「別人寫的整段
 * 文字」的兩個面。
 *
 * # 它抓得到什麼、抓不到什麼
 *
 * 抓得到：新開一個 `<pre className="skill-md">` 或 `<pre className="diff">`
 * 卻直接塞一個表達式進去。抓不到：用第三個 class 顯示不受信任文字。那需要一份
 * 「哪些資料是不受信任的」的來源標註，而這個 app 今天沒有；在有之前，這兩個
 * class 是誠實的近似，而不是完整的證明。
 */
const dir = join(import.meta.dirname);

function tsxFiles(at: string): Array<[string, string]> {
  const out: Array<[string, string]> = [];
  for (const entry of readdirSync(at, { withFileTypes: true })) {
    const path = join(at, entry.name);
    if (entry.isDirectory()) out.push(...tsxFiles(path));
    else if (entry.name.endsWith(".tsx") && !entry.name.endsWith(".test.tsx")) {
      out.push([path, readFileSync(path, "utf8")]);
    }
  }
  return out;
}

const UNTRUSTED_PRE = /<pre className="(?:skill-md|diff)"[^>]*>\s*([^\s])/g;

test("every <pre> that shows somebody else's text goes through Reveal (04 丙-210)", () => {
  const offenders: string[] = [];
  for (const [path, source] of tsxFiles(dir)) {
    for (const m of source.matchAll(UNTRUSTED_PRE)) {
      // The only thing allowed to start the child is the component that marks
      // the characters a person cannot see. A raw `{` is the failure this
      // test exists for.
      if (m[1] !== "<") {
        offenders.push(path + ": " + m[0].replace(/\s+/g, " "));
      }
    }
  }
  expect(offenders, "這些地方把別人寫的文字直接倒進 <pre>，隱藏字元會照原樣消失在畫面上").toEqual(
    [],
  );
});

test("the roster is not empty — the scan must actually be finding these sites", () => {
  // `matchAll` rather than `test`: UNTRUSTED_PRE carries /g, and `test` on a
  // global regex advances lastIndex between calls, so the same pattern would
  // answer differently for the same file depending on what came before it.
  const found = tsxFiles(dir).filter(([, s]) => [...s.matchAll(UNTRUSTED_PRE)].length > 0).length;
  // Five files today: ModelMarkdown, CreationSession, SkillFiles, RunCompare,
  // RunEvaluation. Asserting a floor rather than the exact number keeps this
  // from failing on every new screen, while still going red if the class names
  // are renamed out from under the scan above — which would otherwise leave it
  // quietly passing over nothing.
  expect(found, "掃描找不到任何一處，那上面那條就是永遠綠的").toBeGreaterThanOrEqual(5);
});
