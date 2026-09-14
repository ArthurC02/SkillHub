import type { CategorizedFindings, ImportFinding } from "../../../../core/api/types";
import { Findings } from "../../../../shared/ui/Findings";

function groupDraftFindings(raw: string): CategorizedFindings | undefined {
  try {
    const parsed = JSON.parse(raw) as { findings?: unknown };
    if (!Array.isArray(parsed.findings)) return undefined;
    const groups: CategorizedFindings = { errors: [], warnings: [], infos: [] };
    for (const item of parsed.findings as ImportFinding[]) {
      if (item.severity === "error") groups.errors.push(item);
      else if (item.severity === "warning") groups.warnings.push(item);
      else if (item.severity === "info") groups.infos.push(item);
    }
    return groups;
  } catch {
    return undefined;
  }
}

export function DraftFindings({ raw }: { raw: string }) {
  const grouped = groupDraftFindings(raw);
  return grouped ? <Findings findings={grouped} level={4} /> : <p>{raw}</p>;
}
