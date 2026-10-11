import {
  CreateTestCaseBodyNameMaxLength as MAX_NAME_CHARS,
  CreateTestCaseBodyUserPromptMaxLength as MAX_PROMPT_CHARS,
} from "@skillhub/api-client-ts/limits";

export { MAX_NAME_CHARS, MAX_PROMPT_CHARS };

const characterCount = (s: string) => [...s].length;

export function oversizeReason(name: string, prompt: string): string | null {
  const n = characterCount(name);
  if (n > MAX_NAME_CHARS) return `名稱最多 ${MAX_NAME_CHARS} 個字，目前 ${n} 個字。`;
  const p = characterCount(prompt);
  if (p > MAX_PROMPT_CHARS) return `Prompt 最多 ${MAX_PROMPT_CHARS} 個字，目前 ${p} 個字。`;
  return null;
}
