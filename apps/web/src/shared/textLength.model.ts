export const runeCount = (text: string) => [...text].length;

export function textLengthReason(text: string, label: string, maximum: number): string | null {
  const count = runeCount(text);
  return count > maximum ? `${label}最多 ${maximum} 個字，目前 ${count} 個字。` : null;
}

export function fieldCount(text: string, maximum: number): string {
  const count = runeCount(text);
  const over = count > maximum ? "——超過了，送出會被伺服器擋下" : "";
  return `${count.toLocaleString("zh-TW")} / ${maximum.toLocaleString("zh-TW")} 字${over}`;
}
