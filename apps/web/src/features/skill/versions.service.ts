import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ApiError,
  apiFetch,
  isWriteResultUnknown as isVersionUploadResultUnknown,
} from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import { isCategorizedFindings, isUploadResult } from "../creation";

export async function saveSkillVersion(skillId: string, file: File) {
  const result = await apiFetch<unknown>(`/skills/${skillId}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/zip" },
    body: file,
  });
  if (!isUploadResult(result) || result.skill_id.toLowerCase() !== skillId.toLowerCase())
    throw new Error("Invalid version upload response");
  return {
    ...result,
    skill_id: result.skill_id.toLowerCase(),
    version_id: result.version_id.toLowerCase(),
  };
}

export function readVersionUploadFindings(error: unknown) {
  return error instanceof ApiError && error.status === 422 && isCategorizedFindings(error.body)
    ? error.body
    : undefined;
}

export { isWriteResultUnknown as isVersionUploadResultUnknown } from "../../core/api/client";

export function useSaveSkillVersion(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => saveSkillVersion(skillId, file),
    onSettled: (_result, error) => {
      if (!error || isVersionUploadResultUnknown(error)) {
        void client.invalidateQueries({ queryKey: queryKeys.skills.detail(skillId) });
      }
    },
  });
}
