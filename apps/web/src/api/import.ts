import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import { queryKeys } from "./queryKeys";

export type ImportFinding = {
  severity: "error" | "warning" | "info";
  code: string;
  path?: string;
  message: string;
  details?: string[];
};

export type CategorizedFindings = {
  errors: ImportFinding[];
  warnings: ImportFinding[];
  infos: ImportFinding[];
};

export type ImportResult = {
  skill_id: string;
  version_id: string;
  version_number: number;
  content_hash: string;
  duplicate: boolean;
  findings: CategorizedFindings;
};

export function importSkillFromURL(url: string) {
  return apiFetch<ImportResult>("/skills/import/url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
}

export function uploadSkillPackage(file: File) {
  return apiFetch<ImportResult>("/skills/import/upload", {
    method: "POST",
    headers: { "Content-Type": "application/zip" },
    body: file,
  });
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
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.skills.own }),
  });
}

export function isCategorizedFindings(value: unknown): value is CategorizedFindings {
  if (typeof value !== "object" || value === null) return false;
  const body = value as Partial<CategorizedFindings>;
  return Array.isArray(body.errors) && Array.isArray(body.warnings) && Array.isArray(body.infos);
}
