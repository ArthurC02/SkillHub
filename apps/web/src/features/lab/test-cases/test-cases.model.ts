import {
  CreateTestCaseBodyNameMaxLength as MAX_NAME_CHARS,
  CreateTestCaseBodyUserPromptMaxLength as MAX_PROMPT_CHARS,
} from "@skillhub/api-client-ts/limits";
import { textLengthReason } from "../../../shared/textLength.model";

export { MAX_NAME_CHARS, MAX_PROMPT_CHARS, textLengthReason };

export function oversizeReason(name: string, prompt: string): string | null {
  return (
    textLengthReason(name, "名稱", MAX_NAME_CHARS) ??
    textLengthReason(prompt, "Prompt ", MAX_PROMPT_CHARS)
  );
}
