export function bytes(n: number): string {
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(1)} GB`;
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${n} B`;
}

export function serverSentenceOr(message: string | undefined, fallback: string): string {
  const sentence = message?.trimStart() ?? "";
  return /^[㐀-鿿]/u.test(sentence) || /^(?:Skill Hub|Bundle) [㐀-鿿]/u.test(sentence)
    ? sentence
    : fallback;
}
