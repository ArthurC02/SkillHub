import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import type { CategorizedFindings, ImportResult } from "../../core/api/types";

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

export interface SkillImportLimits {
  max_zip_bytes: number;
  max_unpacked_bytes: number;
  max_files: number;
  max_file_bytes: number;
  max_path_depth: number;
  allowed_hosts: string[];
  note: string;
}

export function useSkillImportLimits() {
  return useQuery({
    queryKey: queryKeys.skills.importLimits,
    queryFn: () => apiFetch<SkillImportLimits>("/skills/import/limits"),
  });
}
