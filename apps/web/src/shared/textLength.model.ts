export function textLengthReason(text: string, label: string, maximum: number): string | null {
  const count = [...text].length;
  return count > maximum ? `${label}最多 ${maximum} 個字，目前 ${count} 個字。` : null;
}
