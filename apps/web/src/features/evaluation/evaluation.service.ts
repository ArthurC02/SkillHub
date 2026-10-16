import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import { useMe } from "../../core/session/me.service";
import { queryKeys } from "../../core/api/queryKeys";
import { problems, type CheckKind, type CheckNode } from "./check.model";

export type GraderScope = "workspace" | "platform";

export type Grader = {
  id: string;
  name: string;
  scope: GraderScope;
  version_id: string;
  version: number;
  instructions: string;
  revised_at: string;
};

export type CheckKinds = { kinds: CheckKind[]; max_nodes: number; max_depth: number };

export type GraderDraft = { name: string; instructions: string; reason: string };

const GRADER_PATHS: Record<GraderScope, string> = {
  workspace: "/evaluation/graders",
  platform: "/admin/evaluation/graders",
};

function send(body: unknown): RequestInit {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  };
}

export function useCheckKinds() {
  return useQuery({
    queryKey: queryKeys.checks.kinds,
    queryFn: () => apiFetch<CheckKinds>("/evaluation/check-kinds"),
    staleTime: Infinity,
  });
}

export function useGraders(scope: GraderScope) {
  const operator = useMe().data?.operator === true;
  return useQuery({
    queryKey: queryKeys.checks.graders(scope),
    queryFn: () => getGraders(scope),
    enabled: scope === "workspace" || operator,
  });
}

export async function getGraders(scope: GraderScope): Promise<{ graders: Grader[] }> {
  const result = await apiFetch<unknown>(GRADER_PATHS[scope]);
  if (
    !result ||
    typeof result !== "object" ||
    !("graders" in result) ||
    !Array.isArray(result.graders) ||
    !result.graders.every(isGrader)
  ) {
    throw new Error("Invalid grader list response");
  }
  return result as { graders: Grader[] };
}

function isGrader(value: unknown): value is Grader {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const grader = value as Partial<Grader>;
  return (
    [grader.id, grader.version_id].every(
      (id) =>
        typeof id === "string" &&
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id),
    ) &&
    typeof grader.name === "string" &&
    (grader.scope === "platform" || grader.scope === "workspace") &&
    typeof grader.version === "number" &&
    Number.isSafeInteger(grader.version) &&
    grader.version >= 1 &&
    typeof grader.instructions === "string" &&
    typeof grader.revised_at === "string" &&
    Number.isFinite(Date.parse(grader.revised_at))
  );
}

export function useCreateGrader(scope: GraderScope) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (draft: GraderDraft) => apiFetch<Grader>(GRADER_PATHS[scope], send(draft)),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.checks.all }),
  });
}

export function useReviseGrader(
  scope: GraderScope,
  graderId: string,
  onCompleted: (grader: Grader) => void,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (revision: { instructions: string; reason: string }) =>
      apiFetch<Grader>(`${GRADER_PATHS[scope]}/${graderId}/versions`, send(revision)),
    onSuccess: (grader) => {
      onCompleted(grader);
      return client.invalidateQueries({ queryKey: queryKeys.checks.all });
    },
  });
}

export function useCheckReady(value: CheckNode | null): boolean {
  const kinds = useCheckKinds();
  if (!value) return true;
  if (kinds.error || !kinds.data) return false;
  const limits = { maxNodes: kinds.data.max_nodes, maxDepth: kinds.data.max_depth };
  return problems(value, kinds.data.kinds, limits).size === 0;
}
