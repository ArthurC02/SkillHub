import { act } from "react";
import { expect, test, vi } from "vitest";
import * as contractLimits from "@skillhub/api-client-ts/limits";
import { queryClient } from "../../core/api/queryClient";
import { ADMIN_AUDIT_LOG, ADMIN_DISPATCH, ADMIN_ROSTERS } from "../../testing/fixtures/platform";
import {
  type Reply,
  container,
  root,
  calls,
  stub,
  mountAt,
  go,
  waitFor,
  has,
  field,
  type,
  button,
  click,
  submit,
} from "./testing/harness";

for (const entry of ["/admin", "/admin/dispatch"]) {
  test(`${entry} keeps malformed dispatch observations unknown and recovers without a write`, async () => {
    let current: unknown = { dispatching: "false", halts: [] };
    stub(true, (path, method) =>
      path === "/admin/dispatch" && method === "GET" ? { body: current, status: 200 } : undefined,
    );
    await mountAt(entry);
    const failure = entry === "/admin" ? "1 項狀態無法取得" : "暫時無法讀取派送狀態";
    const refresh = entry === "/admin" ? "重新整理狀態" : "重新整理派送狀態";
    await waitFor(has(failure));
    const signal = () =>
      container.querySelector<HTMLElement>(
        entry === "/admin"
          ? '.admin-home-priorities a[href="/admin/dispatch"] strong'
          : 'section[aria-labelledby="admin-dispatch-state"] [role="status"]',
      );
    expect(signal()?.textContent ?? "").not.toContain("正在派送");
    expect(container.querySelector("#admin-halt-lift-target")).toBeNull();
    current = { dispatching: true, halts: [] };
    await click(button(refresh));
    await waitFor(() => signal()?.textContent?.includes("正在派送") === true);
    expect(has(failure)()).toBe(false);
    current = { dispatching: true, halts: [ADMIN_DISPATCH.halts[0], { target: "node-3" }] };
    await click(button(refresh));
    await waitFor(has(failure));
    expect(signal()?.textContent ?? "").not.toContain("正在派送");
    expect(signal()?.textContent ?? "").not.toContain("部分節點");
    expect(container.querySelector("#admin-halt-lift-target")).toBeNull();
    current = { dispatching: false, halts: [] };
    await click(button(refresh));
    await waitFor(() => signal()?.textContent?.includes("停止派送") === true);
    expect(has(failure)()).toBe(false);
    expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
  });
}

for (const outcome of ["503", "malformed"]) {
  for (const target of ["pool", "node-2"]) {
    test(`dispatch lift ${target} retains its unsent draft through ${outcome} and requires deliberate confirmation`, async () => {
      stub(true, (path, method) =>
        path === "/admin/dispatch" && method === "GET"
          ? {
              status: 200,
              body: {
                ...ADMIN_DISPATCH,
                halts: [
                  ...ADMIN_DISPATCH.halts,
                  {
                    ...ADMIN_DISPATCH.halts[0],
                    target: "node-2",
                    source: "orphan_threshold",
                    reason: "orphan capacity reached",
                    automatic_recovery: true,
                    clear_rounds: 1,
                  },
                ],
              },
            }
          : path === "/admin/dispatch/halt" && method === "DELETE"
            ? { status: 204, body: undefined }
            : undefined,
      );
      const fetchBefore = globalThis.fetch;
      let holdRead = false;
      let finishRead: ((response: Response) => void) | undefined;
      vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
        if (holdRead && String(input).endsWith("/admin/dispatch")) {
          holdRead = false;
          return new Promise<Response>((resolve) => {
            finishRead = resolve;
          });
        }
        return fetchBefore(input, init);
      });
      await mountAt("/admin/dispatch");
      await waitFor(has("sandbox escape suspected on node-2"));
      await type("#admin-halt-lift-target", target);
      await type("#admin-halt-lift-note", "evidence preserved and incident resolved");
      await click(button("恢復派送"));
      const confirm = button("確認恢復派送");
      holdRead = true;
      await click(button("重新整理派送狀態"));
      await waitFor(() => Boolean(finishRead) && confirm.disabled);
      expect(field<HTMLSelectElement>("#admin-halt-lift-target").disabled).toBe(true);
      expect(
        field<HTMLSelectElement>("#admin-halt-lift-target").getAttribute("aria-describedby"),
      ).toBe("admin-halt-lift-why");
      expect(field<HTMLElement>("#admin-halt-lift-why").textContent).toBe(
        "正在重新讀取派送狀態，完成後才能恢復派送。",
      );
      await click(confirm);
      expect(calls.filter((call) => call.method === "DELETE")).toEqual([]);
      await act(async () =>
        finishRead!(
          new Response(
            JSON.stringify(
              outcome === "503"
                ? { error: "service unavailable" }
                : { dispatching: "false", halts: [] },
            ),
            { status: outcome === "503" ? 503 : 200 },
          ),
        ),
      );
      await waitFor(has("暫時無法讀取派送狀態"));
      expect(field<HTMLElement>('section[aria-labelledby="admin-dispatch-lift"]').hidden).toBe(
        true,
      );
      expect(field<HTMLSelectElement>("#admin-halt-lift-target").value).toBe(target);
      expect(field<HTMLTextAreaElement>("#admin-halt-lift-note").value).toBe(
        "evidence preserved and incident resolved",
      );
      expect(confirm.disabled).toBe(true);
      await click(confirm);
      expect(calls.filter((call) => call.method === "DELETE")).toEqual([]);
      await click(button("重新整理派送狀態"));
      await waitFor(
        () =>
          !field<HTMLElement>('section[aria-labelledby="admin-dispatch-lift"]').hidden &&
          !confirm.disabled,
      );
      expect(field<HTMLSelectElement>("#admin-halt-lift-target").value).toBe(target);
      expect(field<HTMLTextAreaElement>("#admin-halt-lift-note").value).toBe(
        "evidence preserved and incident resolved",
      );
      expect(calls.filter((call) => call.method === "DELETE")).toEqual([]);
      await click(confirm);
      await waitFor(has("煞車的請求已完成"));
      expect(calls.filter((call) => call.method === "DELETE")).toHaveLength(1);
      expect(calls.find((call) => call.method === "DELETE")?.body).toEqual(
        target === "pool"
          ? { note: "evidence preserved and incident resolved" }
          : { note: "evidence preserved and incident resolved", provider: "node-2" },
      );
    });
  }
}

for (const [changedField, value] of [
  ["source", "p1_incident"],
  ["reason", "new incident"],
  ["declared_at", "2026-09-11T11:00:00Z"],
] as const) {
  test(`dispatch lift resets its reason and confirmation when recovered ${changedField} changes`, async () => {
    const halt = {
      ...ADMIN_DISPATCH.halts[0],
      source: "orphan_threshold",
      automatic_recovery: true,
      clear_rounds: 1,
    };
    let current: Reply = { status: 200, body: { dispatching: true, halts: [halt] } };
    stub(true, (path) => (path === "/admin/dispatch" ? current : undefined));
    await mountAt("/admin/dispatch");
    await waitFor(has("已連續 1／2 輪"));
    await type("#admin-halt-lift-target", halt.target);
    await type("#admin-halt-lift-note", "capacity cleared");
    await click(button("恢復派送"));
    current = { status: 503, body: { error: "service unavailable" } };
    await click(button("重新整理派送狀態"));
    await waitFor(has("暫時無法讀取派送狀態"));
    current = {
      status: 200,
      body: {
        dispatching: true,
        halts: [{ ...halt, [changedField]: value, automatic_recovery: changedField !== "source" }],
      },
    };
    await click(button("重新整理派送狀態"));
    await waitFor(() => !has("暫時無法讀取派送狀態")());
    expect(field<HTMLSelectElement>("#admin-halt-lift-target").value).toBe(halt.target);
    expect(field<HTMLTextAreaElement>("#admin-halt-lift-note").value).toBe("");
    expect(button("恢復派送").disabled).toBe(true);
    expect(
      Array.from(container.querySelectorAll("button")).some(
        (item) => item.textContent === "確認恢復派送",
      ),
    ).toBe(false);
    expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
  });
}

test("dispatch lift keeps its reason when only recovered clear rounds change", async () => {
  const halt = {
    ...ADMIN_DISPATCH.halts[0],
    source: "orphan_threshold",
    automatic_recovery: true,
    clear_rounds: 1,
  };
  let current: Reply = { status: 200, body: { dispatching: true, halts: [halt] } };
  stub(true, (path) => (path === "/admin/dispatch" ? current : undefined));
  await mountAt("/admin/dispatch");
  await waitFor(has("已連續 1／2 輪"));
  await type("#admin-halt-lift-target", halt.target);
  await type("#admin-halt-lift-note", "operator investigation");
  await click(button("恢復派送"));
  current = { status: 503, body: { error: "service unavailable" } };
  await click(button("重新整理派送狀態"));
  await waitFor(has("暫時無法讀取派送狀態"));
  current = { status: 200, body: { dispatching: true, halts: [{ ...halt, clear_rounds: 0 }] } };
  await click(button("重新整理派送狀態"));
  await waitFor(has("已連續 0／2 輪"));
  expect(field<HTMLSelectElement>("#admin-halt-lift-target").value).toBe(halt.target);
  expect(field<HTMLTextAreaElement>("#admin-halt-lift-note").value).toBe("operator investigation");
  expect(button("確認恢復派送").disabled).toBe(false);
  expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
});

test("dispatch lift resets a prepared reason when selecting another otherwise identical halt", async () => {
  const halt = { ...ADMIN_DISPATCH.halts[0], reason: "shared incident" };
  stub(true, (path) =>
    path === "/admin/dispatch"
      ? { status: 200, body: { dispatching: false, halts: [halt, { ...halt, target: "node-2" }] } }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("shared incident"));
  await type("#admin-halt-lift-target", halt.target);
  await type("#admin-halt-lift-note", "pool verified");
  await click(button("恢復派送"));
  await type("#admin-halt-lift-target", "node-2");
  expect(field<HTMLTextAreaElement>("#admin-halt-lift-note").value).toBe("");
  expect(button("恢復派送").disabled).toBe(true);
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "確認恢復派送",
    ),
  ).toBe(false);
  expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
});

test("OPS-005: the dispatch page names the halt, and a declaration without a node halts the fleet", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch/halt" && method === "PUT"
      ? { body: { note: "整個叢集停止派送。" }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  expect(has("P1 事故：只有人能解除")()).toBe(true);
  expect(has("整個叢集")()).toBe(true);
  expect(
    field<HTMLElement>("#admin-dispatch-lift").compareDocumentPosition(
      field<HTMLElement>("#admin-dispatch-declare"),
    ) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  expect(field<HTMLElement>("#admin-halt-declare-scope").textContent).toBe(
    "本次停止範圍：整個叢集。新的 Run 將無法建立或派送；清理會停下以保留現場。",
  );
  expect(button("停止派送").classList.contains("caution")).toBe(true);
  expect(field<HTMLElement>("#admin-halt-declare-why").textContent).toBe(
    "先填宣告理由；節點名稱可以留空，代表停止整個叢集。",
  );
  expect(button("恢復派送").classList.contains("caution")).toBe(false);
  expect(button("恢復派送").disabled).toBe(true);
  await type("#admin-halt-declare-note", "escape drill");
  await click(button("停止派送"));
  await waitFor(has("整個叢集停止派送。"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({ note: "escape drill" });

  await type("#admin-halt-provider", "node-2");
  expect(field<HTMLElement>("#admin-halt-declare-scope").textContent).toBe(
    "本次停止範圍：節點 node-2。該節點不再接收新 Run；清理會停下以保留現場。",
  );
  expect(has("整個叢集停止派送。")()).toBe(false);
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "cleared");
  await click(button("恢復派送"));
  expect(has("觸發條件已消失")()).toBe(true);
  await click(button("確認恢復派送"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({ note: "cleared" });
});

test("an operator reason past the contract limit keeps the action unsent and says how long it is", async () => {
  stub(true, () => undefined);
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));

  await type("#admin-halt-declare-note", ` ${"字".repeat(1000)}\n`);
  expect(button("停止派送").disabled).toBe(false);

  await type("#admin-halt-declare-note", "字".repeat(1001));
  expect(button("停止派送").disabled).toBe(true);
  expect(field<HTMLElement>("#admin-halt-declare-why").textContent).toBe(
    "理由最多 1000 個字，目前 1001 個字。",
  );
  await click(button("停止派送"));
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
});

test("every operator reason in the contract shares the one limit the action form enforces", () => {
  const reasonLimits = Object.entries(contractLimits).filter(([name]) =>
    /Body(Note|Reason)MaxLength$/.test(name),
  );
  expect(reasonLimits.length).toBeGreaterThan(0);
  expect(new Set(reasonLimits.map(([, value]) => value))).toEqual(new Set([1000]));
});

test("OPS-005: a released halt leaves a focused, verified result after its form disappears", async () => {
  let released = false;
  stub(true, (path, method) => {
    if (path === "/admin/dispatch" && method === "GET") {
      return { body: released ? { dispatching: true, halts: [] } : ADMIN_DISPATCH, status: 200 };
    }
    if (path === "/admin/dispatch/halt" && method === "DELETE") {
      released = true;
      return { body: undefined, status: 204 };
    }
    return undefined;
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));

  await waitFor(has("沒有生效中的煞車"));
  const result = field<HTMLElement>("#admin-dispatch-lift-result");
  expect(result.textContent).toContain("整個叢集煞車的請求已完成");
  expect(result.textContent).toContain("已不在生效中的煞車清單");
  expect(result.classList.contains("notice-success")).toBe(true);
  expect(document.activeElement).toBe(result);
  expect(container.querySelector("#admin-halt-lift-target")).toBeNull();
  expect(container.querySelector("#admin-dispatch-lift")).toBeNull();
});

test("OPS-005: a completed release remains visible when status reread fails, then verifies on retry", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === "/admin/dispatch" && method === "GET") {
      reads += 1;
      return reads === 1
        ? { body: ADMIN_DISPATCH, status: 200 }
        : reads === 2
          ? { body: { error: "service unavailable" }, status: 503 }
          : { body: { dispatching: true, halts: [] }, status: 200 };
    }
    if (path === "/admin/dispatch/halt" && method === "DELETE") {
      return { body: undefined, status: 204 };
    }
    return undefined;
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));

  await waitFor(has("暫時無法讀取派送狀態"));
  const result = field<HTMLElement>("#admin-dispatch-lift-result");
  expect(result.textContent).toContain("暫勿假定已恢復派送");
  expect(result.classList.contains("notice-warning")).toBe(true);
  expect(document.activeElement).toBe(result);
  expect(has("sandbox escape suspected on node-2")()).toBe(false);
  expect(field<HTMLElement>('section[aria-labelledby="admin-dispatch-lift"]').hidden).toBe(true);
  expect(field<HTMLSelectElement>("#admin-halt-lift-target").disabled).toBe(true);

  await click(button("重新整理派送狀態"));
  await waitFor(has("沒有生效中的煞車"));
  expect(result.classList.contains("notice-success")).toBe(true);
  expect(result.textContent).toContain("已不在生效中的煞車清單");
  expect(calls.filter((call) => call.method === "DELETE")).toHaveLength(1);
});

test("OPS-005: an unverified release clears when a new halt is declared", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch/halt"
      ? method === "DELETE"
        ? { body: undefined, status: 204 }
        : { body: { note: "叢集已再次停止派送。" }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));

  await waitFor(has("目前仍列有這個對象的煞車"));
  const result = field<HTMLElement>("#admin-dispatch-lift-result");
  expect(result.classList.contains("notice-warning")).toBe(true);
  expect(result.textContent).not.toContain("已不在生效中的煞車清單");

  await type("#admin-halt-declare-note", "new incident");
  await click(button("停止派送"));
  await waitFor(() => container.querySelector("#admin-dispatch-lift-result") === null);
  expect(container.querySelector("#admin-dispatch-lift-result")).toBeNull();
});

test("OPS-005: a node halt without recovery progress stays unknown while other nodes dispatch", async () => {
  stub(true, (path, method) => {
    if (path === "/admin/dispatch" && method === "GET") {
      return {
        body: {
          dispatching: true,
          halts: [
            {
              target: "node-2",
              source: "orphan_threshold",
              reason: "orphan capacity reached",
              declared_at: "2026-09-11T09:00:00Z",
              automatic_recovery: true,
            },
          ],
        },
        status: 200,
      };
    }
    return path === "/admin/dispatch/halt" && method === "DELETE"
      ? { body: undefined, status: 204 }
      : undefined;
  });
  await mountAt("/admin");
  await waitFor(has("仍在派送；1 個煞車"));
  expect(
    field<HTMLElement>('[aria-label="目前需留意"] a[href="/admin/dispatch"]').getAttribute(
      "data-state",
    ),
  ).toBe("pending");

  await go("/admin/dispatch");
  await waitFor(has("部分節點停止派送"));
  expect(has("其他節點仍可派送")()).toBe(true);
  expect(has("自動解除進度未回報")()).toBe(true);
  expect(has("已連續 0／2 輪低於門檻")()).toBe(false);
  await type("#admin-halt-lift-target", "node-2");
  await type("#admin-halt-lift-note", "capacity cleared and verified");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));
  await waitFor(() => calls.some((call) => call.method === "DELETE"));
  expect(calls.find((call) => call.method === "DELETE")?.body).toEqual({
    note: "capacity cleared and verified",
    provider: "node-2",
  });
});

test("OPS-005: no active halt means there is nothing to release", async () => {
  stub(true, (path) =>
    path === "/admin/dispatch"
      ? { body: { dispatching: true, halts: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("沒有生效中的煞車"));
  expect(container.querySelector("#admin-halt-lift-target")).toBeNull();
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "恢復派送",
    ),
  ).toBe(false);
  expect(button("停止派送")).toBeDefined();
});

test("OPS-005: stopped dispatch with no listed halt is not described as healthy", async () => {
  stub(true, (path) =>
    path === "/admin/dispatch"
      ? { body: { dispatching: false, halts: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("未列出煞車，請確認節點設定與平台狀態"));
  expect(has("停止派送")()).toBe(true);
  expect(has("未列出煞車，請確認節點設定與平台狀態")()).toBe(true);
});

test("OPS-005: an unreadable dispatch state keeps emergency halt available but hides release", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { dispatching: true, halts: [] }, status: 200 };
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("暫時無法讀取派送狀態"));
  expect(button("停止派送")).toBeDefined();
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "恢復派送",
    ),
  ).toBe(false);
  await click(button("重新整理派送狀態"));
  await waitFor(has("沒有生效中的煞車"));
  expect(reads).toBe(2);
});

test("OPS-005: refreshing dispatch status removes a halt that is no longer active", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    reads += 1;
    return reads === 1
      ? {
          body: {
            dispatching: false,
            halts: [
              {
                target: "pool",
                source: "p1_incident",
                reason: "incident",
                declared_at: "2026-09-11T09:00:00Z",
                automatic_recovery: false,
              },
            ],
          },
          status: 200,
        }
      : { body: { dispatching: true, halts: [] }, status: 200 };
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("incident"));
  expect(button("恢復派送")).toBeDefined();

  await click(button("重新整理派送狀態"));
  await waitFor(has("沒有生效中的煞車"));
  expect(reads).toBe(2);
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "恢復派送",
    ),
  ).toBe(false);
});

test("OPS-005: a changed halt requires a new release reason and confirmation", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    reads += 1;
    return {
      body: {
        dispatching: true,
        halts: [
          {
            target: "node-2",
            source: reads === 1 ? "orphan_threshold" : "p1_incident",
            reason: reads === 1 ? "capacity threshold" : "new incident",
            declared_at: reads === 1 ? "2026-09-11T09:00:00Z" : "2026-09-11T10:00:00Z",
            clear_rounds: reads === 1 ? 1 : undefined,
            automatic_recovery: reads === 1,
          },
        ],
      },
      status: 200,
    };
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("capacity threshold"));
  expect(has("已連續 1／2 輪低於門檻")()).toBe(true);
  await type("#admin-halt-lift-target", "node-2");
  await type("#admin-halt-lift-note", "capacity cleared");
  await click(button("恢復派送"));
  expect(button("確認恢復派送")).toBeDefined();

  await click(button("重新整理派送狀態"));
  await waitFor(has("new incident"));
  expect(has("已連續 1／2 輪低於門檻")()).toBe(false);
  expect(field<HTMLTextAreaElement>("#admin-halt-lift-note").value).toBe("");
  expect(button("恢復派送").disabled).toBe(true);
});

test("OPS-005: a pending release prevents a conflicting halt declaration", async () => {
  stub(true);
  const fetchBefore = globalThis.fetch;
  let finishRelease: ((response: Response) => void) | undefined;
  let declarations = 0;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "DELETE") {
      return new Promise<Response>((resolve) => {
        finishRelease = resolve;
      });
    }
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "PUT") {
      declarations += 1;
    }
    return fetchBefore(input, init);
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await type("#admin-halt-declare-note", "new incident");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));
  await waitFor(() => Boolean(finishRelease));
  await waitFor(has("正在解除煞車"));

  expect(button("停止派送").disabled).toBe(true);
  expect(has("正在解除煞車")()).toBe(true);
  await submit("#admin-halt-declare-note");
  expect(declarations).toBe(0);
  await act(async () => finishRelease!(new Response(null, { status: 204 })));
});

test("OPS-005: a pending halt declaration prevents a prepared release confirmation", async () => {
  stub(true);
  const fetchBefore = globalThis.fetch;
  let finishDeclaration: ((response: Response) => void) | undefined;
  let releases = 0;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "PUT") {
      return new Promise<Response>((resolve) => {
        finishDeclaration = resolve;
      });
    }
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "DELETE") {
      releases += 1;
    }
    return fetchBefore(input, init);
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  expect(button("確認恢復派送")).toBeDefined();
  await type("#admin-halt-declare-note", "new incident");
  await click(button("停止派送"));
  await waitFor(() => Boolean(finishDeclaration));
  await waitFor(has("正在停止派送"));

  expect(button("確認恢復派送").disabled).toBe(true);
  expect(field<HTMLSelectElement>("#admin-halt-lift-target").disabled).toBe(true);
  expect(has("正在停止派送")()).toBe(true);
  expect(releases).toBe(0);
  await act(async () =>
    finishDeclaration!(
      new Response(JSON.stringify({ note: "stop recorded" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
});

test("OPS-005: the rosters page is read-only", async () => {
  stub(true);
  await mountAt("/admin/rosters");
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(field("main code").textContent).toBe("11111111-1111-1111-1111-111111111111");
  expect(container.querySelectorAll("main :is(input, textarea, select)")).toHaveLength(0);
  expect(button("重新整理")).toBeDefined();
});

test("OPS-005: unreadable rosters hide cached membership and recover on retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/rosters") return undefined;
    reads += 1;
    return reads === 1 || reads === 3
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_ROSTERS, status: 200 };
  });
  await mountAt("/admin/rosters");
  await waitFor(has("暫時無法讀取名冊"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("每一個登入的帳號都算受邀")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(has("名冊清單上次取得於")()).toBe(true);
  await click(button("重新整理"));
  await waitFor(has("先前載入的名冊已隱藏"));
  expect(has("每一個登入的帳號都算受邀")()).toBe(false);
  expect(has("目前 1 位")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(reads).toBe(4);
});

test("OPS-006: the audit log names actions in words and folds the metadata", async () => {
  stub(true);
  await mountAt("/admin/audit-log");
  await waitFor(has("授予點數"));
  expect(has("查詢帳號")()).toBe(true);
  expect(has("點數分錄")()).toBe(true);
  expect(has("credit_entry")()).toBe(false);
  expect(field<HTMLElement>(".table-scroll").tabIndex).toBe(-1);
  const table = field<HTMLTableElement>("table.responsive-table");
  const labels = ["時間", "動作", "行為者", "對象", "內容"];
  expect(table.caption?.textContent).toBe("全平台動作紀錄，新的在上面");
  expect(has("已載入 2 筆動作紀錄")()).toBe(true);
  expect(Array.from(table.querySelectorAll("thead th")).map((th) => th.textContent)).toEqual(
    labels,
  );
  expect(
    Array.from(table.tBodies[0].rows).map((row) =>
      Array.from(row.children).map((cell) => cell.getAttribute("data-label")),
    ),
  ).toEqual([labels, labels]);
  expect(
    Array.from(table.querySelectorAll('tbody th[scope="row"]')).map((th) => th.textContent),
  ).toEqual(["授予點數", "查詢帳號"]);
  expect(table.tBodies[0].rows[0].querySelector('[data-label="對象"]')?.textContent).toContain(
    "工作區：f2222222-2222-4222-8222-222222222222",
  );
  expect(field("td details summary").textContent).toBe("3 項");
  expect(field("td details").textContent).toContain("beta reward");
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "載入更多"),
  ).toBe(false);
});

test("audit events without a workspace name its absence", async () => {
  stub(true, (path) =>
    path === "/admin/audit-log"
      ? { body: { events: [{ ...ADMIN_AUDIT_LOG.events[0], workspace_id: null }] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(has("授予點數"));
  expect(field<HTMLElement>('[data-label="對象"]').textContent).toContain("工作區：不適用");
});

test("a failed audit refresh hides old rows until retry succeeds", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/audit-log") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_AUDIT_LOG, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 2);
  await click(button("重新整理"));
  await waitFor(has("暫時無法讀取動作紀錄"));
  expect(reads).toBe(2);
  expect(container.querySelector("tbody tr")).toBeNull();

  await click(button("再試一次"));
  await waitFor(() => reads === 3 && container.querySelectorAll("tbody tr").length === 2);
  expect(has("暫時無法讀取動作紀錄")()).toBe(false);
});

test("an initial audit read failure offers retry without claiming prior data", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/audit-log") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_AUDIT_LOG, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(has("暫時無法讀取動作紀錄"));
  expect(has("請稍後再試")()).toBe(true);
  expect(has("先前載入的內容已隱藏")()).toBe(false);
  expect(container.querySelector("table")).toBeNull();

  await click(button("再試一次"));
  await waitFor(() => reads === 2 && container.querySelectorAll("tbody tr").length === 2);
});

test("a lost operator session hides previously loaded audit rows", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/audit-log") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "not authenticated" }, status: 401 }
      : { body: ADMIN_AUDIT_LOG, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 2);
  await click(button("重新整理"));
  await waitFor(has("動作紀錄需要登入"));
  expect(container.querySelector("table")).toBeNull();
  expect(has("授予點數")()).toBe(false);
});

test("OPS-006: a halt the platform declared by itself names the platform as the actor", async () => {
  stub(true, (path) =>
    path.startsWith("/admin/audit-log")
      ? {
          body: {
            events: [
              {
                actor_kind: "system",
                actor_user_id: null,
                actor_agent_id: null,
                action: "dispatch.halted",
                resource_type: "dispatch",
                resource_id: "h-1",
                workspace_id: null,
                occurred_at: "2026-09-10T08:00:00Z",
                metadata: { target: "all", source: "orphan_threshold", reason: "orphans" },
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(has("停止派送"));
  expect(has("平台自動")()).toBe(true);
  expect(has("未測量")()).toBe(false);
});

test("OPS-011: an action one of the platform's agents took names the agent, not the platform", async () => {
  stub(true, (path) =>
    path.startsWith("/admin/audit-log")
      ? {
          body: {
            events: [
              {
                actor_kind: "agent",
                actor_user_id: null,
                actor_agent_id: "agent-7",
                action: "dispatch.halted",
                resource_type: "dispatch",
                resource_id: "h-2",
                workspace_id: null,
                occurred_at: "2026-09-10T08:00:00Z",
                metadata: {},
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(has("停止派送"));
  expect(has("平台 Agent agent-7")()).toBe(true);
  expect(has("平台自動")()).toBe(false);
});

test("OPS-006: a full page of 50 stops, the 51st event offers the next page", async () => {
  let total = 50;
  const event = ADMIN_AUDIT_LOG.events[0];
  stub(true, (path) =>
    path === "/admin/audit-log"
      ? {
          body: {
            events: Array.from({ length: Math.min(total, 50) }, () => event),
            ...(total > 50 ? { next_cursor: "next-50" } : {}),
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "載入更多"),
  ).toBe(false);

  total = 51;
  queryClient.clear();
  await act(async () => root.unmount());
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  expect(button("載入更多")).toBeDefined();
  expect(has("已載入 50 筆動作紀錄；還有更多")()).toBe(true);
  expect(
    calls
      .filter((c) => c.url.startsWith("/admin/audit-log"))
      .every((c) => c.url === "/admin/audit-log?limit=50"),
  ).toBe(true);
});

test("a failed next audit page keeps loaded rows and retries only that page", async () => {
  const event = ADMIN_AUDIT_LOG.events[0];
  const cursor = "2026-09-10T08:00:00Z_50";
  let nextPageReads = 0;
  stub(true, (_path, _method, url) => {
    if (!url.startsWith("/admin/audit-log?")) return undefined;
    if (!new URL(url, "http://localhost").searchParams.has("cursor")) {
      return {
        body: { events: Array.from({ length: 50 }, () => event), next_cursor: cursor },
        status: 200,
      };
    }
    nextPageReads += 1;
    expect(new URL(url, "http://localhost").searchParams.get("cursor")).toBe(cursor);
    return nextPageReads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { events: [ADMIN_AUDIT_LOG.events[1]] }, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  await click(button("載入更多"));
  await waitFor(has("後續紀錄暫時無法讀取"));
  expect(container.querySelectorAll("tbody tr")).toHaveLength(50);
  expect(has("已載入 50 筆動作紀錄；還有更多")()).toBe(true);
  await click(button("重試載入更多"));
  await waitFor(() => container.querySelectorAll("tbody tr").length === 51);
  expect(nextPageReads).toBe(2);
  expect(has("後續紀錄暫時無法讀取")()).toBe(false);
  expect(calls.filter((call) => call.url.includes("/admin/audit-log?"))).toHaveLength(3);
});

test("a lost operator session on the next audit page hides loaded rows", async () => {
  const event = ADMIN_AUDIT_LOG.events[0];
  stub(true, (_path, _method, url) => {
    if (!url.startsWith("/admin/audit-log?")) return undefined;
    return url.includes("cursor=next-50")
      ? { body: { error: "not authenticated" }, status: 401 }
      : {
          body: { events: Array.from({ length: 50 }, () => event), next_cursor: "next-50" },
          status: 200,
        };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  await click(button("載入更多"));
  await waitFor(has("動作紀錄需要登入"));
  expect(container.querySelector("table")).toBeNull();
  expect(has("重試載入更多")()).toBe(false);
});
