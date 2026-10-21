import type { Acquisition } from "./publishing.service";

export class AcquisitionResultUnknown extends Error {
  constructor() {
    super("下載結果尚未確認。請先核對交付紀錄，再決定是否重試。");
  }
}

export function readAcquisition(value: unknown): Acquisition {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    throw new AcquisitionResultUnknown();
  const receipt = value as Record<string, unknown>;
  if (
    typeof receipt.artifact_id !== "string" ||
    !/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(receipt.artifact_id) ||
    typeof receipt.file_name !== "string" ||
    typeof receipt.size_bytes !== "number" ||
    !Number.isInteger(receipt.size_bytes) ||
    typeof receipt.content_hash !== "string" ||
    typeof receipt.expires_at !== "string" ||
    !Number.isFinite(Date.parse(receipt.expires_at)) ||
    typeof receipt.duplicate !== "boolean" ||
    receipt.content_url !== `/downloads/${receipt.artifact_id}/content`
  )
    throw new AcquisitionResultUnknown();
  return value as Acquisition;
}
