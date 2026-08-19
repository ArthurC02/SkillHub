import { apiFetch } from "./client";

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

export function isCategorizedFindings(value: unknown): value is CategorizedFindings {
  if (typeof value !== "object" || value === null) return false;
  const body = value as Partial<CategorizedFindings>;
  return Array.isArray(body.errors) && Array.isArray(body.warnings) && Array.isArray(body.infos);
}
