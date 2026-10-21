const OPTIONS = {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
} satisfies Intl.DateTimeFormatOptions;

export type TimestampPrecision = "minute" | "second";

const ABSOLUTE = {
  minute: new Intl.DateTimeFormat("zh-TW", OPTIONS),
  second: new Intl.DateTimeFormat("zh-TW", { ...OPTIONS, second: "2-digit" }),
};

export function formatAt(at: string, precision: TimestampPrecision = "minute"): string {
  const date = new Date(at);
  if (Number.isNaN(date.getTime())) return `${at}（無法解讀的時間格式）`;
  return ABSOLUTE[precision].format(date);
}
