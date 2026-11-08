import {
  CreateTestCaseBodyNameMaxLength as MAX_NAME_CHARS,
  CreateTestCaseBodyUserPromptMaxLength as MAX_PROMPT_CHARS,
} from "@skillhub/api-client-ts/limits";
import { textLengthReason } from "../../../shared/textLength.model";

export { MAX_NAME_CHARS, MAX_PROMPT_CHARS, textLengthReason };

export function oversizeReason(name: string, prompt: string): string | null {
  return (
    textLengthReason(name.trim(), "名稱", MAX_NAME_CHARS) ??
    textLengthReason(prompt.trim(), "Prompt ", MAX_PROMPT_CHARS)
  );
}

export const CREDIT_REFUSED = "點數不足，這次沒有呼叫模型、也沒有扣點。請管理者加點後再試。";
