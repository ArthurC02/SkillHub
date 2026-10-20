import type { CategorizedFindings, ImportFinding } from "./types";

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

export function isImportFinding(value: unknown): value is ImportFinding {
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
