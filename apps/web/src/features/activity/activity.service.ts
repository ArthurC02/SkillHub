import { useInfiniteQuery } from "@tanstack/react-query";
import { apiFetch, ApiError } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import type { Labelled } from "../../core/api/types";

export type ActivitySource = "run" | "evaluation" | "creation" | "packaging" | "publishing";
export type ActivityClassification = "needs_attention" | "in_progress" | "recent" | "neutral";

export type ActivityContinuation =
  | { kind: "run"; run_id: string }
  | { kind: "creation_session"; session_id: string }
  | { kind: "packaging_artifact"; artifact_id: string }
  | { kind: "skill_publication"; publisher: string; publication_name: string };

export interface ActivityItem {
  kind: string;
  source_id: string;
  summary: string;
  classification: ActivityClassification;
  status: Labelled;
  activity_at: string;
  context?: {
    skill_id?: string;
    skill_name?: string;
    skill_version_id?: string;
    test_case_id?: string;
    artifact_name?: string;
    publisher?: string;
    publication_name?: string;
  };
  continuation: ActivityContinuation;
}

export interface ActivityPage {
  complete: true;
  sources: ActivitySource[];
  items: ActivityItem[];
  next_cursor?: string;
}

export interface ActivityUnavailable {
  complete: false;
  unavailable_sources: ActivitySource[];
  error: string;
}

export function activityUnavailable(error: unknown): ActivityUnavailable | undefined {
  if (!(error instanceof ApiError) || error.status !== 503) return undefined;
  const body = error.body;
  if (typeof body !== "object" || body === null) return undefined;
  const value = body as Partial<ActivityUnavailable>;
  if (value.complete !== false || !Array.isArray(value.unavailable_sources)) return undefined;
  return {
    complete: false,
    unavailable_sources: value.unavailable_sources.filter(isActivitySource),
    error: typeof value.error === "string" ? value.error : "",
  };
}

function isActivitySource(value: unknown): value is ActivitySource {
  return ["run", "evaluation", "creation", "packaging", "publishing"].includes(String(value));
}

async function listActivity(cursor: string): Promise<ActivityPage> {
  const query = new URLSearchParams({ limit: "50" });
  if (cursor) query.set("cursor", cursor);
  return apiFetch<ActivityPage>(`/me/activity?${query}`);
}

export function useActivity() {
  return useInfiniteQuery({
    queryKey: queryKeys.activity,
    initialPageParam: "",
    queryFn: ({ pageParam }) => listActivity(pageParam),
    getNextPageParam: (page) => page.next_cursor,
  });
}
