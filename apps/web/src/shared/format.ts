const oneDecimal = (n: number) => n.toFixed(1).replace(/\.0$/, "");

export function bytes(n: number): string {
  if (n >= 1024 ** 3) return `${oneDecimal(n / 1024 ** 3)} GB`;
  if (n >= 1024 ** 2) return `${oneDecimal(n / 1024 ** 2)} MB`;
  if (n >= 1024) return `${oneDecimal(n / 1024)} KB`;
  return `${n} B`;
}
