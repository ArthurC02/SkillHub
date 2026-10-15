import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, isWriteResultUnknown } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import type {
  CategorizedFindings,
  ImportFinding,
  ImportResult,
  ImportedSkill,
  RefusedSkill,
  UploadResult,
} from "../../core/api/types";

export function importSkillFromURL(url: string) {
  return apiFetch<unknown>("/skills/import/url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  }).then(readImportResult);
}

export function uploadSkillPackage(file: File) {
  return apiFetch<unknown>("/skills/import/upload", {
    method: "POST",
    headers: { "Content-Type": "application/zip" },
    body: file,
  }).then(readImportResult);
}

export type ImportSource = { url: string } | { file: File | undefined };

export function useImportSkill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (source: ImportSource) => {
      if ("url" in source) return importSkillFromURL(source.url);
      if (!source.file) return Promise.reject(new Error("請選擇 zip 套件。"));
      return uploadSkillPackage(source.file);
    },
    onSettled: (_result, error) => {
      if (!error || isWriteResultUnknown(error))
        return client.invalidateQueries({ queryKey: queryKeys.skills.own });
    },
  });
}

export function isImportResult(value: unknown): value is ImportResult {
  if (typeof value !== "object" || value === null) return false;
  const body = value as Partial<ImportResult>;
  return (
    ["skill", "plugin", "tree"].includes(body.shape ?? "") &&
    Array.isArray(body.skills) &&
    body.skills.every(isImportedSkill) &&
    Array.isArray(body.refused) &&
    body.refused.every(isRefusedSkill) &&
    Array.isArray(body.excluded_components) &&
    body.excluded_components.every(isImportFinding) &&
    (body.plugin === undefined ||
      (body.plugin !== null &&
        typeof body.plugin === "object" &&
        typeof body.plugin.name === "string" &&
        (body.plugin.version === undefined || typeof body.plugin.version === "string") &&
        (body.plugin.repository === undefined || typeof body.plugin.repository === "string")))
  );
}

function readImportResult(value: unknown): ImportResult {
  if (!isImportResult(value) || value.skills.length === 0)
    throw new Error("Invalid import response");
  return value;
}

function isImportedSkill(value: unknown): value is ImportedSkill {
  return isUploadResult(value) && typeof (value as Partial<ImportedSkill>).path === "string";
}

function isRefusedSkill(value: unknown): value is RefusedSkill {
  if (!value || typeof value !== "object") return false;
  const refused = value as Partial<RefusedSkill>;
  return typeof refused.path === "string" && isCategorizedFindings(refused.findings);
}

export function isUploadResult(value: unknown): value is UploadResult {
  if (!value || typeof value !== "object") return false;
  const result = value as Partial<UploadResult>;
  return (
    [result.skill_id, result.version_id].every(
      (id) =>
        typeof id === "string" &&
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id),
    ) &&
    typeof result.version_number === "number" &&
    Number.isSafeInteger(result.version_number) &&
    result.version_number > 0 &&
    typeof result.content_hash === "string" &&
    result.content_hash.trim().length > 0 &&
    typeof result.duplicate === "boolean" &&
    isCategorizedFindings(result.findings)
  );
}

export function isCategorizedFindings(value: unknown): value is CategorizedFindings {
  if (typeof value !== "object" || value === null) return false;
  const body = value as Partial<CategorizedFindings>;
  return (
    Array.isArray(body.errors) &&
    Array.isArray(body.warnings) &&
    Array.isArray(body.infos) &&
    [body.errors, body.warnings, body.infos].flat().every(isImportFinding)
  );
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

export interface SkillImportLimits {
  max_zip_bytes: number;
  max_unpacked_bytes: number;
  max_files: number;
  max_file_bytes: number;
  max_path_depth: number;
  max_skills_per_import: number;
  allowed_hosts: string[];
  note: string;
}

export function useSkillImportLimits() {
  return useQuery({
    queryKey: queryKeys.skills.importLimits,
    queryFn: async () => {
      const limits = await apiFetch<SkillImportLimits>("/skills/import/limits");
      if (!Array.isArray(limits?.allowed_hosts)) throw new Error("Invalid import limits");
      return limits;
    },
  });
}
