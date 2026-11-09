import type { SkillImportLimits } from "./import.service";
import { bytes, exactBytes } from "../../shared/format";

function sizesApart(size: number, ceiling: number): [string, string] {
  if (bytes(size) !== bytes(ceiling)) return [bytes(size), bytes(ceiling)];
  return [exactBytes(size), exactBytes(ceiling)];
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
