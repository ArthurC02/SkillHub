import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, jsonInit } from "../../../core/api/client";
import { queryKeys } from "../../../core/api/queryKeys";
import { confirmPreflight, getPreflight } from "../lab.service";
import { createVersionFromSuggestions } from "../../runs";
import {
  isBatchReflection,
  isBatchReflectionList,
  type BatchReflection,
  type UnconfirmedPair,
} from "./reflection.model";
import {
  isBatchComparison,
  isBatchList,
  isBatchReport,
  isEvaluationBatch,
  reportStillMoving,
} from "./batches.model";

const REPORT_REFRESH_MS = 15_000;

export type NewBatch = {
  skill_version_id: string;
  test_case_ids: string[];
  trials: number;
  with_skill_withheld: boolean;
};

const base = (skillId: string) => `/skills/${skillId}/evaluation-batches`;

export function useEvaluationBatches(skillId: string) {
  return useQuery({
    queryKey: queryKeys.evaluationBatches.list(skillId),
    queryFn: async () => {
      const body = await apiFetch<unknown>(base(skillId));
      if (!isBatchList(body, skillId)) throw new Error("Invalid evaluation batch list");
      return body.batches;
    },
    enabled: skillId !== "",
  });
}

export function useEvaluationBatchReport(skillId: string, batchId: string) {
  return useQuery({
    queryKey: queryKeys.evaluationBatches.report(skillId, batchId),
    queryFn: async () => {
      const body = await apiFetch<unknown>(`${base(skillId)}/${batchId}`);
      if (!isBatchReport(body, skillId, batchId))
        throw new Error("Invalid evaluation batch report");
      return body;
    },
    refetchInterval: (query) =>
      query.state.data && reportStillMoving(query.state.data) ? REPORT_REFRESH_MS : false,
    enabled: batchId !== "",
  });
}

export function useEvaluationBatchComparison(skillId: string, batchId: string, against: string) {
  return useQuery({
    queryKey: queryKeys.evaluationBatches.comparison(skillId, batchId, against),
    queryFn: async () => {
      const params = new URLSearchParams({ against });
      const body = await apiFetch<unknown>(`${base(skillId)}/${batchId}/comparison?${params}`);
      if (!isBatchComparison(body, skillId, batchId, against))
        throw new Error("Invalid evaluation batch comparison");
      return body;
    },
    enabled: against !== "",
  });
}

export function useCreateEvaluationBatch(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (batch: NewBatch) => {
      const body = await apiFetch<unknown>(base(skillId), jsonInit("POST", batch));
      if (!isEvaluationBatch(body, skillId)) throw new Error("Invalid evaluation batch");
      return body;
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: queryKeys.evaluationBatches.list(skillId) });
      void client.invalidateQueries({ queryKey: queryKeys.runs.lists });
    },
  });
}

export function useResumeEvaluationBatch(skillId: string, batchId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<void>(`${base(skillId)}/${batchId}/resume`, { method: "POST" }),
    onSettled: () =>
      client.invalidateQueries({ queryKey: queryKeys.evaluationBatches.list(skillId) }),
  });
}

export function useConfirmRunPermissions(skillId: string, versionId: string, testCaseId: string) {
  return useMutation({
    mutationFn: (summaryHash: string) =>
      confirmPreflight(skillId, versionId, testCaseId, summaryHash),
  });
}

export function useBatchReflections(skillId: string, batchId: string) {
  return useQuery({
    queryKey: queryKeys.evaluationBatches.reflections(skillId, batchId),
    queryFn: async () => {
      const body = await apiFetch<unknown>(`${base(skillId)}/${batchId}/reflections`);
      if (!isBatchReflectionList(body, batchId)) throw new Error("Invalid batch reflections");
      return body.reflections;
    },
    enabled: batchId !== "",
  });
}

export function useReflectOnBatch(skillId: string, batchId: string) {
  const client = useQueryClient();
  const key = queryKeys.evaluationBatches.reflections(skillId, batchId);
  return useMutation({
    mutationFn: async (devTestCaseIds: string[]) => {
      const body = await apiFetch<unknown>(
        `${base(skillId)}/${batchId}/reflections`,
        jsonInit("POST", { dev_test_case_ids: devTestCaseIds }),
      );
      if (!isBatchReflection(body, batchId)) throw new Error("Invalid batch reflection");
      return body;
    },
    onSuccess: (created) => {
      client.setQueryData<BatchReflection[]>(key, (held) => [created, ...(held ?? [])]);
      void client.invalidateQueries({ queryKey: key });
      void client.invalidateQueries({ queryKey: queryKeys.credits });
    },
  });
}

export function useApplyReflection(skillId: string, batchId: string) {
  const client = useQueryClient();
  const key = queryKeys.evaluationBatches.reflections(skillId, batchId);
  return useMutation({
    mutationFn: ({
      reflectionId,
      suggestionIds,
    }: {
      reflectionId: string;
      suggestionIds: string[];
    }) => createVersionFromSuggestions(skillId, { reflection_id: reflectionId }, suggestionIds),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: key });
      void client.invalidateQueries({ queryKey: queryKeys.skills.versions(skillId) });
      void client.invalidateQueries({ queryKey: queryKeys.skills.candidateVersions(skillId) });
    },
  });
}

function replaceRound(held: BatchReflection[] | undefined, round: BatchReflection) {
  return (held ?? []).map((r) =>
    r.reflection_id.toLowerCase() === round.reflection_id.toLowerCase() ? round : r,
  );
}

function useRoundAction<T>(
  skillId: string,
  batchId: string,
  action: string,
  body: (input: T) => unknown,
) {
  const client = useQueryClient();
  const key = queryKeys.evaluationBatches.reflections(skillId, batchId);
  return useMutation({
    mutationFn: async ({ reflectionId, input }: { reflectionId: string; input: T }) => {
      const round = await apiFetch<unknown>(
        `${base(skillId)}/${batchId}/reflections/${reflectionId}/${action}`,
        jsonInit("POST", body(input)),
      );
      if (!isBatchReflection(round, batchId)) throw new Error("Invalid batch reflection");
      return round;
    },
    onSuccess: (round) =>
      client.setQueryData<BatchReflection[]>(key, (held) => replaceRound(held, round)),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: key });
      void client.invalidateQueries({ queryKey: queryKeys.skills.versions(skillId) });
      void client.invalidateQueries({ queryKey: queryKeys.credits });
    },
  });
}

export function useEvaluateCandidates(skillId: string, batchId: string) {
  return useRoundAction<void>(skillId, batchId, "evaluate", () => ({}));
}

export function useAdoptCandidate(skillId: string, batchId: string) {
  return useRoundAction<number>(skillId, batchId, "adopt", (candidate) => ({ candidate }));
}

export function usePairPreflights(skillId: string, pairs: UnconfirmedPair[]) {
  return useQueries({
    queries: pairs.map((p) => ({
      queryKey: queryKeys.lab.preflight(skillId, p.skill_version_id, p.test_case_id),
      queryFn: () => getPreflight(skillId, p.skill_version_id, p.test_case_id),
      staleTime: 0,
      gcTime: 0,
    })),
  });
}

export function useConfirmEveryPair(skillId: string) {
  return useMutation({
    mutationFn: async (shown: { pair: UnconfirmedPair; summaryHash: string }[]) => {
      for (const { pair, summaryHash } of shown) {
        await confirmPreflight(skillId, pair.skill_version_id, pair.test_case_id, summaryHash);
      }
    },
  });
}
