import { apiFetch } from "./client";

export type FeedbackKind = "blocking_issue" | "need_signal";

export const FEEDBACK_MAX_MESSAGE = 2000;

export interface FeedbackReport {
  kind: FeedbackKind;
  message: string;
  page_path?: string;
  run_id?: string;
  build_id?: string;
}

export const BUILD_ID: string | undefined = import.meta.env.VITE_BUILD_ID;

export function submitFeedback(report: FeedbackReport) {
  return apiFetch<void>("/feedback", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(report),
  });
}

export function feedbackPagePath(pathname: string): string {
  return pathname.split(/[?#]/)[0];
}

export function feedbackRunID(pathname: string): string | undefined {
  const match = /^\/runs\/([0-9a-fA-F-]{36})(\/|$)/.exec(feedbackPagePath(pathname));
  return match ? match[1] : undefined;
}
