export const MAX_NAME_BYTES = 200;

export const MAX_PROMPT_BYTES = 32768;

// UTF-8 byte length, not JS string length (UTF-16 units) — the server's limit is bytes.
function byteLength(s: string): number {
  return new TextEncoder().encode(s).length;
}

export function oversizeReason(name: string, prompt: string): string | null {
  const n = byteLength(name);
  if (n > MAX_NAME_BYTES) return `名稱最多 ${MAX_NAME_BYTES} bytes，目前 ${n} bytes。`;
  const p = byteLength(prompt);
  if (p > MAX_PROMPT_BYTES) return `Prompt 最多 ${MAX_PROMPT_BYTES} bytes，目前 ${p} bytes。`;
  return null;
}
