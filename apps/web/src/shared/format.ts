const oneDecimal = (n: number) => n.toFixed(1).replace(/\.0$/, "");

export function bytes(n: number): string {
  if (n >= 1024 ** 3) return `${oneDecimal(n / 1024 ** 3)} GB`;
  if (n >= 1024 ** 2) return `${oneDecimal(n / 1024 ** 2)} MB`;
  if (n >= 1024) return `${oneDecimal(n / 1024)} KB`;
  return `${n} B`;
}

export const points = (n: number) => `${n} 點`;

export const credits = (value: number | null) => (value === null ? "未測量" : points(value));

export const ESTIMATE_MARK = "（估計值）";
