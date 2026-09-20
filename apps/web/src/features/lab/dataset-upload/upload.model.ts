import type { DatasetLimits } from "../lab.service";

export function roundedBytes(n: number): string {
  if (n >= 1 << 20) return `${Math.round(n / (1 << 20))} MB`;
  if (n >= 1 << 10) return `${Math.round(n / (1 << 10))} KB`;
  return `${n} B`;
}

export interface TestCaseUsage {
  fileCount: number;
  totalBytes: number;
}

export function uploadRefusal(
  file: { name: string; size: number },
  limits: DatasetLimits,
  used: TestCaseUsage,
): string {
  if (file.size > limits.max_file_bytes) {
    return `${file.name} 是 ${roundedBytes(file.size)}，超過單一檔案上限 ${roundedBytes(
      limits.max_file_bytes,
    )}，沒有送出。`;
  }
  if (used.fileCount + 1 > limits.max_files_per_test_case) {
    return `這個 Test Case 已經有 ${used.fileCount} 個檔案，達到上限 ${limits.max_files_per_test_case} 個，沒有送出。請先刪掉一個再上傳。`;
  }
  const remaining = limits.max_test_case_bytes - used.totalBytes;
  if (file.size > remaining) {
    return `這個 Test Case 還剩 ${roundedBytes(remaining)} 可用，${file.name} 是 ${roundedBytes(
      file.size,
    )}，沒有送出。請先刪掉一些檔案再上傳。`;
  }
  return "";
}
