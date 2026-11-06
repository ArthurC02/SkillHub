import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../../core/api/client";
import { queryKeys } from "../../../core/api/queryKeys";
import { confirmPreflight } from "../lab.service";
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
      const body = await apiFetch<unknown>(base(skillId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(batch),
      });
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
