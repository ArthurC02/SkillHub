import { act } from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ME, RUN, SUGGESTIONS } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const REFUSAL_SAID: Record<string, string> = {
  "rejected input": "這個決定被拒絕。請重新讀取目前判定後再試。",
  "missing suggestion": "這個決定被拒絕。這個建議已經不在了，請重新讀取這份評估。",
  "applied conflict":
    "這個決定被拒絕。這個建議的狀態已經變了，可能已在別處做了決定或建成新版本；請重新讀取目前判定後再試。",
};

function truncateFirstDecisionReply() {
  const real = globalThis.fetch;
  let truncated = false;
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    const response = await real(input, init);
    if (truncated || !String(input).includes("/suggestions/s1/decision")) return response;
    truncated = true;
    return new Response("{", { status: 200, headers: { "Content-Type": "application/json" } });
  });
}

function receiveDecision(page: App, fault: string, uncertain: boolean) {
  const writes: { decision: "accepted" | "rejected" }[] = [];
  const initialDecision = ["lost reply", "rejected input"].includes(fault) ? "accepted" : "pending";
  let saved = {
    ...SUGGESTIONS.suggestions[0],
    decision: initialDecision,
  };
  let releaseResponse!: () => void;
  const responseGate = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  let submissions = 0;
  if (fault === "truncated reply") truncateFirstDecisionReply();
  page.route("**/suggestions/s1/decision", async (request) => {
    submissions++;
    const intent = request.body as { decision: "accepted" | "rejected" };
    writes.push(intent);
    if (writes.length === 1) await responseGate;
    if (uncertain || writes.length > 1) saved = { ...saved, ...intent };
    if (writes.length > 1) return { json: saved };
    const receipts: Record<string, unknown> = {
      "null receipt": null,
      "incomplete receipt": {},
      "opposite choice": {
        ...saved,
        decision: intent.decision === "accepted" ? "rejected" : "accepted",
      },
      "another suggestion": { ...saved, suggestion_id: "s2" },
    };
    if (fault === "lost reply") return "network-error";
    if (Object.hasOwn(receipts, fault)) return { json: receipts[fault] };
    if (fault === "truncated reply") return { json: saved };
    const status =
      fault === "server error"
        ? 503
        : fault === "rejected input"
          ? 400
          : fault === "missing suggestion"
            ? 404
            : 409;
    return { status, json: { error: "這次決定被拒絕，請重新核對建議。" } };
  });
  return {
    writes,
    releaseResponse,
    initialDecision,
    current: () => saved,
    submissions: () => submissions,
  };
}

for (const [fault, decision, uncertain] of [
  ["server error", "accepted", true],
  ["lost reply", "rejected", true],
  ["truncated reply", "accepted", true],
  ["null receipt", "rejected", true],
  ["incomplete receipt", "accepted", true],
  ["opposite choice", "rejected", true],
  ["another suggestion", "accepted", true],
  ["rejected input", "rejected", false],
  ["missing suggestion", "accepted", false],
  ["applied conflict", "rejected", false],
] as const) {
  test(`a suggestion decision with ${fault} reports its actual certainty before an explicit resend`, async () => {
    const page = app();
    let holdSession = false;
    let sessionReads = 0;
    let releaseSession!: () => void;
    const sessionGate = new Promise<void>((resolve) => {
      releaseSession = resolve;
    });
    page.route("**/me", async () => {
      sessionReads++;
      if (holdSession) await sessionGate;
      return { json: ME };
    });
    const receiver = receiveDecision(page, fault, uncertain);
    let reads = 0;
    page.route(`**/runs/${RUN}/suggestions`, () => {
      reads++;
      return { json: { ...SUGGESTIONS, suggestions: [receiver.current()] } };
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await page.eventually(() => expect(page.locator(".auth-name").text()).toBe("tester"));
      const item = page.locator("li.suggestion");
      const submit = item.getByRole("button", {
        name: decision === "accepted" ? "接受" : "拒絕",
        exact: true,
      });
      await page.eventually(() => expect(submit.count()).toBe(1));
      await submit.click();
      await page.eventually(() => expect(receiver.writes).toHaveLength(1));
      const apply = page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ });
      await page.eventually(() => expect(apply.isDisabled()).toBe(true));
      receiver.releaseResponse();
      const alert = item.getByRole("alert");
      const prior = receiver.initialDecision === "accepted" ? "已接受" : "尚未決定";
      await page.eventually(() => {
        expect(alert.text()).toContain(uncertain ? "尚未確認" : REFUSAL_SAID[fault]);
        expect(alert.text()).not.toContain("這次決定被拒絕，請重新核對建議。");
        expect(alert.isFocused()).toBe(true);
        expect(item.text()).not.toContain("這個決定沒有記錄");
        expect(item.text()).toContain(`${uncertain ? "上次讀取" : "目前"}：${prior}`);
      });
      expect(receiver.writes).toEqual([{ decision }]);
      expect(reads).toBe(1);
      expect(await page.axeViolations("body")).toEqual([]);
      if (uncertain) {
        expect(apply.isDisabled()).toBe(true);
        expect(
          page
            .getByText(
              "改善建議的決定尚未確認；請等待目前操作完成，或重新送出未確認的決定後，再建立版本。",
              { exact: true },
            )
            .count(),
        ).toBe(1);
        expect(receiver.current().decision).toBe(decision);
        holdSession = true;
        const previous = sessionReads;
        await act(async () => {
          document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
        });
        await page.eventually(() => expect(sessionReads).toBe(previous + 1));
        await submit.click();
        await page.eventually(() => {
          expect(item.getByRole("alert").filter({ hasText: "這次操作尚未送出" }).isFocused()).toBe(
            true,
          );
          expect(item.getByRole("alert").filter({ hasText: "尚未確認" }).count()).toBe(1);
          expect(item.text()).toContain(`上次讀取：${prior}`);
        });
        expect(receiver.writes).toEqual([{ decision }]);
        holdSession = false;
        releaseSession();
        await page.eventually(() =>
          expect(page.locator("header").getByRole("status").count()).toBe(0),
        );
        await submit.click();
        await page.eventually(() => {
          expect(submit.attribute("aria-pressed")).toBe("true");
          expect(item.getByRole("alert").count()).toBe(0);
          expect(item.text()).toContain(`目前：${decision === "accepted" ? "已接受" : "已拒絕"}`);
          expect(apply.isDisabled()).toBe(decision === "rejected");
        });
        expect(receiver.writes).toEqual([{ decision }, { decision }]);
        expect(reads).toBe(2);
      } else {
        expect(receiver.current().decision).toBe(receiver.initialDecision);
        expect(page.locator("#suggestion-decisions-note").count()).toBe(0);
        if (receiver.initialDecision === "accepted") expect(apply.isDisabled()).toBe(false);
      }
    } finally {
      releaseSession();
      receiver.releaseResponse();
    }
  });
}
