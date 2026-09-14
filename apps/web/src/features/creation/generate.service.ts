import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import { useMe } from "../../core/session/me.service";
import { queryKeys } from "../../core/api/queryKeys";
import type {
  GenerateSkillRequest,
  GenerateSkillResult,
  GenerationFailure,
} from "../../core/api/types";

export function generateSkill(request: GenerateSkillRequest) {
  const body: GenerateSkillRequest = {};
  if (request.task_description) body.task_description = request.task_description;
  if (request.diagram) body.diagram = request.diagram;
  if (request.reference_skill_ids?.length) body.reference_skill_ids = request.reference_skill_ids;
  return apiFetch<GenerateSkillResult>("/skills/generate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function useGenerateSkill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: generateSkill,
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.skills.own }),
    onSettled: () => client.invalidateQueries({ queryKey: queryKeys.generate.failures }),
  });
}

export function useGenerateEntryPoint(): boolean {
  const me = useMe();
  return me.data?.features?.generate_skill === true;
}

export function useGenerateFailures() {
  return useQuery({
    queryKey: queryKeys.generate.failures,
    queryFn: () => apiFetch<{ failures: GenerationFailure[] }>("/skills/generate/failures"),
  });
}
