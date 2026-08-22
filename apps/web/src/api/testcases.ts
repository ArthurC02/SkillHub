import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Dataset } from "./lab";
import type { OwnSkills } from "./types";

/**
 * 03:TEST-012 — the consumer side of the Test Case and acceptance-criteria
 * endpoints. Every route below already exists in
 * contracts/openapi/public.yaml; nothing here adds or changes a contract.
 *
 * Two facts shape the calls:
 *
 * 1. **The draft is editable, the run is not.** A run freezes the prompt and
 *    the criteria into a snapshot, so editing here never rewrites what a past
 *    run executed or what an evaluation judged (iron rule 4, ADR-003).
 * 2. **Editing text and confirming are one statement.** The server clears a
 *    confirmation when the wording changes, because the agreement was to the
 *    old words — so the client sends `text` and `confirmed` on the same PATCH
 *    rather than trying to preserve a confirmation across an edit.
 */

export type AcceptanceCriterion = {
  id: string;
  text: string;
  /** `suggested` is a model's proposal, never presented as the user's own. */
  source: "user" | "suggested";
  confirmed_at: string | null;
};

/**
 * CONTENT-007's rubric. `id` is the acceptance criterion this item strengthens,
 * not an id of its own: the judge answers one verdict per criterion id and the
 * platform drops any id it did not send, so an item pointing anywhere else could
 * never produce a stored verdict.
 */
export type RubricItem = {
  id: string;
  text: string;
  /** The author's relative-importance signal for the model. Not a score: nothing computes with it. */
  weight?: number;
  evidence_required: boolean;
};

export type Rubric = { version: string; items: RubricItem[] };

export type TestCase = {
  test_case_id: string;
  skill_id: string;
  name: string;
  user_prompt: string;
  acceptance_criteria: AcceptanceCriterion[];
  /** Absent means no rubric — never "the default rubric". */
  rubric?: Rubric;
  created_at: string;
  updated_at: string;
};

/**
 * One row of GET /test-cases. The three aggregates are served rather than
 * reduced here so a page of fifty rows is not fifty client-side counts, and
 * `skill_name` spares the list a bare UUID. An empty `skill_name` is "we cannot
 * name it" — the skill left the caller's view — never a name.
 */
export type TestCaseListItem = TestCase & {
  skill_name: string;
  criteria_confirmed: number;
  criteria_total: number;
  has_rubric: boolean;
};

/*
 * There used to be a `SkillSummary` here — skill_id / name / summary — and
 * because GET /skills has exactly one caller through this hook, that narrower
 * type was the app's whole view of the endpoint. `forked_from_skill_id` and
 * `forked_from_version_id` were in the contract and on the wire the entire time,
 * and the page that promises to tell a fork from an import could not see them.
 * Two shapes for one endpoint is how a gap hides (04 丙-31).
 */

/** `skillId` narrows the list to one skill; it is a filter, never a widening (WS-006). */
export function useTestCases(skillId?: string) {
  return useInfiniteQuery({
    // Under the ["test-cases"] prefix so every existing invalidation still
    // reaches it, and keyed by the filter so two filters are two caches.
    queryKey: ["test-cases", "list", skillId ?? ""],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: "51", offset: String(pageParam) });
      if (skillId) params.set("skill_id", skillId);
      return apiFetch<{ test_cases: TestCaseListItem[] }>(`/test-cases?${params}`).then((page) => ({
        test_cases: page.test_cases.slice(0, 50),
        nextOffset: page.test_cases.length > 50 ? pageParam + 50 : undefined,
      }));
    },
    getNextPageParam: (last) => last.nextOffset,
    retry: false,
  });
}

export function useTestCase(testCaseId: string) {
  return useQuery({
    queryKey: ["test-cases", testCaseId],
    queryFn: () => apiFetch<TestCase>(`/test-cases/${testCaseId}`),
    enabled: testCaseId.length > 0,
    retry: false,
  });
}

export function useOwnSkills() {
  return useQuery({
    queryKey: ["own-skills"],
    queryFn: () => apiFetch<OwnSkills>("/skills"),
    retry: false,
  });
}

export function createTestCase(skillId: string, name: string, userPrompt: string) {
  return apiFetch<TestCase>("/test-cases", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ skill_id: skillId, name, user_prompt: userPrompt }),
  });
}

/**
 * `rubric` is three-valued and each value says something different: leaving the
 * key out keeps the stored rubric, an object replaces it, and an explicit `null`
 * removes it. There is no empty rubric.
 */
export function updateTestCase(
  testCaseId: string,
  patch: { name?: string; user_prompt?: string; rubric?: Rubric | null },
) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
}

/**
 * The one write path for a criterion, whether the user typed it or adopted a
 * proposal. `source: "suggested"` labels text taken verbatim from a suggestion
 * so a model's wording is never reported as the user's own judgement; either
 * way it arrives unconfirmed, because adopting a wording is not agreeing to it.
 */
export function addCriterion(
  testCaseId: string,
  text: string,
  source: "user" | "suggested" = "user",
) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text, source }),
  });
}

export function updateCriterion(
  testCaseId: string,
  criterionId: string,
  patch: { text?: string; confirmed?: boolean },
) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria/${criterionId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
}

export function deleteCriterion(testCaseId: string, criterionId: string) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria/${criterionId}`, {
    method: "DELETE",
  });
}

/**
 * TEST-002's optional enhancement. 503 means unavailable, not a failed request.
 *
 * **Proposals, not criteria.** The call stores nothing: a proposal has no id,
 * no source and no confirmation, because those are facts about a criterion the
 * user has adopted. Adopting one is `addCriterion(..., "suggested")`, one at a
 * time — the route that wrote first and left the user deleting what it had
 * decided for them was the opposite of TEST-001's "確認權在使用者".
 */
export function suggestCriteria(testCaseId: string) {
  return apiFetch<{ suggestions: { text: string }[] }>(
    `/test-cases/${testCaseId}/criteria/suggest`,
    { method: "POST" },
  );
}

/**
 * WS-002 delete. `datasets_deleted` is the deletion's actual scope, which the
 * caller states back to the user: what went is the draft and its live files,
 * what stays is every past run's snapshot (ADR-003).
 */
export function deleteTestCase(testCaseId: string) {
  return apiFetch<{ deleted: boolean; datasets_deleted: number; note: string }>(
    `/test-cases/${testCaseId}`,
    { method: "DELETE" },
  );
}

export function useTestCaseDatasets(testCaseId: string) {
  return useQuery({
    queryKey: ["test-cases", testCaseId, "datasets"],
    queryFn: () =>
      apiFetch<{ datasets: Dataset[]; total_bytes: number }>(`/test-cases/${testCaseId}/datasets`),
    enabled: testCaseId.length > 0,
    retry: false,
  });
}

export function deleteDataset(testCaseId: string, datasetId: string) {
  return apiFetch<{ deleted: boolean; dataset_id: string; note: string }>(
    `/test-cases/${testCaseId}/datasets/${datasetId}`,
    { method: "DELETE" },
  );
}
