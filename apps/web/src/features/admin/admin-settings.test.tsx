import { act } from "react";
import { expect, test, vi } from "vitest";
import { notifyManager } from "@tanstack/react-query";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { type PlatformAgent } from "./admin.service";
import {
  ADMIN_AGENTS,
  ADMIN_MODEL_BUDGETS,
  ADMIN_RUN_SETTINGS,
} from "../../testing/fixtures/platform";
import {
  container,
  calls,
  stub,
  mountAt,
  waitFor,
  has,
  field,
  type,
  button,
  click,
  submit,
} from "./testing/harness";

test.each(["judge", "rounds", "budget", "tokens"] as const)(
  "an incomplete successful settings response for %s requires a fresh read without replaying the write",
  async (kind) => {
    const judge = kind === "judge";
    const group =
      kind === "rounds"
        ? "continuation_rounds"
        : kind === "budget"
          ? "model_budget"
          : "tokens_per_minute";
    const valueKey = kind === "rounds" ? "rounds" : kind === "budget" ? "credits" : "limit";
    const value = kind === "rounds" ? 5 : kind === "budget" ? 700 : 300000;
    const id = judge
      ? "admin-settings-judge-panel"
      : kind === "rounds"
        ? "admin-settings-rounds"
        : kind === "budget"
          ? "admin-settings-run-budget"
          : "admin-settings-run-tpm";
    const read = judge ? "/admin/settings" : "/admin/settings/runs";
    const write = judge
      ? "/admin/settings/judge-panel"
      : kind === "rounds"
        ? "/admin/settings/runs/continuation-rounds"
        : kind === "budget"
          ? "/admin/settings/runs/model-budget"
          : "/admin/settings/runs/tokens-per-minute";
    let written = false;
    let recover = false;
    stub(true, (path, method) => {
      if (path === write && method === "PUT") {
        written = true;
        return { body: {}, status: 200 };
      }
      if (path === read && method === "GET") {
        if (written && !recover) return { body: { error: "settings unavailable" }, status: 503 };
        return {
          body: judge
            ? {
                judge_panel: { enabled: written },
                injection_check: { enabled: false },
                reflection_candidates: { count: 1 },
              }
            : written
              ? {
                  ...ADMIN_RUN_SETTINGS,
                  [group]: { ...ADMIN_RUN_SETTINGS[group], [valueKey]: value },
                }
              : ADMIN_RUN_SETTINGS,
          status: 200,
        };
      }
      return undefined;
    });
    await mountAt("/admin/settings");
    await waitFor(has("目前：3 輪"));
    if (!judge) await type(`#${id}-value`, String(value));
    await type(`#${id}-note`, "verify the setting");
    await submit(`#${id}-note`);
    await waitFor(has("目前設定尚未確認"));
    await waitFor(has(judge ? "暫時無法讀取平台設定" : "暫時無法讀取試跑設定"));
    expect(container.querySelector(`#${id}-note`)).toBeNull();
    expect(field<HTMLElement>(`#${id}-result`).classList.contains("notice-success")).toBe(false);
    expect(calls.filter((call) => call.url === read && call.method === "GET")).toHaveLength(2);
    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);
    recover = true;
    await click(button(judge ? "再讀取平台設定" : "再讀取試跑設定"));
    const success = judge
      ? "已開啟，需要模型判定的條件"
      : kind === "rounds"
        ? "已改為 5 輪"
        : kind === "budget"
          ? "已改為 700 點"
          : "已改為每分鐘 300,000 tokens";
    await waitFor(has(success));
    expect(field<HTMLElement>(`#${id}-result`).classList.contains("notice-success")).toBe(true);
    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);
  },
);

test("a revised model timeout replaces the previous result even when reread stays stale", async () => {
  stub(true, (path, method) =>
    path === "/admin/model-budgets/judge-run" && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));

  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await click(button("改 評估判定 的秒數"));
  await waitFor(has("重新讀取仍顯示 90 秒；請重新整理確認。"));
  expect(field<HTMLElement>("#admin-budget-result").classList.contains("notice-warning")).toBe(
    true,
  );
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));

  await type("#admin-budget-judge-run-seconds", "101");
  expect(container.querySelector("#admin-budget-result")).toBeNull();
});

test("changing a timeout confirms the platform setting without promising the service deadline", async () => {
  let currentSeconds = 90;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      return {
        body: {
          budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
            budget.kind === "judge-run" ? { ...budget, seconds: currentSeconds } : budget,
          ),
        },
        status: 200,
      };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "PUT") {
      currentSeconds = 100;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await click(button("改 評估判定 的秒數"));

  await waitFor(has("目前：100 秒（已調整）"));
  await waitFor(has("目前顯示設定 100 秒；下次呼叫平台會送出此值，模型服務可能採用更短的上限。"));
  expect(has("設定已變更；草稿改為最新的 100 秒")()).toBe(false);
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("100");
  expect(field<HTMLElement>("#admin-budget-result").classList.contains("notice-success")).toBe(
    true,
  );
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));
});

test("OPS-009: a completed timeout change stays visible when rereading fails", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      reads += 1;
      return reads === 1
        ? { body: ADMIN_MODEL_BUDGETS, status: 200 }
        : { body: { error: "service unavailable" }, status: 503 };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "PUT") {
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await click(button("改 評估判定 的秒數"));
  await waitFor(has("暫時無法讀取模型呼叫逾時"));

  expect(has("目前：90 秒（已調整）")()).toBe(false);
  expect(field<HTMLElement>("#admin-budget-result").textContent).toContain(
    "目前設定暫時無法重新讀取，請稍後核對。",
  );
  expect(field<HTMLElement>("#admin-budget-result").classList.contains("notice-warning")).toBe(
    true,
  );
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));
});

test("OPS-009: an override shows its effective value, default, range, and closed choices", async () => {
  stub(true);
  await mountAt("/admin/model-budgets");
  await waitFor(() => !!container.querySelector("li.download-item"));

  const judge = field<HTMLLIElement>("li.download-item");
  expect(judge.querySelector(".badge-row")?.textContent).toContain("目前：90 秒（已調整）");
  expect(judge.textContent).toContain("程式預設：130 秒；可設定範圍：1～130 秒");
  expect(judge.querySelector(".badge-danger")).toBeNull();
  const change = field<HTMLDetailsElement>("#admin-budget-judge-run-set");
  const restore = field<HTMLDetailsElement>("#admin-budget-judge-run-clear");
  expect(change.open).toBe(false);
  expect(restore.open).toBe(false);
  expect(restore.querySelector("summary")?.textContent).toContain("130 秒");

  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  expect(change.open).toBe(true);
  expect(restore.open).toBe(false);
  expect(container.querySelector("#admin-budget-match-reasons-clear")).toBeNull();
});

test("OPS-009: restoring the default keeps a focused result after the restore form disappears", async () => {
  let overridden = true;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      return {
        body: {
          budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
            budget.kind === "judge-run" && !overridden
              ? { ...budget, seconds: null, reason: null, set_at: null }
              : budget,
          ),
        },
        status: 200,
      };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "DELETE") {
      overridden = false;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("90");
  await click(field<HTMLElement>("#admin-budget-judge-run-clear summary"));
  await type("#admin-budget-judge-run-clear-note", "恢復平台預設");
  await click(button("把 評估判定 改回預設"));
  await waitFor(has("目前：預設 130 秒"));

  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("130");
  expect(has("設定已變更；草稿改為最新的 130 秒")()).toBe(false);
  expect(container.querySelector("#admin-budget-judge-run-clear")).toBeNull();
  await waitFor(
    has("目前顯示程式預設 130 秒；下次呼叫平台會送出此值，模型服務可能採用更短的上限。"),
  );
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));
  await type("#admin-budget-judge-run-seconds", "100");
  expect(container.querySelector("#admin-budget-result")).toBeNull();
});

test("OPS-009: a successful restore remains visible when the new budget cannot be read", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      reads += 1;
      return reads === 1
        ? { body: ADMIN_MODEL_BUDGETS, status: 200 }
        : { body: { error: "service unavailable" }, status: 503 };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "DELETE") {
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-clear summary"));
  await type("#admin-budget-judge-run-clear-note", "恢復平台預設");
  await click(button("把 評估判定 改回預設"));
  await waitFor(has("暫時無法讀取模型呼叫逾時"));

  expect(has("目前：90 秒（已調整）")()).toBe(false);
  await waitFor(has("評估判定的恢復預設請求已完成。"));
  expect(field<HTMLElement>("#admin-budget-result").textContent).toContain(
    "目前設定暫時無法重新讀取，請稍後核對。",
  );
});

test("OPS-009: refreshing a changed model timeout replaces an obsolete editable draft", async () => {
  let currentSeconds = 90;
  stub(true, (path, method) =>
    path === "/admin/model-budgets" && method === "GET"
      ? {
          body: {
            budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
              budget.kind === "judge-run" ? { ...budget, seconds: currentSeconds } : budget,
            ),
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");

  currentSeconds = 120;
  await click(button("重新整理"));
  await waitFor(has("目前：120 秒（已調整）"));
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("120");
  expect(has("設定已變更；草稿改為最新的 120 秒，請確認後再送出。")()).toBe(true);
});

for (const setting of [
  {
    name: "answered rounds",
    fact: "目前：3 輪",
    page: "/admin/settings",
    endpoint: "/admin/settings/runs",
    queryKey: queryKeys.admin.runSettings,
    selector: "#admin-settings-rounds-value",
    original: "3",
    changed: "5",
    edited: "7",
    snapshot: (value: string) => ({
      ...ADMIN_RUN_SETTINGS,
      continuation_rounds: { ...ADMIN_RUN_SETTINGS.continuation_rounds, rounds: Number(value) },
    }),
  },
  {
    name: "model timeout",
    fact: "目前：90 秒（已調整）",
    page: "/admin/model-budgets",
    endpoint: "/admin/model-budgets",
    queryKey: queryKeys.admin.modelBudgets,
    selector: "#admin-budget-judge-run-seconds",
    original: "90",
    changed: "120",
    edited: "100",
    snapshot: (value: string) => ({
      budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
        budget.kind === "judge-run" ? { ...budget, seconds: Number(value) } : budget,
      ),
    }),
  },
  {
    name: "Agent daily spend cap",
    fact: "每日花費上限 $0.2000",
    page: "/admin/agents",
    endpoint: "/admin/agents",
    queryKey: queryKeys.admin.agents,
    selector: "#admin-agent-daily-report-cap",
    original: "0.2",
    changed: "1",
    edited: "2.5",
    snapshot: (value: string) => ({
      agents: [
        { ...ADMIN_AGENTS.agents[0], daily_spend_cap_usd_micros: Number(value) * 1_000_000 },
      ],
    }),
  },
] as const) {
  for (const phase of ["round trip", "unchanged blank"] as const) {
    test(`observed admin ${setting.name} retains its current draft after ${phase}`, async () => {
      let current: string = setting.original;
      stub(true, (path, method) =>
        path === setting.endpoint && method === "GET"
          ? { body: setting.snapshot(current), status: 200 }
          : undefined,
      );
      await mountAt(setting.page);
      await waitFor(() => Boolean(container.querySelector(setting.selector)));
      await type(setting.selector, phase === "round trip" ? setting.edited : "");
      if (phase === "round trip") {
        current = setting.changed;
        await act(async () => {
          await queryClient.refetchQueries({ queryKey: setting.queryKey, exact: true });
        });
        await waitFor(() => field<HTMLInputElement>(setting.selector).value === setting.changed);
      }
      current = setting.original;
      await act(async () => {
        await queryClient.refetchQueries({ queryKey: setting.queryKey, exact: true });
      });
      await waitFor(() => queryClient.isFetching() === 0);
      await waitFor(has(setting.fact));
      expect(field<HTMLInputElement>(setting.selector).value).toBe(
        phase === "round trip" ? setting.original : "",
      );
      expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
    });
  }
}

test("OPS-009: range endpoints are accepted and adjacent values are blocked", async () => {
  stub(true);
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-note", "reviewing deadline");

  for (const seconds of ["1", "130"]) {
    await type("#admin-budget-judge-run-seconds", seconds);
    expect(button("改 評估判定 的秒數").disabled).toBe(false);
  }
  for (const seconds of ["0", "131"]) {
    await type("#admin-budget-judge-run-seconds", seconds);
    expect(button("改 評估判定 的秒數").disabled).toBe(true);
    expect(has("要填 1 到 130 之間的整數秒")()).toBe(true);
  }
});

test("OPS-009: failed refresh hides stale settings and retry restores the list", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/model-budgets") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_MODEL_BUDGETS, status: 200 };
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await click(button("重新整理"));
  await waitFor(has("暫時無法讀取模型呼叫逾時"));
  expect(reads).toBe(2);
  expect(container.querySelector("li.download-item")).toBeNull();

  await click(button("再試一次"));
  await waitFor(() => reads === 3 && !!container.querySelector("li.download-item"));
  expect(has("暫時無法讀取模型呼叫逾時")()).toBe(false);
});

test("OPS-009: an in-flight timeout change blocks restoring the same call kind", async () => {
  stub(true, (path, method) =>
    path === "/admin/model-budgets/judge-run" && method === "DELETE"
      ? { body: {}, status: 200 }
      : undefined,
  );
  const read = globalThis.fetch;
  let finishSet: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/model-budgets/judge-run") && init?.method === "PUT") {
      return new Promise<Response>((resolve) => {
        finishSet = resolve;
      });
    }
    return read(input, init);
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await click(field<HTMLElement>("#admin-budget-judge-run-clear summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await type("#admin-budget-judge-run-clear-note", "恢復預設");
  await click(button("改 評估判定 的秒數"));
  await waitFor(() => finishSet !== undefined);
  await waitFor(has("這一種呼叫正在調整秒數，完成後才能恢復預設。"));

  expect(button("把 評估判定 改回預設").disabled).toBe(true);
  expect(has("這一種呼叫正在調整秒數，完成後才能恢復預設。")()).toBe(true);
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").readOnly).toBe(true);
  await act(async () => finishSet!(new Response("{}", { status: 200 })));
  await waitFor(has("評估判定的調整秒數請求已完成。"));

  await click(button("把 評估判定 改回預設"));
  await waitFor(has("評估判定的恢復預設請求已完成。"));
  expect(has("評估判定的調整秒數請求已完成。")()).toBe(false);
});

test("OPS-009: an empty configured-call roster names the absence", async () => {
  stub(true, (path) =>
    path === "/admin/model-budgets" ? { body: { budgets: [] }, status: 200 } : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前沒有可設定的模型呼叫"));
  expect(container.querySelector("li.download-item")).toBeNull();
});

test("OPS-011: switching the judge panel on sends the choice with its reason and shows the new state", async () => {
  let enabled = false;
  stub(true, (path, method) => {
    if (path === "/admin/settings/judge-panel" && method === "PUT") {
      enabled = true;
      return {
        body: {
          judge_panel: { enabled, reason: "measure agreement", set_at: "2026-10-09T08:00:00Z" },
          injection_check: { enabled: false },
          reflection_candidates: { count: 1 },
        },
        status: 200,
      };
    }
    return undefined;
  });
  await mountAt("/admin/settings");
  await waitFor(has("關閉：一個判定（預設）"));
  expect(button("開啟評審團").disabled).toBe(true);

  await type("#admin-settings-judge-panel-note", "measure agreement");
  await click(button("開啟評審團"));
  await waitFor(has("已開啟，需要模型判定的條件由三個判定投票。"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: "/admin/settings/judge-panel",
    body: { enabled: true, note: "measure agreement" },
  });
  expect(has("開啟：三個判定逐條多數決")()).toBe(true);
  expect(has("理由：measure agreement")()).toBe(true);
  expect(button("關閉評審團")).toBeDefined();
});

test("switching the injection check on sends its price with the reason and shows the priced state", async () => {
  stub(true, (path, method) =>
    path === "/admin/settings/injection-check" && method === "PUT"
      ? {
          body: {
            judge_panel: { enabled: false },
            reflection_candidates: { count: 1 },
            injection_check: {
              enabled: true,
              credits_per_million_tokens: 1000000,
              reason: "second signal",
              set_at: "2026-11-01T08:00:00Z",
            },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/settings");
  await waitFor(has("尚未定價"));
  await type("#admin-settings-injection-check-note", "second signal");
  expect(button("開啟注入檢查").disabled).toBe(true);

  for (const price of ["0", "1000001", "2.5"]) {
    await type("#admin-settings-injection-check-price", price);
    expect(button("開啟注入檢查").disabled).toBe(true);
  }
  await type("#admin-settings-injection-check-price", "1");
  expect(button("開啟注入檢查").disabled).toBe(false);
  await type("#admin-settings-injection-check-price", "1000000");
  await click(button("開啟注入檢查"));
  await waitFor(has("已開啟，下一次需要模型判定的評估會先檢查輸出。"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: "/admin/settings/injection-check",
    body: { enabled: true, credits_per_million_tokens: 1000000, note: "second signal" },
  });
  expect(has("開啟：判定前先檢查輸出")()).toBe(true);
  expect(has("每百萬輸入 token 1000000 點")()).toBe(true);
  expect(has("理由：second signal")()).toBe(true);
  expect(button("關閉注入檢查")).toBeDefined();
});

test("a judge panel change focuses its result and editing the next reason clears the old confirmation", async () => {
  stub(true, (path, method) =>
    path === "/admin/settings/judge-panel" && method === "PUT"
      ? {
          body: {
            judge_panel: { enabled: true },
            injection_check: { enabled: false },
            reflection_candidates: { count: 1 },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/settings");
  await waitFor(has("關閉：一個判定（預設）"));
  await type("#admin-settings-judge-panel-note", "measure agreement");
  await submit("#admin-settings-judge-panel-note");
  await waitFor(has("已開啟，需要模型判定的條件由三個判定投票。"));
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-settings-judge-panel-result"));
  await type("#admin-settings-judge-panel-note", "back to baseline");
  await waitFor(() => !has("已開啟，需要模型判定的條件由三個判定投票。")());
  expect(has("已開啟，需要模型判定的條件由三個判定投票。")()).toBe(false);
  expect(button("關閉評審團").disabled).toBe(false);
});

test.each(["judge panel", "answer rounds"] as const)(
  "a changed %s can be reviewed and edited again without leaving settings",
  async (policy) => {
    const judge = policy === "judge panel";
    const id = judge ? "admin-settings-judge-panel" : "admin-settings-rounds";
    const writePath = judge
      ? "/admin/settings/judge-panel"
      : "/admin/settings/runs/continuation-rounds";
    stub(true, (path, method) =>
      path === writePath && method === "PUT"
        ? {
            body: judge
              ? {
                  judge_panel: { enabled: true },
                  injection_check: { enabled: false },
                  reflection_candidates: { count: 1 },
                }
              : {
                  continuation_rounds: { rounds: 5, min: 1, max: 10 },
                  model_budget: ADMIN_RUN_SETTINGS.model_budget,
                  tokens_per_minute: ADMIN_RUN_SETTINGS.tokens_per_minute,
                },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/settings");
    await waitFor(has("目前：3 輪"));
    if (!judge) await type(`#${id}-value`, "5");
    await type(`#${id}-note`, "first decision");
    await submit(`#${id}-note`);
    await waitFor(has(judge ? "已開啟，需要模型判定的條件" : "已改為 5 輪，下一次回答"));

    await click(button("重新整理設定"));
    await waitFor(has("目前設定尚未確認"));
    expect(container.querySelector(`#${id}-note`)).toBeNull();
    expect(field<HTMLElement>(`#${id}-result`).classList.contains("notice-success")).toBe(false);
    const reviewLabel = judge ? "檢視目前的評審團設定" : "檢視目前的回答輪數";
    const review = button(reviewLabel);

    const fetch = window.fetch;
    const readPath = judge ? "/admin/settings" : "/admin/settings/runs";
    let finishRead!: (response: Response) => void;
    const held = new Promise<Response>((resolve) => {
      finishRead = resolve;
    });
    vi.stubGlobal("fetch", (input: string, init?: RequestInit) =>
      String(input).endsWith(readPath) && (!init?.method || init.method === "GET")
        ? held
        : fetch(input, init),
    );
    try {
      await click(button("重新整理設定"));
      await waitFor(has("重新整理中…"));
      expect(review.disabled).toBe(true);
      await click(review);
      expect(container.querySelector(`#${id}-note`)).toBeNull();
      expect(container.querySelector(`#${id}-result`)).not.toBeNull();
    } finally {
      await act(async () => {
        finishRead(
          new Response(JSON.stringify({ error: "settings unavailable" }), { status: 503 }),
        );
      });
    }
    await waitFor(has(judge ? "暫時無法讀取平台設定" : "暫時無法讀取試跑設定"));
    expect(container.querySelector(`#${id}-note`)).toBeNull();
    expect(container.contains(review)).toBe(false);

    vi.stubGlobal("fetch", fetch);
    await click(button(judge ? "再讀取平台設定" : "再讀取試跑設定"));
    await waitFor(has(reviewLabel));
    expect(container.querySelector(`#${id}-note`)).toBeNull();
    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);
    const currentReview = button(reviewLabel);
    await act(async () => {
      currentReview.click();
      await new Promise<void>((resolve) => notifyManager.schedule(resolve));
    });

    expect(document.activeElement).toBe(field<HTMLElement>(`#${id}-heading`));
    expect(container.querySelector(`#${id}-result`)).toBeNull();
    expect(field<HTMLInputElement | HTMLTextAreaElement>(`#${id}-note`).value).toBe("");
    expect(has(judge ? "關閉：一個判定（預設）" : "目前：3 輪（可設 1–10 輪）")()).toBe(true);
    const edit = field<HTMLInputElement | HTMLTextAreaElement>(
      judge ? `#${id}-note` : `#${id}-value`,
    );
    expect(edit.value).toBe(judge ? "" : "3");
    expect(button(judge ? "開啟評審團" : "改輪數").disabled).toBe(true);
    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);
  },
);

test("unreadable cached platform settings hide their controls and retry each setting in place", async () => {
  let unavailable = false;
  stub(true, (path) => {
    if (path === "/admin/settings" || path === "/admin/settings/runs") {
      if (unavailable) return { body: { error: "settings unavailable" }, status: 503 };
      return {
        body:
          path === "/admin/settings"
            ? {
                judge_panel: { enabled: false },
                injection_check: { enabled: false },
                reflection_candidates: { count: 1 },
              }
            : {
                continuation_rounds: { rounds: 3, min: 1, max: 10 },
                model_budget: ADMIN_RUN_SETTINGS.model_budget,
                tokens_per_minute: ADMIN_RUN_SETTINGS.tokens_per_minute,
              },
        status: 200,
      };
    }
    return undefined;
  });
  await mountAt("/admin/settings");
  await waitFor(has("目前：3 輪"));
  unavailable = true;
  await act(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.admin.settings }),
      queryClient.invalidateQueries({ queryKey: queryKeys.admin.runSettings }),
    ]);
  });
  await waitFor(has("暫時無法讀取試跑設定"));
  expect(container.querySelector("#admin-settings-judge-panel-note")).toBeNull();
  expect(container.querySelector("#admin-settings-rounds-value")).toBeNull();
  expect(has("關閉：一個判定（預設）")()).toBe(false);
  unavailable = false;
  await click(button("再讀取平台設定"));
  await waitFor(has("關閉：一個判定（預設）"));
  expect(container.querySelector("#admin-settings-rounds-value")).toBeNull();
  await click(button("再讀取試跑設定"));
  await waitFor(has("目前：3 輪"));
  expect(field<HTMLInputElement>("#admin-settings-rounds-value").value).toBe("3");
});

test("a refreshed answer-round limit replaces a stale draft with the current value", async () => {
  let rounds = 3;
  stub(true, (path) =>
    path === "/admin/settings/runs"
      ? {
          body: {
            continuation_rounds: { rounds, min: 1, max: 10 },
            model_budget: ADMIN_RUN_SETTINGS.model_budget,
            tokens_per_minute: ADMIN_RUN_SETTINGS.tokens_per_minute,
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/settings");
  await waitFor(has("目前：3 輪"));
  await type("#admin-settings-rounds-value", "4");
  rounds = 5;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.runSettings });
  });
  await waitFor(has("目前：5 輪"));
  expect(field<HTMLInputElement>("#admin-settings-rounds-value").value).toBe("5");
  expect(has("輪數已變更；草稿改為目前的 5 輪，請確認後再送出。")()).toBe(true);
  expect(calls.filter((call) => call.method === "PUT")).toHaveLength(0);
});

test.each(["set", "restore"] as const)(
  "an incomplete successful Agent cap response can confirm %s through a fresh read without replaying the write",
  async (action) => {
    let sent = false;
    let recover = false;
    const before = {
      ...ADMIN_AGENTS.agents[0],
      daily_spend_cap_usd_micros: action === "set" ? 2_000_000 : 1_000_000,
      daily_spend_cap_overridden: true,
    };
    const after =
      action === "restore"
        ? ADMIN_AGENTS.agents[0]
        : { ...before, daily_spend_cap_usd_micros: 2_000_000 };
    stub(true, (path, method) => {
      if (path === "/admin/agents/daily-report/spend-cap" && method === "PUT") {
        sent = true;
        return { body: {}, status: 200 };
      }
      if (path === "/admin/agents") {
        if (sent && !recover) return { body: { error: "agent status unavailable" }, status: 503 };
        return { body: { agents: [sent ? after : before] }, status: 200 };
      }
      return undefined;
    });
    await mountAt("/admin/agents");
    await waitFor(has("（營運者設定；預設 $0.2000）"));
    if (action === "set") await type("#admin-agent-daily-report-cap", "2.00");
    const note = `#admin-agent-daily-report-cap-${action === "set" ? "set" : "clear"}-note`;
    await type(note, "verify the cap");
    await submit(note);
    await waitFor(has("daily-report 的每日花費上限的變更請求已完成；最新上限尚未確認。"));
    await waitFor(has("暫時無法讀取 Agent 控制"));
    expect(
      field<HTMLInputElement>("#admin-agent-daily-report-cap").closest<HTMLElement>("[hidden]")
        ?.hidden,
    ).toBe(true);
    recover = true;
    await click(button("重新整理此 Agent 的上限"));
    await waitFor(
      has(action === "restore" ? "目前顯示預設 $0.2000" : "目前顯示營運者設定 $2.0000"),
    );
    expect(field<HTMLInputElement>("#admin-agent-daily-report-cap").value).toBe(
      action === "restore" ? "0.2" : "2",
    );
    expect(field<HTMLInputElement>("#admin-agent-daily-report-cap").readOnly).toBe(false);
    if (action === "set") {
      expect(field<HTMLTextAreaElement>("#admin-agent-daily-report-cap-set-note").value).toBe(
        "verify the cap",
      );
    }
    expect(calls.filter((call) => call.method === "PUT")).toEqual([
      {
        method: "PUT",
        url: "/admin/agents/daily-report/spend-cap",
        body: {
          daily_spend_cap_usd_micros: action === "restore" ? null : 2_000_000,
          note: "verify the cap",
        },
      },
    ]);
  },
);

test("OPS-011: an agent's daily spend cap accepts up to five dollars and sends micro-dollars", async () => {
  stub(true, (path, method) =>
    path === "/admin/agents/daily-report/spend-cap" && method === "PUT"
      ? { body: ADMIN_AGENTS.agents[0], status: 200 }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has("（預設）"));
  await type("#admin-agent-daily-report-cap-set-note", "busy week");
  const cap = field<HTMLInputElement>("#admin-agent-daily-report-cap");
  expect([null, "false"]).toContain(cap.getAttribute("aria-invalid"));

  for (const refused of ["0", "5.000001", "0.0000001", "abc", ""]) {
    await type("#admin-agent-daily-report-cap", refused);
    expect(button("改 daily-report 的每日上限").disabled, refused).toBe(true);
    expect(cap.getAttribute("aria-invalid"), refused).toBe("true");
  }
  await type("#admin-agent-daily-report-cap", "5");
  expect(button("改 daily-report 的每日上限").disabled).toBe(false);
  expect([null, "false"]).toContain(cap.getAttribute("aria-invalid"));
  await click(button("改 daily-report 的每日上限"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: "/admin/agents/daily-report/spend-cap",
    body: { daily_spend_cap_usd_micros: 5_000_000, note: "busy week" },
  });
});

test("OPS-011: an operator-set cap shows the default it replaced and can be cleared back to it", async () => {
  stub(true, (path, method) => {
    if (path === "/admin/agents" && method === "GET")
      return {
        body: {
          agents: [
            {
              ...ADMIN_AGENTS.agents[0],
              daily_spend_cap_usd_micros: 1_000_000,
              daily_spend_cap_overridden: true,
            },
          ],
        },
        status: 200,
      };
    if (path === "/admin/agents/daily-report/spend-cap")
      return { body: ADMIN_AGENTS.agents[0], status: 200 };
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("（營運者設定；預設 $0.2000）"));
  await type("#admin-agent-daily-report-cap-clear-note", "back to normal");
  await click(button("把 daily-report 的上限改回預設 $0.2000"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({
    daily_spend_cap_usd_micros: null,
    note: "back to normal",
  });
});

test("restoring an Agent cap updates its input and retains a focused result after the form disappears", async () => {
  let current = {
    ...ADMIN_AGENTS.agents[0],
    daily_spend_cap_usd_micros: 1_000_000,
    daily_spend_cap_overridden: true,
  };
  stub(true, (path, method) => {
    if (path === "/admin/agents/daily-report/spend-cap" && method === "PUT") {
      current = { ...ADMIN_AGENTS.agents[0] };
      return { body: current, status: 200 };
    }
    if (path === "/admin/agents") return { body: { agents: [current] }, status: 200 };
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("（營運者設定；預設 $0.2000）"));
  expect(field<HTMLInputElement>("#admin-agent-daily-report-cap").value).toBe("1");
  await type("#admin-agent-daily-report-cap-clear-note", "back to normal");
  await submit("#admin-agent-daily-report-cap-clear-note");
  await waitFor(has("目前顯示預設 $0.2000"));
  expect(field<HTMLInputElement>("#admin-agent-daily-report-cap").value).toBe("0.2");
  expect(container.querySelector("#admin-agent-daily-report-cap-clear-note")).toBeNull();
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-agent-daily-report-cap-result"));
  await type("#admin-agent-daily-report-cap", "0.5");
  expect(container.querySelector("#admin-agent-daily-report-cap-result")).toBeNull();
});

test.each(["stale", "unavailable"] as const)(
  "a restored Agent cap keeps its result and hides controls when the readback is %s",
  async (readback) => {
    let sent = false;
    let verified = false;
    stub(true, (path, method) => {
      if (path === "/admin/agents/daily-report/spend-cap" && method === "PUT") {
        sent = true;
        return { body: ADMIN_AGENTS.agents[0], status: 200 };
      }
      if (path === "/admin/agents") {
        if (sent && !verified && readback === "unavailable") {
          return { body: { error: "agent status unavailable" }, status: 503 };
        }
        return {
          body: {
            agents: [
              {
                ...ADMIN_AGENTS.agents[0],
                daily_spend_cap_usd_micros: verified ? 200_000 : 1_000_000,
                daily_spend_cap_overridden: !verified,
              },
            ],
          },
          status: 200,
        };
      }
      return undefined;
    });
    await mountAt("/admin/agents");
    await waitFor(has("（營運者設定；預設 $0.2000）"));
    await type("#admin-agent-daily-report-cap-clear-note", "back to normal");
    await submit("#admin-agent-daily-report-cap-clear-note");
    await waitFor(has("每日花費上限的變更請求已完成；最新上限尚未確認。"));
    const cap = field<HTMLInputElement>("#admin-agent-daily-report-cap");
    expect(cap.readOnly).toBe(true);
    expect(cap.closest<HTMLElement>("[hidden]")?.hidden).toBe(true);
    expect(document.activeElement).toBe(field<HTMLElement>("#admin-agent-daily-report-cap-result"));
    verified = true;
    await click(button("重新整理此 Agent 的上限"));
    await waitFor(has("目前顯示預設 $0.2000"));
    expect(field<HTMLInputElement>("#admin-agent-daily-report-cap").value).toBe("0.2");
  },
);

test("normalizing an Agent cap keeps the completed request blocked until a user edits", async () => {
  let current: PlatformAgent = { ...ADMIN_AGENTS.agents[0] };
  stub(true, (path, method) => {
    if (path === "/admin/agents/daily-report/spend-cap" && method === "PUT") {
      current = {
        ...current,
        daily_spend_cap_usd_micros: 1_000_000,
        daily_spend_cap_overridden: true,
      };
      return { body: current, status: 200 };
    }
    if (path === "/admin/agents") return { body: { agents: [current] }, status: 200 };
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("（預設）"));
  await type("#admin-agent-daily-report-cap", "1.00");
  await type("#admin-agent-daily-report-cap-set-note", "busy week");
  await submit("#admin-agent-daily-report-cap-set-note");
  await waitFor(has("目前顯示營運者設定 $1.0000"));
  expect(field<HTMLInputElement>("#admin-agent-daily-report-cap").value).toBe("1");
  expect(field<HTMLTextAreaElement>("#admin-agent-daily-report-cap-set-note").value).toBe(
    "busy week",
  );
  expect(button("改 daily-report 的每日上限").disabled).toBe(true);
  await submit("#admin-agent-daily-report-cap-set-note");
  expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);
  await type("#admin-agent-daily-report-cap", "2");
  expect(button("改 daily-report 的每日上限").disabled).toBe(false);
});

test("an unchanged Agent cap is still unconfirmed when its refresh fails", async () => {
  let sent = false;
  const current = {
    ...ADMIN_AGENTS.agents[0],
    daily_spend_cap_usd_micros: 1_000_000,
    daily_spend_cap_overridden: true,
  };
  stub(true, (path, method) => {
    if (path === "/admin/agents/daily-report/spend-cap" && method === "PUT") {
      sent = true;
      return { body: current, status: 200 };
    }
    if (path === "/admin/agents")
      return sent
        ? { body: { error: "agent status unavailable" }, status: 503 }
        : { body: { agents: [current] }, status: 200 };
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("（營運者設定；預設 $0.2000）"));
  await type("#admin-agent-daily-report-cap-set-note", "reviewed the limit");
  await submit("#admin-agent-daily-report-cap-set-note");
  await waitFor(has("每日花費上限的變更請求已完成；最新上限尚未確認。"));
  expect(has("目前顯示營運者設定 $1.0000")()).toBe(false);
  const cap = field<HTMLInputElement>("#admin-agent-daily-report-cap");
  expect(cap.readOnly).toBe(true);
  expect(cap.closest<HTMLElement>("[hidden]")?.hidden).toBe(true);
});

test("an Agent cap change blocks a competing restore until the request finishes", async () => {
  let current = { ...ADMIN_AGENTS.agents[0], daily_spend_cap_overridden: true };
  stub(true, (path, method) => {
    if (path === "/admin/agents")
      return {
        body: { agents: [current] },
        status: 200,
      };
    if (path === "/admin/agents/daily-report/spend-cap" && method === "PUT") {
      current = { ...current, daily_spend_cap_usd_micros: 2_000_000 };
      return { body: current, status: 200 };
    }
    return undefined;
  });
  const originalFetch = fetch;
  const finishes: Array<() => void> = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const reply = originalFetch(input, init);
    if (String(input).endsWith("/daily-report/spend-cap") && init?.method === "PUT") {
      return new Promise<Response>((resolve) => {
        finishes.push(() => void reply.then(resolve));
      });
    }
    return reply;
  });
  await mountAt("/admin/agents");
  await waitFor(has("（營運者設定；預設 $0.2000）"));
  await type("#admin-agent-daily-report-cap", "2");
  await type("#admin-agent-daily-report-cap-set-note", "busy week");
  await type("#admin-agent-daily-report-cap-clear-note", "restore");
  await submit("#admin-agent-daily-report-cap-set-note");
  await waitFor(() => finishes.length > 0);
  try {
    await waitFor(
      () => field<HTMLTextAreaElement>("#admin-agent-daily-report-cap-clear-note").readOnly,
    );
    expect(field<HTMLTextAreaElement>("#admin-agent-daily-report-cap-clear-note").readOnly).toBe(
      true,
    );
    expect(button("把 daily-report 的上限改回預設 $0.2000").disabled).toBe(true);
    await submit("#admin-agent-daily-report-cap-clear-note");
    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);
  } finally {
    finishes.forEach((finish) => finish());
  }
  await waitFor(() => !field<HTMLInputElement>("#admin-agent-daily-report-cap").readOnly);
});

const RUN_SETTINGS = {
  continuation_rounds: { rounds: 3, min: 1, max: 10 },
  model_budget: ADMIN_RUN_SETTINGS.model_budget,
  tokens_per_minute: ADMIN_RUN_SETTINGS.tokens_per_minute,
};

function stubRunSettings(put: { body: unknown; status: number }, read: unknown = RUN_SETTINGS) {
  stub(true, (path, method) => {
    if (path === "/admin/settings/runs" && method === "GET") {
      return { body: read, status: 200 };
    }
    if (path === "/admin/settings/runs/continuation-rounds" && method === "PUT") return put;
    if (
      (path === "/admin/settings/runs/model-budget" ||
        path === "/admin/settings/runs/tokens-per-minute") &&
      (method === "PUT" || method === "DELETE")
    )
      return put;
    return undefined;
  });
}

const OPERATOR_BUDGET = {
  ...RUN_SETTINGS,
  model_budget: {
    credits: 260,
    default_credits: 650,
    min_credits: 65,
    max_credits: 6500,
    reason: "smaller runs while we measure",
    set_at: "2026-10-20T08:00:00Z",
  },
};

test("the run spend ceiling shows the deployment default and sends the new ceiling with its reason", async () => {
  stubRunSettings({ body: OPERATOR_BUDGET, status: 200 });
  await mountAt("/admin/settings");
  await waitFor(has("每次試跑的模型花費上限"));
  expect(has("目前：每次試跑最多花 650 點的模型費用（部署預設）")()).toBe(true);
  expect(has("回到部署預設 650 點")()).toBe(false);

  await type("#admin-settings-run-budget-value", "260");
  await type("#admin-settings-run-budget-note", "smaller runs while we measure");
  await click(button("改上限"));
  await waitFor(has("已改為 260 點，新建立的試跑就照這個上限。"));

  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: "/admin/settings/runs/model-budget",
    body: { credits: 260, note: "smaller runs while we measure" },
  });
  expect(has("目前：每次試跑最多花 260 點的模型費用（部署預設 650 點）")()).toBe(true);
  expect(has("理由：smaller runs while we measure")()).toBe(true);
});

test("the run spend ceiling input accepts the range edges and refuses just past them", async () => {
  stubRunSettings({ body: RUN_SETTINGS, status: 200 });
  await mountAt("/admin/settings");
  await waitFor(has("每次試跑的模型花費上限"));
  await type("#admin-settings-run-budget-note", "boundary check");

  for (const [value, refused] of [
    ["64", true],
    ["65", false],
    ["6500", false],
    ["6501", true],
    ["100.5", true],
    ["", true],
    ["abc", true],
  ] as const) {
    await type("#admin-settings-run-budget-value", value);
    expect(button("改上限").disabled, JSON.stringify(value)).toBe(refused);
  }
});

const OPERATOR_RATE = {
  ...RUN_SETTINGS,
  tokens_per_minute: {
    limit: 20000,
    default_limit: 200000,
    min: 10000,
    max: 2000000,
    reason: "provider throttling",
    set_at: "2026-10-20T08:00:00Z",
  },
};

for (const setting of [
  {
    name: "spend ceiling",
    key: "model_budget",
    selector: "#admin-settings-run-budget-value",
    note: "#admin-settings-run-budget-note",
    changed: OPERATOR_BUDGET,
    latest: "260",
    original: "650",
    edited: "300",
    current: "目前：每次試跑最多花 260 點的模型費用",
    initial: "目前：每次試跑最多花 650 點的模型費用",
    reset: "回到部署預設 650 點",
    receipt: "已回到部署預設 650 點，新建立的試跑就照這個上限。",
    warning: "設定已變更；草稿改為目前的 260 點，請確認後再送出。",
  },
  {
    name: "token rate",
    key: "tokens_per_minute",
    selector: "#admin-settings-run-tpm-value",
    note: "#admin-settings-run-tpm-note",
    changed: OPERATOR_RATE,
    latest: "20000",
    original: "200000",
    edited: "30000",
    current: "目前：試跑的模型金鑰最多 每分鐘 20,000 tokens",
    initial: "目前：試跑的模型金鑰最多 每分鐘 200,000 tokens",
    reset: "回到部署預設每分鐘 200,000 tokens",
    receipt: "已回到部署預設每分鐘 200,000 tokens，之後發出的金鑰就照這個值。",
    warning: "設定已變更；草稿改為每分鐘 20,000 tokens，請確認後再送出。",
  },
] as const) {
  for (const phase of ["pristine", "edited", "round trip", "unchanged blank"] as const) {
    test(`run setting ${setting.name} synchronizes its draft after ${phase} refresh`, async () => {
      let current: unknown = RUN_SETTINGS;
      stub(true, (path, method) =>
        path === "/admin/settings/runs" && method === "GET"
          ? { body: current, status: 200 }
          : undefined,
      );
      await mountAt("/admin/settings");
      await waitFor(() => Boolean(container.querySelector(setting.selector)));
      if (phase !== "pristine") {
        await type(setting.selector, phase === "unchanged blank" ? "" : setting.edited);
      }
      current = phase === "unchanged blank" ? RUN_SETTINGS : setting.changed;
      await act(async () => {
        await queryClient.refetchQueries({ queryKey: queryKeys.admin.runSettings, exact: true });
      });
      if (phase === "round trip") {
        await waitFor(has(setting.current));
        current = RUN_SETTINGS;
        await act(async () => {
          await queryClient.refetchQueries({ queryKey: queryKeys.admin.runSettings, exact: true });
        });
      }
      await waitFor(() => queryClient.isFetching() === 0);
      await waitFor(
        has(
          phase === "round trip" || phase === "unchanged blank" ? setting.initial : setting.current,
        ),
      );
      const expected =
        phase === "round trip"
          ? setting.original
          : phase === "unchanged blank"
            ? ""
            : setting.latest;
      expect(field<HTMLInputElement>(setting.selector).value).toBe(expected);
      expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
      if (phase === "pristine" || phase === "edited") expect(has(setting.warning)()).toBe(true);
      if (phase === "unchanged blank") expect(has("設定已變更；草稿改為")()).toBe(false);
    });
  }

  test(`run setting ${setting.name} synchronizes its draft after a same-value restore`, async () => {
    stubRunSettings(
      { body: RUN_SETTINGS, status: 200 },
      {
        ...RUN_SETTINGS,
        [setting.key]: { ...RUN_SETTINGS[setting.key], reason: "explicit default-sized override" },
      },
    );
    await mountAt("/admin/settings");
    await waitFor(() => Boolean(container.querySelector(setting.selector)));
    await type(setting.selector, setting.edited);
    await type(setting.note.replace("-note", "-clear-note"), "restore deployment defaults");
    await click(button(setting.reset));
    await waitFor(has(setting.receipt));
    expect(field<HTMLInputElement>(setting.selector).value).toBe(setting.original);
    expect(container.querySelector('[role="status"]:focus')?.textContent).toContain(
      setting.receipt,
    );
    expect(calls.filter((call) => call.method === "DELETE")).toHaveLength(1);
    expect(calls.filter((call) => call.method === "PUT")).toEqual([]);
    await type(setting.selector, setting.edited);
    expect(field<HTMLInputElement>(setting.selector).value).toBe(setting.edited);
    expect(has(setting.receipt)()).toBe(false);
  });

  test(`run setting ${setting.name} synchronizes its draft after restoring the default`, async () => {
    stubRunSettings({ body: RUN_SETTINGS, status: 200 }, setting.changed);
    await mountAt("/admin/settings");
    await waitFor(() => Boolean(container.querySelector(setting.selector)));
    const clearNote = setting.note.replace("-note", "-clear-note");
    await type(clearNote, "restore the deployment default");
    await click(button(setting.reset));
    await waitFor(has(setting.receipt));
    expect(field<HTMLInputElement>(setting.selector).value).toBe(setting.original);
    expect(container.querySelector('[role="status"]:focus')?.textContent).toContain(
      setting.receipt,
    );
    expect(calls.filter((call) => call.method === "DELETE")).toHaveLength(1);
  });
}

test("the run key tokens-per-minute limit shows the deployment default and sends the new limit with its reason", async () => {
  stubRunSettings({ body: OPERATOR_RATE, status: 200 });
  await mountAt("/admin/settings");
  await waitFor(has("試跑模型金鑰的每分鐘 tokens 上限"));
  expect(has("目前：試跑的模型金鑰最多 每分鐘 200,000 tokens（部署預設）")()).toBe(true);

  await type("#admin-settings-run-tpm-value", "20000");
  await type("#admin-settings-run-tpm-note", "provider throttling");
  await click(button("改每分鐘上限"));
  await waitFor(has("已改為每分鐘 20,000 tokens，之後發出的金鑰就照這個值。"));

  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: "/admin/settings/runs/tokens-per-minute",
    body: { limit: 20000, note: "provider throttling" },
  });
  expect(has("（部署預設每分鐘 200,000 tokens）")()).toBe(true);
  expect(has("理由：provider throttling")()).toBe(true);
});

test("the run key tokens-per-minute input accepts the range edges and refuses just past them", async () => {
  stubRunSettings({ body: RUN_SETTINGS, status: 200 });
  await mountAt("/admin/settings");
  await waitFor(has("試跑模型金鑰的每分鐘 tokens 上限"));
  await type("#admin-settings-run-tpm-note", "boundary check");

  for (const [value, refused] of [
    ["9999", true],
    ["10000", false],
    ["2000000", false],
    ["2000001", true],
    ["15000.5", true],
    ["", true],
  ] as const) {
    await type("#admin-settings-run-tpm-value", value);
    expect(button("改每分鐘上限").disabled, JSON.stringify(value)).toBe(refused);
  }
});

test("an operator-set run key tokens-per-minute limit can be returned to the deployment default", async () => {
  stubRunSettings({ body: RUN_SETTINGS, status: 200 }, OPERATOR_RATE);
  await mountAt("/admin/settings");
  await waitFor(has("理由：provider throttling"));

  await type("#admin-settings-run-tpm-clear-note", "throttling over");
  await click(button("回到部署預設每分鐘 200,000 tokens"));
  await waitFor(has("已回到部署預設每分鐘 200,000 tokens，之後發出的金鑰就照這個值。"));

  expect(calls.find((c) => c.method === "DELETE")).toEqual({
    method: "DELETE",
    url: "/admin/settings/runs/tokens-per-minute",
    body: { note: "throttling over" },
  });
});

test("an operator-set run spend ceiling can be returned to the deployment default", async () => {
  stubRunSettings({ body: RUN_SETTINGS, status: 200 }, OPERATOR_BUDGET);
  await mountAt("/admin/settings");
  await waitFor(has("理由：smaller runs while we measure"));

  await type("#admin-settings-run-budget-clear-note", "measurement finished");
  await click(button("回到部署預設 650 點"));
  await waitFor(has("已回到部署預設 650 點，新建立的試跑就照這個上限。"));

  expect(calls.find((c) => c.method === "DELETE")).toEqual({
    method: "DELETE",
    url: "/admin/settings/runs/model-budget",
    body: { note: "measurement finished" },
  });
  expect(has("目前：每次試跑最多花 650 點的模型費用（部署預設）")()).toBe(true);
});

test("the answered-rounds setting shows the current value and its range, and sends the new value with its reason", async () => {
  stubRunSettings({
    body: {
      continuation_rounds: {
        rounds: 5,
        min: 1,
        max: 10,
        reason: "longer interviews",
        set_at: "2026-10-10T08:00:00Z",
      },
      model_budget: ADMIN_RUN_SETTINGS.model_budget,
      tokens_per_minute: ADMIN_RUN_SETTINGS.tokens_per_minute,
    },
    status: 200,
  });
  await mountAt("/admin/settings");
  await waitFor(has("一段對話可以回答幾輪"));
  expect(has("目前：3 輪（可設 1–10 輪）")()).toBe(true);
  expect(field<HTMLInputElement>("#admin-settings-rounds-value").value).toBe("3");

  await type("#admin-settings-rounds-value", "5");
  await type("#admin-settings-rounds-note", "longer interviews");
  await click(button("改輪數"));
  await waitFor(has("已改為 5 輪，下一次回答就照這個值。"));

  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: "/admin/settings/runs/continuation-rounds",
    body: { rounds: 5, note: "longer interviews" },
  });
  expect(has("目前：5 輪（可設 1–10 輪）")()).toBe(true);
  expect(has("理由：longer interviews")()).toBe(true);
});

test("the answered-rounds input accepts the range edges and refuses one past them", async () => {
  stubRunSettings({ body: RUN_SETTINGS, status: 200 });
  await mountAt("/admin/settings");
  await waitFor(has("一段對話可以回答幾輪"));
  await type("#admin-settings-rounds-note", "boundary check");

  for (const [value, refused] of [
    ["0", true],
    ["1", false],
    ["10", false],
    ["11", true],
    ["2.5", true],
    ["", true],
    ["abc", true],
  ] as const) {
    await type("#admin-settings-rounds-value", value);
    expect(button("改輪數").disabled, JSON.stringify(value)).toBe(refused);
  }
});

test("a refused answered-rounds change shows the server's sentence and keeps the old value", async () => {
  stubRunSettings({ body: { error: "輪數要在 1 到 10 之間。" }, status: 400 });
  await mountAt("/admin/settings");
  await waitFor(has("一段對話可以回答幾輪"));

  await type("#admin-settings-rounds-value", "4");
  await type("#admin-settings-rounds-note", "try");
  await click(button("改輪數"));
  await waitFor(has("操作未被接受：輪數要在 1 到 10 之間。"));

  expect(has("目前：3 輪（可設 1–10 輪）")()).toBe(true);
  expect(has("已改為")()).toBe(false);
});

test("an unreadable answered-rounds setting says so instead of drawing a form", async () => {
  stub(true, (path) =>
    path === "/admin/settings/runs" ? { body: { error: "not found" }, status: 404 } : undefined,
  );
  await mountAt("/admin/settings");
  await waitFor(has("暫時無法讀取試跑設定"));
  expect(container.querySelector("#admin-settings-rounds-value")).toBeNull();
});

test.each([
  [
    "with a cap",
    "300",
    { count: 3, round_credit_cap: 300, note: "compare three" },
    "每輪最多 300 點",
  ],
  ["with the cap left empty", "", { count: 3, note: "compare three" }, "每輪點數上限尚未設定"],
])(
  "saving the reflection candidates %s sends exactly what was entered and shows the saved state",
  async (_name, cap, sent, shown) => {
    stub(true, (path, method) =>
      path === "/admin/settings/reflection-candidates" && method === "PUT"
        ? {
            body: {
              judge_panel: { enabled: false },
              injection_check: { enabled: false },
              reflection_candidates: {
                count: 3,
                ...(cap === "" ? {} : { round_credit_cap: 300 }),
                reason: "compare three",
                set_at: "2026-11-07T08:00:00Z",
              },
            },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/settings");
    await waitFor(has("每輪 1 個候選；每輪點數上限尚未設定"));
    await type("#admin-settings-reflection-candidates-note", "compare three");
    for (const [count, capValue] of [
      ["0", ""],
      ["5", ""],
      ["1.5", ""],
      ["2", "0"],
      ["2", "2147483648"],
    ]) {
      await type("#admin-settings-reflection-candidates-count", count);
      await type("#admin-settings-reflection-candidates-cap", capValue);
      expect(button("儲存候選數與上限").disabled, `${count} / ${capValue}`).toBe(true);
    }
    await type("#admin-settings-reflection-candidates-count", "3");
    await type("#admin-settings-reflection-candidates-cap", cap);
    await click(button("儲存候選數與上限"));
    await waitFor(has("已儲存，下一輪找改進會問 3 個候選。"));
    expect(calls.find((c) => c.method === "PUT")).toEqual({
      method: "PUT",
      url: "/admin/settings/reflection-candidates",
      body: sent,
    });
    expect(has(`每輪 3 個候選；${shown}`)()).toBe(true);
    expect(has("理由：compare three")()).toBe(true);
  },
);
