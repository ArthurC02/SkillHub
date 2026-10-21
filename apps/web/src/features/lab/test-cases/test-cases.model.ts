import {
  CreateTestCaseBodyNameMaxLength as MAX_NAME_CHARS,
  CreateTestCaseBodyUserPromptMaxLength as MAX_PROMPT_CHARS,
} from "@skillhub/api-client-ts/limits";

export { MAX_NAME_CHARS, MAX_PROMPT_CHARS };

const characterCount = (s: string) => [...s].length;

export function textLengthReason(text: string, label: string, maximum: number): string | null {
  const count = characterCount(text);
  return count > maximum ? `${label}最多 ${maximum} 個字，目前 ${count} 個字。` : null;
}

export function oversizeReason(name: string, prompt: string): string | null {
  return (
    textLengthReason(name, "名稱", MAX_NAME_CHARS) ??
    textLengthReason(prompt, "Prompt ", MAX_PROMPT_CHARS)
  );
}
