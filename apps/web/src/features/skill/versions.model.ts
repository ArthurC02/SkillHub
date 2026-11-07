import type { CandidateVersion, ListedSkillVersion } from "../../core/api/types";

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
