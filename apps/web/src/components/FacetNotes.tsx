/**
 * 一份清單的但書：哪幾句可以只講一次，以及一列該不該再自己印一次。
 *
 * 這段規則與它的兩個分支原本住在 `pages/Home.tsx`，是搜尋結果那份清單的私有函式。
 * 2026-09-07 搬到這裡，因為 `/workspace/skills` 需要同一個機制——**而這份文件自己
 * 說過，修法是套用既有的那一個，不是寫第二份**（設計 §4.3 的措辭是給卡片族的，但
 * 它的理由對一段判斷邏輯只有更強：兩份複述第一次不一致的那天，兩頁就會對同一件事
 * 講不同的話）。搬動沒有改變規則，只加了一道 `by` 的守衛，見下。
 *
 * **一句話可以搬到清單層級，若且唯若讀者搬完之後仍然分得出它在講哪一列。** 兩種
 * 情況滿足它，而它們是 `facetNoteLines` 的兩個分支：
 *
 * 1. **這一句在整份清單上逐位元相同**——那它從第 2 列起就不可能改變任何判斷，講一次
 *    就是全部。設計 §2.13 去重 1 的字面。
 * 2. **這一句有幾種值，但每一列都戴著挑出自己那一行的那個詞**（`by`）——`tier` 與
 *    `category` 的每一列都渲染 `LabelledBadge`，badge 上的 `label` 與 note 來自同一個
 *    `Labelled`，所以「來源層級「精選」：…」指得回它限定的那些列。
 *
 * **沒有那個詞就不准搬，這條是承重的。** `risk.note` 有兩種值——「掃過了」與「尚無
 * 掃描紀錄」——而 `scan_status: "unavailable"` 配 `level: "none"` 的那一列身上**一個
 * 標記都沒有**。兩句一起提到上面，畫面會同時印出兩句而讀者分不出哪一列是哪一句：
 * 那不是去重，是把 §2.9 的型別詞弄丟（`disc.test.tsx` 的「a result row carries all
 * seven columns」在這個版本第一次寫錯時就是這樣紅的）。
 *
 * 一列的清單也不搬：那時候沒有複述可去，搬走只是把但書推離它限定的那顆徽章。
 * 任何一列的 note 是空的也不搬——那一列沒有東西被講到，剩下的列會被代表。
 *
 * **新增的守衛（2026-09-07，`/workspace/skills` 這個呼叫端逼出來的）**：`by` 分支
 * 還要求**一個詞只能對應一句話**。搜尋那兩個 facet 天生滿足它（`tier`／`category`
 * 的 note 是 label 的函數），所以這道守衛對既有呼叫端是恆真的；但工作區清單的
 * `verification` 不是——兩列都可能戴著「已掃描（來源）」而句子裡各自嵌著**不同的
 * 來源 Skill 名稱**。那種情況下第 2 分支會印出兩行開頭一模一樣的但書，讀者一樣分不
 * 出哪一列是哪一句，也就是第 2 分支自己要防的那件事，只是換一個入口發生。撞到就
 * 整個 facet 不搬，句子留在列上。
 */
import type { ReactElement } from "react";

export type FacetNote<Row> = {
  /** 這份清單自己的鍵；只用來讓呼叫端問「這一個搬走了沒有」。 */
  key: string;
  /** 印在清單層級那一行前面的 facet 名字，例如 `來源層級`。 */
  label: string;
  note: (row: Row) => string | undefined;
  /**
   * 這一列身上已經戴著、而且對得回這一句的那個詞。沒有它就只有第 1 分支
   * （逐位元相同）能搬。
   */
  by?: (row: Row) => string;
};

/** 這一份清單上，哪幾個 facet 的 note 已經被提到清單層級講過了。 */
export type LiftedNotes = Record<string, boolean>;

export function facetNoteLines<Row>(
  rows: Row[],
  facets: Array<FacetNote<Row>>,
): Array<{ key: string; text: string }> {
  if (rows.length < 2) return [];
  const lines: Array<{ key: string; text: string }> = [];
  for (const { key, label, note, by } of facets) {
    // note → the row-visible words carrying it, in first-seen order.
    const byNote = new Map<string, Set<string>>();
    let complete = true;
    for (const row of rows) {
      const text = note(row);
      if (!text) {
        complete = false;
        break;
      }
      const words = byNote.get(text) ?? new Set<string>();
      if (by) words.add(by(row));
      byNote.set(text, words);
    }
    if (!complete) continue;
    if (byNote.size === 1) {
      lines.push({ key, text: `${label}：${[...byNote.keys()][0]}` });
      continue;
    }
    if (!by) continue;
    // 一個詞只能對應一句話，否則兩行但書會用同一個詞開頭。
    const claimed = new Map<string, string>();
    let unambiguous = true;
    for (const [text, words] of byNote) {
      for (const word of words) {
        const owner = claimed.get(word);
        if (owner !== undefined && owner !== text) {
          unambiguous = false;
          break;
        }
        claimed.set(word, text);
      }
      if (!unambiguous) break;
    }
    if (!unambiguous) continue;
    for (const [text, words] of byNote) {
      lines.push({ key, text: `${label}「${[...words].join("、")}」：${text}` });
    }
  }
  return lines;
}

export function liftedNotes<Row>(rows: Row[], facets: Array<FacetNote<Row>>): LiftedNotes {
  const lifted: LiftedNotes = {};
  for (const { key } of facetNoteLines(rows, facets)) lifted[key] = true;
  return lifted;
}

export function FacetNotes<Row>({
  rows,
  facets,
}: {
  rows: Row[];
  facets: Array<FacetNote<Row>>;
}): ReactElement {
  // 帶著 facet 名稱——與 `CompatibilityStatus` 逐軸註記同一個寫法（`{label}：{note}`）。
  // 沒有新元件、沒有新 class、沒有新字級。
  return (
    <>
      {facetNoteLines(rows, facets).map(({ text }) => (
        <p key={text} className="note">
          {text}
        </p>
      ))}
    </>
  );
}
