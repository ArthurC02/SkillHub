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
    queryFn: () => apiFetch<{ graders: Grader[] }>(GRADER_PATHS[scope]),
    enabled: scope === "workspace" || operator,
  });
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
