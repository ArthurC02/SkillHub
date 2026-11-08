import type { DatasetLimits } from "../lab.service";
import {
  ApiError,
  SessionNotReadyError,
  isInAppWords,
  isWriteResultUnknown,
} from "../../../core/api/client";
import { bytes } from "../../../shared/format";

export interface TestCaseUsage {
  fileCount: number;
  totalBytes: number;
}

export function uploadErrorSentence(error: unknown): string {
  if (error instanceof SessionNotReadyError) return error.message;
  if (!(error instanceof ApiError) || isWriteResultUnknown(error))
    return "無法確認上傳結果。請先核對下方最新檔案清單，再決定是否重試。";
  const { status, message } = error;
  if ([400, 413, 415].includes(status) && isInAppWords(message)) return message;
  if ([400, 413, 415].includes(status)) {
    return "上傳沒有成功，請檢查檔案類型與大小後再試一次。";
  }
  if (status === 404) return "這個測試題已不存在，上傳沒有成功；請回到測試題清單。";
  return "上傳沒有成功，可以再按一次。";
}

export function uploadRefusal(
  file: { name: string; size: number },
  limits: DatasetLimits,
  used: TestCaseUsage,
): string {
  if (file.size > limits.max_file_bytes) {
    return `${file.name} 是 ${bytes(file.size)}，超過單一檔案上限 ${bytes(
      limits.max_file_bytes,
    )}，沒有送出。`;
  }
  if (used.fileCount + 1 > limits.max_files_per_test_case) {
    return `這個測試題已經有 ${used.fileCount} 個檔案，達到上限 ${limits.max_files_per_test_case} 個，沒有送出。請先刪掉一個再上傳。`;
  }
  const remaining = limits.max_test_case_bytes - used.totalBytes;
  if (file.size > remaining) {
    return `這個測試題還剩 ${bytes(remaining)} 可用，${file.name} 是 ${bytes(
      file.size,
    )}，沒有送出。請先刪掉一些檔案再上傳。`;
  }
  return "";
}
