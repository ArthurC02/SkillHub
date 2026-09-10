import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import { useMe } from "./me";
import type { GenerateSkillRequest, GenerateSkillResult, GenerationFailure } from "./types";

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
  return useMutation({ mutationFn: generateSkill });
}

export function useGenerateEntryPoint(): boolean {
  const me = useMe();
  return me.data?.features?.generate_skill === true;
}

export function useGenerateFailures() {
  return useQuery({
    queryKey: ["generate", "failures"],
    queryFn: () => apiFetch<{ failures: GenerationFailure[] }>("/skills/generate/failures"),
    retry: false,
  });
}
