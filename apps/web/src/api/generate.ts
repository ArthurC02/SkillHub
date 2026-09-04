import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import { useMe } from "./me";
import type { GenerateSkillRequest, GenerateSkillResult, GenerationFailure } from "./types";

/**
 * POST /skills/generate — GEN-001.
 *
 * The route exists only where the deployment turned ADR-052's exposure flag on,
 * and the way to know that is `me.features?.generate_skill`, never a failed
 * request: a feature discovered by probing has already been drawn on somebody's
 * screen.
 *
 * Synchronous, and slow — one or two model calls, no job and no run id. That is
 * why the in-flight copy says the tab must stay open: unlike a Run, closing it
 * cancels the request (design system §2.12 第 2 條 requires saying so, §2.2
 * requires it to be true).
 *
 * Three input modes as of 02:GEN-005/006 (ADR-066): task description, an
 * uploaded diagram, and up to three reference Skills — any combination, at
 * least one of task_description/diagram. Absent keys are omitted rather than
 * sent as `undefined` — an empty `reference_skill_ids: []` is a different
 * statement from "not sent" and the contract only requires the former to be
 * absent-or-nonempty on the wire.
 */
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

/**
 * Whether this deployment shows the generation entry point (ADR-052).
 *
 * Read from GET /me rather than from a build-time constant: one build has to
 * serve a cohort that sees it and a cohort that does not, and the flag's whole
 * purpose is that a beta participant who meets 「搜不到 → 生成一個」 changes what
 * 01 §11.2's first funnel segment measures — a number with one chance and twelve
 * people.
 */
export function useGenerateEntryPoint(): boolean {
  const me = useMe();
  return me.data?.features?.generate_skill === true;
}

/**
 * GET /skills/generate/failures — GEN-003's read half.
 *
 * The write half has existed since M5's first pass, and for a while that was
 * counted as 「在工作區留下可查的失敗紀錄」 being met. It was not: a row only
 * somebody with a database connection can see is not a record left in the
 * workspace, and the gap has no symptom — the write succeeds and the user sees
 * nothing at all.
 *
 * Same flag as the entry point, so this is only ever called from behind
 * `useGenerateEntryPoint`; where the flag is off the route does not exist.
 */
export function useGenerateFailures() {
  return useQuery({
    queryKey: ["generate", "failures"],
    queryFn: () => apiFetch<{ failures: GenerationFailure[] }>("/skills/generate/failures"),
    retry: false,
  });
}
