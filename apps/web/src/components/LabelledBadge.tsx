import type { Labelled } from "../api/types";

/**
 * Renders a server-owned `Labelled` (trust, license status, tier). The copy
 * comes from the API on purpose: NFR-001 requires every surface to explain a
 * trust state with the same factual wording, so the client must not keep its
 * own enum-to-Chinese map that can drift from what was actually checked.
 */
/**
 * The `note` is VISIBLE, and that is the whole point of this component.
 *
 * It used to be `title={value.note}` and nothing else, on every `tier`,
 * `trust`, `license` and `redistribution` badge in the app. The clearest cost:
 * the server's `tier.note` is 「收錄不等於精選。」 — a sentence whose entire
 * reason for existing is to stop a reader treating the badge as an endorsement
 * — and on the search results page it existed only in a tooltip, which on a
 * touch device is not anywhere. `docs/design/system.md` §2.4 第 3 項 names this
 * component by name, and §2.11(c) makes it a prohibition rather than a
 * preference: 「每一個驗證徽章要在同一個區塊、以文字說出它不涵蓋什麼」, on the
 * evidence of the npm provenance badge that carried no such sentence while a
 * worm shipped 84 signed malicious versions. §0 ranks 安全與不誤導 first.
 *
 * ~~`title` stays as well: the rule is 「可以在 tooltip 裡，不可以只在 tooltip 裡」.~~
 * **2026-09-03 (設計 §2.13 去重 2, ADR-065): the `title` is gone.** That rule
 * permitted the tooltip; it did not ask for it, and by the time the sentence is
 * visible the tooltip only makes a screen reader say it twice. It also became
 * actively wrong the moment `noteInRow` existed: on a row whose note has been
 * lifted to the list, a surviving `title` would be the row's ONLY copy — which
 * is 設計 §2.4 第 3 項, the exact defect this component was written to fix, and
 * `design-system.test.ts` could not see it (it matches strings within one file,
 * and `{value.note}` is still rendered here on the other branch).
 *
 * A `<span>` rather than a `<p>`: this renders inside `<dd>`, `<td>`, `<p>` and
 * another `<span>` (LicenseBadge), and a `<p>` is invalid in the last two.
 * Callers that printed the same string themselves have stopped — the same fact
 * twice on one screen is 設計 §3 第 14 條.
 */
export function LabelledBadge({
  kind,
  value,
  noteInRow = true,
}: {
  kind: string;
  value: Labelled;
  /**
   * 設計 §2.13 去重 1（ADR-065）. False when the LIST this badge sits in already
   * prints this `note` once, above the rows — which is only ever true when the
   * sentence is byte-identical down the whole list, and so cannot be what tells
   * one row from another. The badge and its label never move; only the sentence
   * does. Defaults to today's behaviour, so the four other callers
   * (Compare／SkillDetail／WorkspaceSkills／Packaging) are untouched.
   *
   * Same shape and same reason as `Home.tsx`'s `rankNoteInList`.
   */
  noteInRow?: boolean;
}) {
  return (
    <>
      <span className={`badge badge-${kind}-${value.value}`}>{value.label}</span>
      {noteInRow && value.note && <span className="note">{value.note}</span>}
    </>
  );
}
