import type { SkillImportLimits } from "./import.service";

export function mb(bytes: number): string {
  return (bytes / (1 << 20)).toFixed(1).replace(/\.0$/, "") + " MB";
}

function sizesApart(size: number, ceiling: number): [string, string] {
  if (mb(size) !== mb(ceiling)) return [mb(size), mb(ceiling)];
  return [`${size.toLocaleString("en-US")} 位元組`, `${ceiling.toLocaleString("en-US")} 位元組`];
}

export function importRefusal(
  source: "url" | "upload",
  url: string,
  file: { name: string; size: number } | undefined,
  rules: SkillImportLimits,
): string | null {
  if (source === "upload") {
    if (!file || file.size <= rules.max_zip_bytes) return null;
    const [size, ceiling] = sizesApart(file.size, rules.max_zip_bytes);
    return `${file.name} 是 ${size}，超過 zip 上限 ${ceiling}，沒有送出。`;
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
