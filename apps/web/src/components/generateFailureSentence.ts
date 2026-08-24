import type { GenerationFailure } from "../api/types";

// Not in GenerateSkill.tsx, for oxlint's only-export-components: a component
// file that also exports a table breaks Fast Refresh, and the rule counts it.
// The table is exported because generate.test.tsx asserts it against the
// generated enum.

/**
 * One row's sentence, keyed on the contract's enum so that adding a value to
 * the union without a sentence fails `tsc` here (the PACKAGING_BLOCKED_LABEL
 * pattern; generate.test.tsx asserts the table against the generated enum so
 * a value added to the contract and not to this union fails too).
 *
 * Two of the failures are things the user can act on — a truncated answer
 * means make the task smaller, a collision means rename the other skill — and
 * they take precedence over the kind, because they are the two with a next
 * step. The rest say what happened and stop, because 「再試一次」 is the only
 * answer and it is already a button.
 */
export const FAILURE_SENTENCE: Record<GenerationFailure["failure"], (f: GenerationFailure) => string> = {
  quota: () => "額度不足，沒有呼叫模型，也沒有花錢。",
  // Not "額度不足": the allowance could not be counted, and a healthy account
  // must not be told it ran out (d555564 fixed the 422; this is the same
  // sentence on the record).
  unavailable: () => "當時算不出剩餘額度，平台沒有冒險呼叫模型，也沒有花錢。",
  // "Gateway" on the Go side covers both a service that did not answer and one
  // that answered something the platform could not use (an empty body, an
  // answer over a contract cap). The sentence must not claim the first.
  gateway: () => "模型服務那一端失敗——沒有回應，或回了平台用不了的答案。沒有建立任何版本。",
  unpackageable: () => "模型交出來的東西沒辦法打包成一個套件。",
  rejected: () => "驗證之後被拒絕，沒有建立任何版本。",
  blocked: (f) =>
    f.codes?.length
      ? `套件沒有通過驗證（${f.codes.join("、")}），試了 ${f.attempts} 次。`
      : `套件沒有通過驗證，試了 ${f.attempts} 次。`,
  // The row exists and its timestamp is real; only its detail is missing.
  // Saying so beats dropping the row, which would silently shorten history.
  "": () => "這一次沒有成功，而這列紀錄的細節讀不出來。",
};

export function failureSentence(f: GenerationFailure): string {
  if (f.truncated) return "模型的輸出超過一次生成的上限，已經停下。把任務拆小一點再試會有幫助。";
  // Not 「而它不是生成的」: since the guard widened (81ae767), the most common
  // collision is a REGENERATION landing on the earlier generated skill — the
  // model takes the same name from the same task. The sentence must be true
  // for both kinds of neighbour.
  if (f.collision)
    return "工作區已經有一個同名的 Skill。刪掉它（或改掉它的名字）再生成一次——同一段描述通常會讓模型取到同一個名字。";
  // A value this build does not know falls to the "" sentence: the row
  // happened, the detail is unreadable to this build. Own-property, not a
  // bare lookup — a wire value of "constructor" must not reach the table's
  // prototype (the RedistributionBadge guard, for the same reason).
  const sentence = Object.prototype.hasOwnProperty.call(FAILURE_SENTENCE, f.failure)
    ? FAILURE_SENTENCE[f.failure]
    : FAILURE_SENTENCE[""];
  return sentence(f);
}
