import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { ImportResult } from "./import";

export function saveSkillVersion(skillId: string, file: File) {
  return apiFetch<ImportResult>(`/skills/${skillId}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/zip" },
    body: file,
  });
}

export function useSaveSkillVersion(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => saveSkillVersion(skillId, file),
    onSuccess: () => client.invalidateQueries({ queryKey: ["skills", skillId] }),
  });
}
