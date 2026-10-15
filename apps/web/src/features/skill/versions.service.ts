import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError, SessionNotReadyError, apiFetch } from "../../core/api/client";
import type { CategorizedFindings, ImportFinding, UploadResult } from "../../core/api/types";
import { queryKeys } from "../../core/api/queryKeys";
import { isCategorizedFindings } from "../creation";

export async function saveSkillVersion(skillId: string, file: File) {
  const result = await apiFetch<unknown>(`/skills/${skillId}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/zip" },
    body: file,
  });
  if (!isUploadResult(result, skillId)) throw new Error("Invalid version upload response");
  return {
    ...result,
    skill_id: result.skill_id.toLowerCase(),
    version_id: result.version_id.toLowerCase(),
  };
}

function isImportFinding(value: unknown): value is ImportFinding {
  if (!value || typeof value !== "object") return false;
  const finding = value as Partial<ImportFinding>;
  return (
    ["error", "warning", "info"].includes(finding.severity ?? "") &&
    typeof finding.code === "string" &&
    typeof finding.message === "string" &&
    (finding.path === undefined || typeof finding.path === "string") &&
    (finding.details === undefined ||
      (Array.isArray(finding.details) &&
        finding.details.every((detail) => typeof detail === "string")))
  );
}

function isUploadFindings(value: unknown): value is CategorizedFindings {
  return (
    isCategorizedFindings(value) &&
    [value.errors, value.warnings, value.infos].flat().every(isImportFinding)
  );
}

function isUploadResult(value: unknown, skillId: string): value is UploadResult {
  if (!value || typeof value !== "object") return false;
  const result = value as Partial<UploadResult>;
  return (
    [result.skill_id, result.version_id].every(
      (id) =>
        typeof id === "string" &&
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id),
    ) &&
    result.skill_id?.toLowerCase() === skillId.toLowerCase() &&
    typeof result.version_number === "number" &&
    Number.isSafeInteger(result.version_number) &&
    result.version_number > 0 &&
    typeof result.content_hash === "string" &&
    result.content_hash.trim().length > 0 &&
    typeof result.duplicate === "boolean" &&
    isUploadFindings(result.findings)
  );
}

export function readVersionUploadFindings(error: unknown) {
  return error instanceof ApiError && error.status === 422 && isUploadFindings(error.body)
    ? error.body
    : undefined;
}

export function isVersionUploadResultUnknown(error: unknown) {
  if (error instanceof SessionNotReadyError) return false;
  return !(error instanceof ApiError) || error.status >= 500;
}

export function useSaveSkillVersion(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => saveSkillVersion(skillId, file),
    onSettled: (_result, error) => {
      if (!error || isVersionUploadResultUnknown(error)) {
        void client.invalidateQueries({ queryKey: queryKeys.skills.detail(skillId) });
      }
    },
  });
}
