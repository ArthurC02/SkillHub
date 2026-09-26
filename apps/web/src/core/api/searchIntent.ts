import type { SearchCorrection, SearchIntent } from "./types";

export const INTENT_FIELD_LIMIT = 2000; // one-number: searchMaxQueryRunes
export const INTENT_KEYWORD_LIMIT = 8; // one-number: intentMaxKeywords
export const INTENT_KEYWORD_LENGTH = 128; // one-number: intentMaxKeywordRunes
export const INTENT_FIELDS = ["input", "output", "tools", "data", "environment"] as const;

export function emptySearchIntent(): SearchIntent {
  return { input: null, output: null, tools: null, data: null, environment: null };
}

export function readSearchCorrection(serialized: string): SearchCorrection {
  let value: unknown;
  try {
    value = JSON.parse(serialized);
  } catch {
    throw new Error("搜尋修正格式無效，請重新輸入任務描述。");
  }
  if (!value || typeof value !== "object" || !("intent" in value) || !("keywords" in value)) {
    throw new Error("搜尋修正缺少意圖或關鍵詞，請重新輸入任務描述。");
  }
  const { intent, keywords } = value;
  if (
    !intent ||
    typeof intent !== "object" ||
    Object.keys(intent).length !== INTENT_FIELDS.length ||
    INTENT_FIELDS.some((field) => {
      const text = (intent as Record<string, unknown>)[field];
      return (
        text !== null &&
        (typeof text !== "string" || !text.trim() || [...text].length > INTENT_FIELD_LIMIT)
      );
    })
  ) {
    throw new Error(`意圖必須包含五欄，每欄最多 ${INTENT_FIELD_LIMIT} 字；未提及的欄位請留白。`);
  }
  if (
    !Array.isArray(keywords) ||
    keywords.length > INTENT_KEYWORD_LIMIT ||
    keywords.some(
      (text) =>
        typeof text !== "string" || !text.trim() || [...text].length > INTENT_KEYWORD_LENGTH,
    )
  ) {
    throw new Error(
      `關鍵詞最多 ${INTENT_KEYWORD_LIMIT} 組，每組最多 ${INTENT_KEYWORD_LENGTH} 字。`,
    );
  }
  const correction = { intent: intent as SearchIntent, keywords };
  if ([...correctedSearchText(correction)].length > INTENT_FIELD_LIMIT) {
    throw new Error(`五欄理解與關鍵詞合計最多 ${INTENT_FIELD_LIMIT} 字，請縮短後再搜尋。`);
  }
  return correction;
}

export function correctedSearchText({ intent, keywords }: SearchCorrection): string {
  return [...new Set([...INTENT_FIELDS.map((field) => intent[field]), ...keywords])]
    .filter((text) => text !== null)
    .join(" ");
}
