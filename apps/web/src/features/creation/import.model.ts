import type { SkillImportLimits } from "./import.service";

export function mb(bytes: number): string {
  return (bytes / (1 << 20)).toFixed(1).replace(/\.0$/, "") + " MB";
}

export function importRefusal(
  source: "url" | "upload",
  url: string,
  file: { name: string; size: number } | undefined,
  rules: SkillImportLimits,
): string | null {
  if (source === "upload") {
    return file && file.size > rules.max_zip_bytes
      ? `${file.name} 是 ${mb(file.size)}，超過 zip 上限 ${mb(rules.max_zip_bytes)}，沒有送出。`
      : null;
  }
  let host: string;
  try {
    host = new URL(url.trim()).host;
  } catch {
    return null;
  }
  return rules.allowed_hosts.includes(host)
    ? null
    : `${host} 不在允許的來源（${rules.allowed_hosts.join("、")}）裡，沒有送出。`;
}
