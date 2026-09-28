import type {
  CreatedDownloadArtifact,
  PackagingTarget,
  PackagingTargetId,
} from "../packaging.service";

export function resolveTarget(
  chosen: PackagingTargetId | "",
  targets: PackagingTarget[] | undefined,
): PackagingTargetId | "" {
  return chosen || (targets?.[0]?.id ?? "");
}

export function matchingBuiltArtifact(
  build: {
    data: CreatedDownloadArtifact | undefined;
    variables: { versionId: string } | undefined;
  },
  skillId: string,
  versionId: string,
): CreatedDownloadArtifact | null {
  return build.data?.skill_id === skillId && build.variables?.versionId === versionId
    ? build.data
    : null;
}
