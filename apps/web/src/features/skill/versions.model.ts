import type { CandidateVersion, ListedSkillVersion } from "../../core/api/types";
import { IN_PROGRESS, type Absence } from "../../shared/ui/absence";

const VERSIONS_FAILED: Absence = "測量失敗";
const VERSION_NOT_LISTED: Absence = "未測量";

export type VersionsRead = { isPending: boolean; error: unknown };

export function versionNumberWord(number: number | undefined, read: VersionsRead): string {
  if (number !== undefined) return `v${number}`;
  if (read.isPending) return `版本編號${IN_PROGRESS}`;
  if (read.error) return `版本編號${VERSIONS_FAILED}`;
  return `版本編號${VERSION_NOT_LISTED}`;
}

export const inLine = (version: ListedSkillVersion) =>
  !version.held_for_evaluation || version.promoted;

export const lineNewest = <T extends ListedSkillVersion>(newestFirst: readonly T[]) =>
  newestFirst.find(inLine);

export function heldLabel(version: ListedSkillVersion, candidates: readonly CandidateVersion[]) {
  if (inLine(version)) return undefined;
  const built = candidates.find(
    (c) => c.version_id.toLowerCase() === version.version_id.toLowerCase(),
  );
  return built ? `第 ${built.round} 輪候選 ${built.candidate + 1}，保留中` : "保留中";
}
