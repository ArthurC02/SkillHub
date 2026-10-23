import type { FindingSeverityEnum } from "@skillhub/api-client-ts";
import { contractEnum } from "./contract-enum";
import type { CategorizedFindings, ImportFinding } from "./types";

const FINDING_SEVERITIES = contractEnum<FindingSeverityEnum>({
  error: true,
  warning: true,
  info: true,
});

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
    FINDING_SEVERITIES.includes(finding.severity ?? "") &&
    typeof finding.code === "string" &&
    typeof finding.message === "string" &&
    (finding.path === undefined || typeof finding.path === "string") &&
    (finding.details === undefined ||
      (Array.isArray(finding.details) &&
        finding.details.every((detail) => typeof detail === "string")))
  );
}
