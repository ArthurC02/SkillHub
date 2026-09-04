import { apiFetch } from "./client";

/**
 * POST /feedback (BETA-003/004/005), exactly as contracts/openapi/public.yaml
 * declares it.
 *
 * Two contract rules are enforced on this side rather than discovered from a
 * 400, because both would lose the report:
 *
 * 1. **`page_path` is a path, never a URL with a query string.** A search query
 *    can carry personal data and this channel is not where it belongs
 *    (beta-design §4.2 界線 2). The server drops the query string rather than
 *    refusing; dropping it here means what the form shows is what is sent.
 * 2. **`message` is the user's own words and must not be blank.** Everything
 *    else travelling with it is a field they can see on screen — no screenshot,
 *    no console, no automatic context grab. `build_id` (資訊架構 IA-11) passes
 *    that test the same way `page_path` does: it is the footer's own 「Build
 *    識別碼」, on every page, and it names the software rather than the person.
 */

export type FeedbackKind = "blocking_issue" | "need_signal";

/** public.yaml's `maxLength`. Same number in one place, shown as a counter. */
export const FEEDBACK_MAX_MESSAGE = 2000;

export interface FeedbackReport {
  kind: FeedbackKind;
  message: string;
  page_path?: string;
  run_id?: string;
  build_id?: string;
}

/** The build this page was served from — the same value the footer prints. */
export const BUILD_ID: string | undefined = import.meta.env.VITE_BUILD_ID;

/** 204 and no body: there is no read surface for feedback and no id to hand back. */
export function submitFeedback(report: FeedbackReport) {
  return apiFetch<void>("/feedback", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(report),
  });
}

/** Route path only, query string and fragment removed (contract rule 1 above). */
export function feedbackPagePath(pathname: string): string {
  return pathname.split(/[?#]/)[0];
}

/**
 * The run the reporter is looking at, when the address names one. Read from the
 * path rather than from page state so the entry point stays a single component
 * in the layout instead of a prop threaded through every screen; a value that is
 * not the caller's own run is dropped by the server rather than failing the
 * report.
 */
export function feedbackRunID(pathname: string): string | undefined {
  // Strips the query string first rather than trusting the caller to: both
  // helpers take the same raw location, and a run id that only resolves when
  // the address happens to carry no `?` is the kind of thing that works in every
  // test and drops the context on the one page that has a tab parameter.
  const match = /^\/runs\/([0-9a-fA-F-]{36})(\/|$)/.exec(feedbackPagePath(pathname));
  return match ? match[1] : undefined;
}
