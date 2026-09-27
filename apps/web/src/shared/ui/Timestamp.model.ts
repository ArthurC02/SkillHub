const ABSOLUTE = new Intl.DateTimeFormat("zh-TW", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export function formatAt(at: string): string {
  const date = new Date(at);
  if (Number.isNaN(date.getTime())) return `${at}（無法解讀的時間格式）`;
  return ABSOLUTE.format(date);
}
