import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import { useMe } from "../../core/session/me.service";
import { queryKeys } from "../../core/api/queryKeys";

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
  actor_user_id: string | null;
  action: string;
  resource_type: string;
  resource_id: string | null;
  workspace_id: string | null;
  occurred_at: string;
  metadata: Record<string, unknown>;
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

export function useAccountLookup(email: string) {
  return useQuery({
    queryKey: queryKeys.admin.account(email),
    queryFn: () => apiFetch<AccountLookup>(`/admin/accounts?email=${encodeURIComponent(email)}`),
    enabled: useOperator() && email !== "",
  });
}

export function useCreditLedger(workspaceId: string) {
  return useQuery({
    queryKey: queryKeys.admin.ledger(workspaceId),
    queryFn: () => apiFetch<CreditLedger>(`/admin/credits/${workspaceId}`),
    enabled: useOperator(),
  });
}

export function useGrantCredits(workspaceId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { amount_credits: number; reason: string }) =>
      apiFetch<{ balance_credits: number; amount_credits: number }>(
        `/admin/credits/${workspaceId}/grants`,
        send("POST", body),
      ),
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
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ method, body }: { method: "PUT" | "DELETE"; body: Record<string, unknown> }) =>
      apiFetch<unknown>(`/admin/skills/${skillId}/${action}`, send(method, body)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.admin.skills }),
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
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      apiFetch<{ events: OperatorAuditEvent[] }>(
        `/admin/audit-log?limit=${AUDIT_PAGE + 1}&offset=${pageParam}`,
      ).then((page) => ({
        events: page.events.slice(0, AUDIT_PAGE),
        nextOffset: page.events.length > AUDIT_PAGE ? pageParam + AUDIT_PAGE : undefined,
      })),
    getNextPageParam: (last) => last.nextOffset,
    enabled: useOperator(),
  });
}

export function useCostStatistics() {
  return useQuery({
    queryKey: queryKeys.admin.costStatistics,
    queryFn: () => apiFetch<{ statistics: CostStatisticsWindow[] }>("/admin/cost-statistics"),
    enabled: useOperator(),
  });
}

export function usd(micros: number | null): string {
  return micros === null ? "未測量" : `$${(micros / 1_000_000).toFixed(4)}`;
}

export type DailyCount = { day: string; key: string; count: number };
export type DailyAmount = DailyCount & { total: number };
export type Trend<B extends DailyCount = DailyCount> = { from: string; to: string; buckets: B[] };
export type CreditTrend = Trend<DailyAmount> & { balance_total: number };
export type TrendDays = 7 | 30 | 90;
export const TREND_DAYS: TrendDays[] = [7, 30, 90];

export function useTrend<T extends Trend<DailyCount>>(
  path: "cost" | "credits" | "runs" | "operator-actions",
  days: TrendDays,
) {
  return useQuery({
    queryKey: queryKeys.admin.trend(path, days),
    queryFn: () => apiFetch<T>(`/admin/trends/${path}?days=${days}`),
    enabled: useOperator(),
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
