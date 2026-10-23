import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiFetch, isLastingReadFailure } from "../../core/api/client";
import { useMe } from "../../core/session/me.service";
import { queryKeys } from "../../core/api/queryKeys";

export const AGENT_RUN_POLL_MS = 3000;

export type AccountLookup = {
  user_id: string;
  email: string;
  display_name: string;
  workspace_id: string;
  created_at: string;
  deletion_requested_at: string | null;
  in_beta_allowlist: boolean;
};

export type CreditLedgerEntry = {
  kind: "debit" | "grant" | "topup" | "adjustment";
  delta_credits: number;
  ref_type: string | null;
  estimated: boolean;
  created_at: string;
};

export type CreditLedger = {
  workspace_id: string;
  balance_credits: number;
  entries: CreditLedgerEntry[];
};

export type SkillGovernance = {
  skill_id: string;
  workspace_id: string;
  name: string;
  access_restriction: string | null;
  redistribution: string;
  takedown_at: string | null;
  takedown_reason: string | null;
  deleted_at: string | null;
};

export type DispatchHalt = {
  target: string;
  source: "p1_incident" | "orphan_threshold";
  reason: string;
  declared_at: string;
  clear_rounds?: number;
  automatic_recovery: boolean;
};

export type DispatchStatus = { dispatching: boolean; halts: DispatchHalt[] };

export type ModelCallBudget = {
  kind: string;
  seconds: number | null;
  default_seconds: number;
  min_seconds: number;
  max_seconds: number;
  reason: string | null;
  set_at: string | null;
};

export type Rosters = { operator_user_ids: string[]; beta_allowlist: string[] };

export type OperatorAuditEvent = {
  actor_kind: "person" | "agent" | "system";
  actor_user_id: string | null;
  actor_agent_id: string | null;
  action: string;
  resource_type: string;
  resource_id: string | null;
  workspace_id: string | null;
  occurred_at: string;
  metadata: Record<string, unknown>;
};

export type ExposureRelease = {
  release_id: string;
  version_id: string;
  version_number: number;
  content_hash: string;
  released_at: string;
};

export type ExposureQueueEntry = {
  publisher: string;
  name: string;
  address: string;
  release: ExposureRelease;
  sequence: number;
  reviewed_again: boolean;
};

export type ExposureSnapshot = {
  version_id: string;
  current: boolean;
  name: string;
  summary: string;
  enriched_summary: string;
  task_examples: string;
  tags: unknown;
  limitations: string;
  enriched: boolean;
  digest: string;
};

export type ExposureDecision = "approved" | "revoked";

export type ExposureReviewRecord = {
  sequence: number;
  release_id: string;
  content_hash: string;
  snapshot_digest: string;
  decision: ExposureDecision;
  reason: string;
  reviewer_user_id: string;
  reviewed_at: string;
};

export type ExposureCase = {
  publisher: string;
  name: string;
  address: string;
  status: "published" | "delisted";
  release: ExposureRelease;
  sequence: number;
  exposed: boolean;
  approval: {
    allowed: boolean;
    refusal?: { error: string; reason: string };
  };
  snapshot?: ExposureSnapshot;
  history: ExposureReviewRecord[];
};

export type CostStatisticsWindow = {
  kind: string;
  window_start: string;
  window_end: string;
  sample_count: number;
  p50_usd_micros: number | null;
  p90_usd_micros: number | null;
  p95_usd_micros: number | null;
  max_usd_micros: number | null;
};

function useOperator(): boolean {
  return useMe().data?.operator === true;
}

function send(method: string, body: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export async function lookupAccount(email: string): Promise<AccountLookup> {
  const result = await apiFetch<unknown>(`/admin/accounts?email=${encodeURIComponent(email)}`);
  if (!isAccountLookup(result)) throw new Error("Invalid account lookup response");
  return result;
}

function isAccountLookup(value: unknown): value is AccountLookup {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const account = value as Partial<AccountLookup>;
  return (
    [account.user_id, account.workspace_id].every(
      (id) =>
        typeof id === "string" &&
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id),
    ) &&
    typeof account.email === "string" &&
    typeof account.display_name === "string" &&
    typeof account.in_beta_allowlist === "boolean" &&
    typeof account.created_at === "string" &&
    Number.isFinite(Date.parse(account.created_at)) &&
    (account.deletion_requested_at === null ||
      (typeof account.deletion_requested_at === "string" &&
        Number.isFinite(Date.parse(account.deletion_requested_at))))
  );
}

export function useAccountLookup(email: string) {
  return useQuery({
    queryKey: queryKeys.admin.account(email),
    queryFn: () => lookupAccount(email),
    enabled: useOperator() && email !== "",
  });
}

export async function getCreditLedger(workspaceId: string): Promise<CreditLedger> {
  const result = await apiFetch<unknown>(`/admin/credits/${encodeURIComponent(workspaceId)}`);
  if (!isCreditLedger(result, workspaceId)) throw new Error("Invalid credit ledger response");
  return result;
}

function isCreditLedger(value: unknown, workspaceId: string): value is CreditLedger {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const ledger = value as Partial<CreditLedger>;
  return (
    typeof ledger.workspace_id === "string" &&
    ledger.workspace_id.toLowerCase() === workspaceId.toLowerCase() &&
    Number.isSafeInteger(ledger.balance_credits) &&
    Array.isArray(ledger.entries) &&
    ledger.entries.every(isCreditLedgerEntry)
  );
}

function isCreditLedgerEntry(value: unknown): value is CreditLedgerEntry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const entry = value as Partial<CreditLedgerEntry>;
  return (
    ["debit", "grant", "topup", "adjustment"].includes(entry.kind ?? "") &&
    Number.isSafeInteger(entry.delta_credits) &&
    (entry.ref_type === null || typeof entry.ref_type === "string") &&
    typeof entry.estimated === "boolean" &&
    typeof entry.created_at === "string" &&
    Number.isFinite(Date.parse(entry.created_at))
  );
}

export function useCreditLedger(workspaceId: string) {
  return useQuery({
    queryKey: queryKeys.admin.ledger(workspaceId),
    queryFn: () => getCreditLedger(workspaceId),
    enabled: useOperator(),
  });
}

type CreditGrantResult = {
  workspace_id: string;
  balance_credits: number;
  amount_credits: number;
};

export async function grantCredits(
  workspaceId: string,
  body: { amount_credits: number; reason: string; idempotency_key: string },
): Promise<CreditGrantResult> {
  const result = await apiFetch<unknown>(
    `/admin/credits/${encodeURIComponent(workspaceId)}/grants`,
    send("POST", body),
  );
  if (!isCreditGrantResult(result, workspaceId, body.amount_credits))
    throw new Error("Invalid credit grant response");
  return result;
}

function isCreditGrantResult(
  value: unknown,
  workspaceId: string,
  amount: number,
): value is CreditGrantResult {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const result = value as Partial<CreditGrantResult>;
  return (
    typeof result.workspace_id === "string" &&
    result.workspace_id.toLowerCase() === workspaceId.toLowerCase() &&
    Number.isSafeInteger(result.balance_credits) &&
    Number.isSafeInteger(result.amount_credits) &&
    result.amount_credits === amount
  );
}

export function useGrantCredits(workspaceId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { amount_credits: number; reason: string; idempotency_key: string }) =>
      grantCredits(workspaceId, body),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: queryKeys.admin.ledger(workspaceId) }),
  });
}

export function useGovernance(q: string) {
  return useQuery({
    queryKey: queryKeys.admin.skillSearch(q),
    queryFn: () =>
      apiFetch<{ skills: SkillGovernance[] }>(`/admin/skills?q=${encodeURIComponent(q)}`),
    enabled: useOperator() && q !== "",
  });
}

export function useGovernanceAction(
  skillId: string,
  action: "restriction" | "redistribution" | "takedown",
  onComplete?: (submitted: { method: "PUT" | "DELETE"; body: Record<string, unknown> }) => void,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ method, body }: { method: "PUT" | "DELETE"; body: Record<string, unknown> }) =>
      apiFetch<unknown>(`/admin/skills/${skillId}/${action}`, send(method, body)),
    onSuccess: (_result, submitted) => {
      onComplete?.(submitted);
      return queryClient.invalidateQueries({ queryKey: queryKeys.admin.skills });
    },
  });
}

export function useDispatchStatus() {
  return useQuery({
    queryKey: queryKeys.admin.dispatch,
    queryFn: () => apiFetch<DispatchStatus>("/admin/dispatch"),
    enabled: useOperator(),
  });
}

export function useDispatchHalt(method: "PUT" | "DELETE") {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { note: string; provider?: string }) =>
      apiFetch<{ note?: string } | undefined>("/admin/dispatch/halt", send(method, body)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.dispatch }),
  });
}

export function useModelBudgets() {
  return useQuery({
    queryKey: queryKeys.admin.modelBudgets,
    queryFn: () => apiFetch<{ budgets: ModelCallBudget[] }>("/admin/model-budgets"),
    enabled: useOperator(),
  });
}

export function useModelBudgetChange(method: "PUT" | "DELETE") {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ kind, ...body }: { kind: string; reason: string; seconds?: number }) =>
      apiFetch<ModelCallBudget | undefined>(
        `/admin/model-budgets/${encodeURIComponent(kind)}`,
        send(method, body),
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.modelBudgets }),
  });
}

export function useRosters() {
  return useQuery({
    queryKey: queryKeys.admin.rosters,
    queryFn: () => apiFetch<Rosters>("/admin/rosters"),
    enabled: useOperator(),
  });
}

const AUDIT_PAGE = 50;

export function useOperatorAuditLog() {
  return useInfiniteQuery({
    queryKey: queryKeys.admin.auditLog,
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      apiFetch<{ events: OperatorAuditEvent[]; next_cursor?: string }>(
        `/admin/audit-log?limit=${AUDIT_PAGE}${pageParam ? `&cursor=${encodeURIComponent(pageParam)}` : ""}`,
      ),
    getNextPageParam: (last) => last.next_cursor,
    enabled: useOperator(),
  });
}

export function useExposureQueue() {
  return useQuery({
    queryKey: queryKeys.admin.exposureQueue,
    queryFn: () => apiFetch<{ publications: ExposureQueueEntry[] }>("/admin/exposure-reviews"),
    enabled: useOperator(),
  });
}

export function useExposureCase(publication: string) {
  return useQuery({
    queryKey: queryKeys.admin.exposureCase(publication),
    queryFn: () => apiFetch<ExposureCase>(`/admin/publications/${publication}/exposure`),
    enabled: useOperator() && publication !== "",
  });
}

export function useReviewExposure(publication: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      release_id: string;
      expected_sequence: number;
      expected_snapshot_digest: string;
      decision: ExposureDecision;
      reason: string;
    }) => apiFetch<ExposureCase>(`/admin/publications/${publication}/exposure`, send("POST", body)),
    onSuccess: (currentCase) => {
      queryClient.setQueryData(queryKeys.admin.exposureCase(publication), currentCase);
      queryClient.invalidateQueries({ queryKey: queryKeys.admin.exposureQueue });
      queryClient.invalidateQueries({ queryKey: queryKeys.admin.exposureCase(publication) });
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 409) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.admin.exposureCase(publication) });
      }
    },
  });
}

export async function getCostStatistics(): Promise<{ statistics: CostStatisticsWindow[] }> {
  const result = await apiFetch<unknown>("/admin/cost-statistics");
  if (!isCostStatistics(result)) throw new Error("Invalid cost response");
  return result;
}

function isCostStatistics(value: unknown): value is { statistics: CostStatisticsWindow[] } {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const statistics = (value as { statistics?: unknown }).statistics;
  return Array.isArray(statistics) && statistics.every(isCostStatisticsWindow);
}

function isCostStatisticsWindow(value: unknown): value is CostStatisticsWindow {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const statistic = value as Partial<CostStatisticsWindow>;
  return (
    typeof statistic.kind === "string" &&
    [statistic.window_start, statistic.window_end].every(
      (date) => typeof date === "string" && Number.isFinite(Date.parse(date)),
    ) &&
    Number.isSafeInteger(statistic.sample_count) &&
    [
      statistic.p50_usd_micros,
      statistic.p90_usd_micros,
      statistic.p95_usd_micros,
      statistic.max_usd_micros,
    ].every((amount) => amount === null || Number.isSafeInteger(amount))
  );
}

export function useCostStatistics() {
  return useQuery({
    queryKey: queryKeys.admin.costStatistics,
    queryFn: getCostStatistics,
    enabled: useOperator(),
  });
}

const usdFormatter = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 4,
  maximumFractionDigits: 6,
});

export function usd(micros: number | null): string {
  return micros === null ? "未測量" : usdFormatter.format(micros / 1_000_000);
}

export type PlatformAgent = {
  name: string;
  purpose: string;
  model_role: string;
  daily_spend_cap_usd_micros: number;
  default_daily_spend_cap_usd_micros: number;
  daily_spend_cap_overridden: boolean;
  tools: string[];
  actions: string[];
  enabled: boolean;
  owner_user_id?: string;
};

export type PlatformAgentBrake = {
  reason: string;
  engaged_at: string;
  engaged_by_user_id?: string;
};

export type PlatformAgentRunStatus = "running" | "completed" | "incomplete" | "stopped" | "failed";

export type PlatformAgentRun = {
  id: string;
  agent: string;
  status: PlatformAgentRunStatus;
  reason?: string;
  started_at: string;
  finished_at?: string;
  last_step_at?: string;
  result?: Record<string, unknown>;
  steps: number;
  usd_micros: number;
  unpriced_steps: number;
};

export type PlatformAgentStep = {
  seq: number;
  tool: string;
  arguments: string;
  result: string;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  usd_micros?: number;
  created_at: string;
};

export function usePlatformAgents() {
  return useQuery({
    queryKey: queryKeys.admin.agents,
    queryFn: () =>
      apiFetch<{ agents: PlatformAgent[]; brake?: PlatformAgentBrake }>("/admin/agents"),
    enabled: useOperator(),
  });
}

export function usePlatformAgentSwitch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, ...body }: { name: string; enabled: boolean; note: string }) =>
      apiFetch<PlatformAgent>(
        `/admin/agents/${encodeURIComponent(name)}/enabled`,
        send("PUT", body),
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.agents }),
  });
}

export const MAX_AGENT_SPEND_CAP_MICROS = 5_000_000;

export function usePlatformAgentSpendCap() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      name,
      ...body
    }: {
      name: string;
      daily_spend_cap_usd_micros: number | null;
      note: string;
    }) =>
      apiFetch<PlatformAgent>(
        `/admin/agents/${encodeURIComponent(name)}/spend-cap`,
        send("PUT", body),
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.agents }),
  });
}

export type PlatformSettings = {
  judge_panel: { enabled: boolean; reason?: string; set_by_user_id?: string; set_at?: string };
};

export function usePlatformSettings() {
  return useQuery({
    queryKey: queryKeys.admin.settings,
    queryFn: () => apiFetch<PlatformSettings>("/admin/settings"),
    enabled: useOperator(),
  });
}

export function useJudgePanelSwitch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { enabled: boolean; note: string }) =>
      apiFetch<PlatformSettings>("/admin/settings/judge-panel", send("PUT", body)),
    onSuccess: (settings) => queryClient.setQueryData(queryKeys.admin.settings, settings),
  });
}

export type RunSettings = {
  continuation_rounds: {
    rounds: number;
    min: number;
    max: number;
    reason?: string;
    set_by_user_id?: string;
    set_at?: string;
  };
  tokens_per_minute: {
    limit: number;
    default_limit: number;
    min: number;
    max: number;
    reason?: string;
    set_by_user_id?: string;
    set_at?: string;
  };
  model_budget: {
    credits: number;
    default_credits: number;
    min_credits: number;
    max_credits: number;
    reason?: string;
    set_by_user_id?: string;
    set_at?: string;
  };
};

export function useRunSettings() {
  return useQuery({
    queryKey: queryKeys.admin.runSettings,
    queryFn: () => apiFetch<RunSettings>("/admin/settings/runs"),
    enabled: useOperator(),
  });
}

export function useContinuationRounds() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { rounds: number; note: string }) =>
      apiFetch<RunSettings>("/admin/settings/runs/continuation-rounds", send("PUT", body)),
    onSuccess: (settings) => queryClient.setQueryData(queryKeys.admin.runSettings, settings),
  });
}

export function useRunModelBudget() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { credits?: number; note: string }) =>
      apiFetch<RunSettings>(
        "/admin/settings/runs/model-budget",
        send(body.credits === undefined ? "DELETE" : "PUT", body),
      ),
    onSuccess: (settings) => queryClient.setQueryData(queryKeys.admin.runSettings, settings),
  });
}

export function useRunTokensPerMinute() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { limit?: number; note: string }) =>
      apiFetch<RunSettings>(
        "/admin/settings/runs/tokens-per-minute",
        send(body.limit === undefined ? "DELETE" : "PUT", body),
      ),
    onSuccess: (settings) => queryClient.setQueryData(queryKeys.admin.runSettings, settings),
  });
}

export function usePlatformAgentBrake(method: "PUT" | "DELETE") {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { note: string }) =>
      apiFetch<PlatformAgentBrake | undefined>("/admin/agents/brake", send(method, body)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.agents }),
  });
}

export function usePlatformAgentRuns() {
  return useQuery({
    queryKey: queryKeys.admin.agentRuns,
    queryFn: () => apiFetch<{ runs: PlatformAgentRun[]; total: number }>("/admin/agents/runs"),
    enabled: useOperator(),
    refetchInterval: (query) =>
      !isLastingReadFailure(query.state.error) &&
      query.state.data?.runs.some((run) => run.status === "running")
        ? AGENT_RUN_POLL_MS
        : false,
  });
}

export function usePlatformAgentRun(id: string) {
  return useQuery({
    queryKey: queryKeys.admin.agentRun(id),
    queryFn: () => apiFetch<PlatformAgentRun>(`/admin/agents/runs/${encodeURIComponent(id)}`),
    enabled: useOperator(),
    refetchInterval: (query) =>
      !isLastingReadFailure(query.state.error) && query.state.data?.status === "running"
        ? AGENT_RUN_POLL_MS
        : false,
  });
}

export type FindingStatus = "open" | "acknowledged" | "resolved" | "dismissed" | "recovered";

export type PlatformAgentFinding = {
  id: string;
  agent: string;
  status: FindingStatus;
  title: string;
  cites: string[];
  assignee_user_id?: string;
  first_seen_at: string;
  last_seen_at: string;
  seen_count: number;
  status_changed_at: string;
};

export type PlatformAgentFindingEvent = {
  seq: number;
  kind: "opened" | "seen" | "reopened" | "recovered" | "acknowledged" | "resolved" | "dismissed";
  run_id?: string;
  operator_user_id?: string;
  text?: string;
  evidence?: Record<string, unknown>;
  note?: string;
  occurred_at: string;
};

export function usePlatformAgentFindings(status?: FindingStatus) {
  return useInfiniteQuery({
    queryKey: queryKeys.admin.agentFindingList(status ?? "live"),
    initialPageParam: "",
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams();
      if (status) params.set("status", status);
      if (pageParam) params.set("cursor", pageParam);
      const query = params.toString();
      return apiFetch<{
        findings: PlatformAgentFinding[];
        counts: Record<FindingStatus, number>;
        next_cursor?: string;
      }>(`/admin/agents/findings${query ? `?${query}` : ""}`);
    },
    getNextPageParam: (last) => last.next_cursor,
    enabled: useOperator(),
  });
}

export function canKeepLoadedFindings(error: Error | null, isFetchNextPageError: boolean) {
  return (
    isFetchNextPageError &&
    !(error instanceof ApiError && error.status >= 400 && error.status < 500)
  );
}

export function usePlatformAgentFinding(id: string) {
  return useQuery({
    queryKey: queryKeys.admin.agentFinding(id),
    queryFn: () =>
      apiFetch<{ finding: PlatformAgentFinding; events: PlatformAgentFindingEvent[] }>(
        `/admin/agents/findings/${encodeURIComponent(id)}`,
      ),
    enabled: useOperator(),
  });
}

export function useMoveFinding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; status: FindingStatus; note: string }) =>
      apiFetch<undefined>(
        `/admin/agents/findings/${encodeURIComponent(id)}/status`,
        send("PUT", body),
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.agentFindings }),
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.admin.agentFindings });
    },
  });
}

export type ProposalStatus =
  "proposed" | "approved" | "rejected" | "expired" | "running" | "succeeded" | "failed";

export type PlatformAgentProposal = {
  id: string;
  agent: string;
  action: string;
  tier: "read_only" | "reversible" | "destructive";
  reason: string;
  status: ProposalStatus;
  proposed_at: string;
  expires_at: string;
  finished_at?: string;
};

export type PlatformAgentProposalDetail = PlatformAgentProposal & {
  run_id: string;
  cites: string[];
  preview: {
    counts: { key: string; count: number; at_most: boolean }[];
    batch_limit?: number;
  };
  decided_by_user_id?: string;
  decided_at?: string;
  decision_note?: string;
  started_at?: string;
  outcome?: string;
};

export type ProposalQueueView = "proposed" | "processing" | "closed";

export function usePlatformAgentProposals(view: ProposalQueueView = "proposed", offset = 0) {
  return useQuery({
    queryKey: queryKeys.admin.agentProposalPage(view, offset),
    queryFn: () =>
      apiFetch<{ proposals: PlatformAgentProposal[]; total: number }>(
        `/admin/agents/proposals?view=${view}&offset=${offset}`,
      ).then((page) => {
        if (
          !Array.isArray(page.proposals) ||
          !Number.isSafeInteger(page.total) ||
          page.total < 0 ||
          (page.proposals.length > 0 && offset + page.proposals.length > page.total)
        ) {
          throw new Error("proposal queue response is incomplete");
        }
        return page;
      }),
    enabled: useOperator(),
  });
}

export function usePlatformAgentProposal(id: string) {
  return useQuery({
    queryKey: queryKeys.admin.agentProposal(id),
    queryFn: () =>
      apiFetch<PlatformAgentProposalDetail>(`/admin/agents/proposals/${encodeURIComponent(id)}`),
    enabled: useOperator(),
  });
}

export function useDecideProposal() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; decision: "approve" | "reject"; note: string }) =>
      apiFetch<undefined>(
        `/admin/agents/proposals/${encodeURIComponent(id)}/decision`,
        send("PUT", body),
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.agentProposals }),
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.admin.agentProposals });
    },
  });
}

export function usePlatformAgentSteps(run: string, live = false) {
  return useQuery({
    queryKey: queryKeys.admin.agentSteps(run),
    queryFn: () =>
      apiFetch<{ steps: PlatformAgentStep[] }>(
        `/admin/agents/runs/${encodeURIComponent(run)}/steps`,
      ),
    enabled: useOperator(),
    refetchInterval: (query) =>
      live && !isLastingReadFailure(query.state.error) ? AGENT_RUN_POLL_MS : false,
  });
}

export type DailyCount = { day: string; key: string; count: number };
export type DailyAmount = DailyCount & { total: number };
export type Trend<B extends DailyCount = DailyCount> = { from: string; to: string; buckets: B[] };
export type CreditTrend = Trend<DailyAmount> & { balance_total: number };
export type FunnelStage = { key: string; label: string; grain: string };
export type FunnelTrend = Trend & { stages: FunnelStage[] };
export type TrendDays = 7 | 30 | 90;
export const TREND_DAYS: TrendDays[] = [7, 30, 90];

export function useTrend<T extends Trend<DailyCount>>(
  path: "cost" | "credits" | "runs" | "operator-actions" | "funnel",
  days: TrendDays,
) {
  return useQuery({
    queryKey: queryKeys.admin.trend(path, days),
    queryFn: () => apiFetch<T>(`/admin/trends/${path}?days=${days}`),
    enabled: useOperator(),
    refetchOnWindowFocus: false,
  });
}

export function daysOf(from: string, to: string): string[] {
  const days: string[] = [];
  const end = Date.parse(`${to}T00:00:00Z`);
  for (let at = Date.parse(`${from}T00:00:00Z`); at <= end; at += 86_400_000) {
    days.push(new Date(at).toISOString().slice(0, 10));
  }
  return days;
}

export type TrendSeries = { key: string; counts: number[]; values: number[] };

export function seriesOf<B extends DailyCount>(trend: Trend<B>, value: (bucket: B) => number) {
  const days = daysOf(trend.from, trend.to);
  const at = new Map(days.map((day, index) => [day, index]));
  const series = new Map<string, TrendSeries>();
  for (const bucket of trend.buckets) {
    const index = at.get(bucket.day);
    if (index === undefined) continue;
    const row = series.get(bucket.key) ?? {
      key: bucket.key,
      counts: days.map(() => 0),
      values: days.map(() => 0),
    };
    row.counts[index] += bucket.count;
    row.values[index] += value(bucket);
    series.set(bucket.key, row);
  }
  return { days, series: [...series.values()].sort((a, b) => a.key.localeCompare(b.key)) };
}
