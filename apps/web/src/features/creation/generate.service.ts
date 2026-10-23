import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import { useMe } from "../../core/session/me.service";
import { queryKeys } from "../../core/api/queryKeys";
import type {
  GenerateAllowance,
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
    onSettled: () =>
      Promise.all([
        client.invalidateQueries({ queryKey: queryKeys.generate.failures }),
        client.invalidateQueries({ queryKey: queryKeys.generate.allowance }),
      ]),
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

function isGenerateAllowance(value: unknown): value is GenerateAllowance {
  const counts = (record: unknown, keys: string[]) =>
    typeof record === "object" &&
    record !== null &&
    keys.every((key) => Number.isInteger((record as Record<string, unknown>)[key]));
  const allowance = value as GenerateAllowance;
  return (
    counts(value, ["remaining_today", "remaining_window"]) &&
    typeof allowance.window_resets_at === "string" &&
    Number.isFinite(Date.parse(allowance.window_resets_at)) &&
    counts(allowance.limits, ["daily", "window", "window_days"])
  );
}

export function useGenerateAllowance() {
  return useQuery({
    queryKey: queryKeys.generate.allowance,
    queryFn: async () => {
      const result = await apiFetch<unknown>("/skills/generate/allowance");
      if (!isGenerateAllowance(result)) throw new Error("Invalid generate allowance response");
      return result;
    },
  });
}
