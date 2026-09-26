import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import type { UploadResult } from "../../core/api/types";
import { queryKeys } from "../../core/api/queryKeys";

export function saveSkillVersion(skillId: string, file: File) {
  return apiFetch<UploadResult>(`/skills/${skillId}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/zip" },
    body: file,
  });
}

export function useSaveSkillVersion(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => saveSkillVersion(skillId, file),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.skills.detail(skillId) }),
  });
}
