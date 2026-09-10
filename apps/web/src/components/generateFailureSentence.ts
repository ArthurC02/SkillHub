import type { GenerationFailure } from "../api/types";

export const FAILURE_SENTENCE: Record<
  GenerationFailure["failure"],
  (f: GenerationFailure) => string
> = {
  quota: () => "額度不足，沒有呼叫模型，也沒有花錢。",
  unavailable: () => "當時算不出剩餘額度，平台沒有冒險呼叫模型，也沒有花錢。",
  gateway: () => "模型服務那一端失敗——沒有回應，或回了平台用不了的答案。沒有建立任何版本。",
  unpackageable: () => "模型交出來的東西沒辦法打包成一個套件。",
  rejected: () => "驗證之後被拒絕，沒有建立任何版本。",
  blocked: (f) =>
    f.codes?.length
      ? `套件沒有通過驗證（${f.codes.join("、")}），試了 ${f.attempts} 次。`
      : `套件沒有通過驗證，試了 ${f.attempts} 次。`,
  "": () => "這一次沒有成功，而這列紀錄的細節讀不出來。",
};

export function failureSentence(f: GenerationFailure): string {
  if (f.truncated) return "模型的輸出超過一次生成的上限，已經停下。把任務拆小一點再試會有幫助。";
  if (f.collision)
    return "工作區已經有一個同名的 Skill。刪掉它（或改掉它的名字）再生成一次——同一段描述通常會讓模型取到同一個名字。";
  // hasOwnProperty, not a bare lookup: a wire value like "constructor" would
  // otherwise resolve to Object.prototype's own method instead of falling through.
  const sentence = Object.prototype.hasOwnProperty.call(FAILURE_SENTENCE, f.failure)
    ? FAILURE_SENTENCE[f.failure]
    : FAILURE_SENTENCE[""];
  return sentence(f);
}
