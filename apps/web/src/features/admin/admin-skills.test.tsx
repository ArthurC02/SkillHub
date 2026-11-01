import { expect, test } from "vitest";
import { ADMIN_SKILLS, SKILL } from "../../testing/fixtures/platform";
import {
  container,
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

test("OPS-004: takedown of the one skill found takes a reason and a second click", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  expect(calls.some((c) => c.url === `/admin/skills?q=${SKILL}`)).toBe(true);
  expect(has("填了理由才能下架。")()).toBe(true);
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "下架"),
  ).toBe(false);

  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", " DMCA notice ");
  await click(button("下架"));
  expect(container.querySelector("#admin-takedown-scope")?.textContent).toContain(
    "審查結束後可以在這裡恢復上架",
  );
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
  await click(button("確認下架"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/skills/${SKILL}/takedown`,
    body: { reason: "DMCA notice" },
  });
});

test("a completed takedown stays confirmed after its controls disappear and a later read fails", async () => {
  const takenDown = {
    ...ADMIN_SKILLS.skills[0],
    takedown_at: "2026-09-11T10:00:00Z",
    takedown_reason: "DMCA notice",
    deleted_at: null,
  };
  let reads = 0;
  stub(true, (path, method) => {
    if (path === `/admin/skills/${SKILL}/takedown` && method === "PUT") {
      return { body: { skill_id: SKILL, taken_down: true }, status: 200 };
    }
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return reads === 3
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { skills: [reads === 1 ? ADMIN_SKILLS.skills[0] : takenDown] }, status: 200 };
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "DMCA notice");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("已下架"));
  expect(has("「PDF Summariser」已下架。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-takedown-result"));
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();

  await click(button("重新整理治理狀態"));
  await waitFor(has("暫時無法讀取小工具"));
  expect(has("「PDF Summariser」已下架。")()).toBe(true);
  expect(has("已下架")()).toBe(true);
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();
});

test("OPS-004: an operator lifts a takedown with a note and the skill reads as listed again", async () => {
  let lifted = false;
  stub(true, (path, method) => {
    if (path === `/admin/skills/${SKILL}/takedown` && method === "DELETE") {
      lifted = true;
      return { body: null, status: 204 };
    }
    return path === "/admin/skills"
      ? {
          body: {
            skills: [
              lifted
                ? ADMIN_SKILLS.skills[0]
                : {
                    ...ADMIN_SKILLS.skills[0],
                    takedown_at: "2026-09-11T10:00:00Z",
                    takedown_reason: "DMCA notice",
                    deleted_at: null,
                  },
            ],
          },
          status: 200,
        }
      : undefined;
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("恢復上架"));
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();
  expect(has("後台目前不提供恢復")()).toBe(false);

  await click(field<HTMLElement>("#admin-skill-lift-takedown summary"));
  await type("#admin-lift-takedown-note", " counter-notice accepted ");
  await click(button("恢復上架"));
  await waitFor(has("「PDF Summariser」已恢復上架。"));
  expect(calls.find((c) => c.method === "DELETE")).toEqual({
    method: "DELETE",
    url: `/admin/skills/${SKILL}/takedown`,
    body: { note: "counter-notice accepted" },
  });
  await waitFor(has("未下架"));
  expect(container.querySelector("#admin-skill-lift-takedown")).toBeNull();
});

test("a successful takedown remains identified when the first refresh fails", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === `/admin/skills/${SKILL}/takedown` && method === "PUT") {
      return { body: { skill_id: SKILL, taken_down: true }, status: 200 };
    }
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return reads === 1
      ? { body: ADMIN_SKILLS, status: 200 }
      : { body: { error: "service unavailable" }, status: 503 };
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "DMCA notice");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("暫時無法讀取小工具"));
  expect(has("「PDF Summariser」已下架。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-takedown-result"));
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();
  expect(reads).toBe(2);
});

test("a new governance query does not inherit the previous takedown result", async () => {
  let takenDown = false;
  stub(true, (path, method) => {
    if (path === `/admin/skills/${SKILL}/takedown` && method === "PUT") {
      takenDown = true;
      return { body: { skill_id: SKILL, taken_down: true }, status: 200 };
    }
    return path === "/admin/skills"
      ? {
          body: {
            skills: [
              takenDown
                ? {
                    ...ADMIN_SKILLS.skills[0],
                    takedown_at: "2026-09-11T10:00:00Z",
                    takedown_reason: "DMCA notice",
                    deleted_at: null,
                  }
                : ADMIN_SKILLS.skills[0],
            ],
          },
          status: 200,
        }
      : undefined;
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "DMCA notice");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("「PDF Summariser」已下架。"));

  await go("/admin/skills", { q: "PDF" });
  await waitFor(() => field<HTMLInputElement>("#admin-skill-q").value === "PDF");
  await waitFor(has("下架於"));
  expect(has("「PDF Summariser」已下架。")()).toBe(false);
});

test("OPS-004: governance search states how many skills were found", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("查到 1 筆小工具。"));
});

test("OPS-004: governance choices stay scannable without hiding takedown scope", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  const choices = Array.from(
    container.querySelectorAll<HTMLDetailsElement>("details[id^='admin-skill-']"),
  );
  expect(choices.map((choice) => choice.id)).toEqual([
    "admin-skill-restriction",
    "admin-skill-redistribution",
    "admin-skill-takedown",
  ]);
  expect(choices.every((choice) => !choice.open)).toBe(true);
  const scope = Array.from(container.querySelectorAll("p")).find((item) =>
    item.textContent?.startsWith("下架後這個小工具從目錄與搜尋消失"),
  );
  expect(scope?.closest("details")).toBeNull();
  expect(scope?.textContent).toContain("審查結束後可以在這裡恢復上架");

  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  expect(choices[1].open).toBe(true);
  expect(choices[0].open).toBe(false);
  expect(choices[2].open).toBe(false);
});

test("OPS-004: governance status precedes machine identifiers", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  const row = field<HTMLLIElement>("li.download-item");
  expect(row.querySelector(".badge-row")?.nextElementSibling?.textContent ?? "").toContain(
    "工作區",
  );
});

test("OPS-004: changing a takedown reason requires a new confirmation", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "old reason");
  await click(button("下架"));
  expect(button("確認下架")).toBeDefined();

  await type("#admin-takedown-reason", "revised reason");
  expect(container.querySelector("#admin-takedown-scope")).toBeNull();
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(() => calls.some((call) => call.url === `/admin/skills/${SKILL}/takedown`));
  expect(calls.find((call) => call.url === `/admin/skills/${SKILL}/takedown`)?.body).toEqual({
    reason: "revised reason",
  });
});

test("OPS-004: a new takedown draft clears the previous governance success", async () => {
  stub(true, (path, method) =>
    path === `/admin/skills/${SKILL}/redistribution` && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "distribution blocked");
  await type("#admin-redistribution-value", "blocked");
  await click(button("送出判定"));
  await waitFor(has("「PDF Summariser」的再散布判定已改為「禁止再散布」。"));

  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "New evidence");
  expect(has("「PDF Summariser」的再散布判定已改為「禁止再散布」。")()).toBe(false);
  expect(field<HTMLInputElement>("#admin-takedown-reason").value).toBe("New evidence");
  expect(calls.some((call) => call.url.endsWith("/takedown"))).toBe(false);
});

test("OPS-004: editing a rejected takedown clears the old failure", async () => {
  stub(true, (path, method) =>
    path === `/admin/skills/${SKILL}/takedown` && method === "PUT"
      ? { body: { error: "review required" }, status: 409 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "Old evidence");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("操作未被接受；目前狀態可能已變更，請重新整理後再判斷。"));
  expect(has("review required")()).toBe(false);

  await type("#admin-takedown-reason", "New evidence");
  expect(has("操作未被接受；目前狀態可能已變更，請重新整理後再判斷。")()).toBe(false);
  expect(container.querySelector("#admin-takedown-scope")).toBeNull();
});

test("OPS-004: a failed governance refresh hides stale state and actions", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return reads === 1
      ? { body: ADMIN_SKILLS, status: 200 }
      : { body: { error: "service unavailable" }, status: 503 };
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(button("重新整理治理狀態"));
  await waitFor(has("暫時無法讀取小工具"));
  expect(reads).toBe(2);
  expect(has("對「PDF Summariser」的動作")()).toBe(false);
  expect(has("沒有符合")()).toBe(false);
});

test("OPS-004: a different skill returned by refresh starts with empty drafts", async () => {
  const other = {
    ...ADMIN_SKILLS.skills[0],
    skill_id: "cccccccc-4444-4444-4444-444444444444",
    name: "Other Tool",
  };
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return { body: { skills: [reads === 1 ? ADMIN_SKILLS.skills[0] : other] }, status: 200 };
  });
  await mountAt("/admin/skills", { q: "tool" });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "first skill evidence");
  await type("#admin-redistribution-value", "allowed");
  await type("#admin-license-expression", "MIT");
  await type("#admin-license-source", "manifest");
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "first skill takedown");
  await click(button("下架"));

  await click(button("重新整理治理狀態"));
  await waitFor(has("對「Other Tool」的動作"));
  expect(field<HTMLSelectElement>("#admin-redistribution-value").value).toBe("");
  expect(field<HTMLTextAreaElement>("#admin-redistribution-note").value).toBe("");
  expect(container.querySelector("#admin-license-expression")).toBeNull();
  expect(field<HTMLInputElement>("#admin-takedown-reason").value).toBe("");
  expect(container.querySelector("#admin-takedown-scope")).toBeNull();
});

test("OPS-004: the search field follows the skill selected in the address", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: "pdf" });
  await waitFor(has("對「PDF Summariser」的動作"));
  await go("/admin/skills", { q: SKILL });
  await waitFor(() => field<HTMLInputElement>("#admin-skill-q").value === SKILL);
  expect(new URLSearchParams(window.location.search).get("q")).toBe(SKILL);
});

test("OPS-004: editing a governance query hides the prior skill and actions until search", async () => {
  const other = {
    ...ADMIN_SKILLS.skills[0],
    skill_id: "cccccccc-4444-4444-4444-444444444444",
    name: "Other Tool",
    workspace_id: "f3333333-3333-4333-8333-333333333333",
  };
  stub(true, (path, _method, url) =>
    path === "/admin/skills" && url.includes("q=other")
      ? { body: { skills: [other] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  await type("#admin-skill-q", "other");
  expect(has("PDF Summariser")()).toBe(false);
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();
  expect(has("查詢條件已變更；按「查詢」載入新小工具。")()).toBe(true);
  expect(calls.some((call) => call.url === "/admin/skills?q=other")).toBe(false);

  await type("#admin-skill-q", SKILL);
  await waitFor(has("對「PDF Summariser」的動作"));
  await type("#admin-skill-q", "other");
  await submit("#admin-skill-q");
  await waitFor(has("對「Other Tool」的動作"));
  expect(has("PDF Summariser")()).toBe(false);
  expect(new URLSearchParams(window.location.search).get("q")).toBe("other");
});

test.each(["unknown", "blocked", "allowed"] as const)(
  "OPS-004: a %s redistribution state does not preselect the operator's next decision",
  async (redistribution) => {
    stub(true, (path) =>
      path === "/admin/skills"
        ? {
            body: {
              skills: [{ ...ADMIN_SKILLS.skills[0], redistribution }],
            },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/skills", { q: SKILL });
    await waitFor(has("再散布判定"));
    await click(field<HTMLElement>("#admin-skill-redistribution summary"));
    expect(field<HTMLSelectElement>("#admin-redistribution-value").value).toBe("");
    await type("#admin-redistribution-note", "legal review");
    expect(button("送出判定").disabled).toBe(true);
    expect(calls.some((call) => call.url.endsWith("/redistribution"))).toBe(false);
    await type("#admin-redistribution-value", "blocked");
    expect(button("送出判定").disabled).toBe(false);
  },
);

test.each([
  ["generated", false],
  ["self_supplied", true],
] as const)(
  "OPS-004: a %s skill offers a redistribution verdict only when the platform would accept one (%s)",
  async (redistribution, offered) => {
    stub(true, (path) =>
      path === "/admin/skills"
        ? { body: { skills: [{ ...ADMIN_SKILLS.skills[0], redistribution }] }, status: 200 }
        : undefined,
    );
    await mountAt("/admin/skills", { q: SKILL });
    await waitFor(has("再散布判定"));

    expect(document.querySelector("#admin-skill-redistribution") !== null).toBe(offered);
    expect(has("「生成」是它來源的唯一紀錄，平台不接受改寫成其他判定")()).toBe(!offered);
  },
);

test("OPS-004: releasing a skill needs licence evidence; blocking it does not", async () => {
  stub(true, (path, method) =>
    path === `/admin/skills/${SKILL}/redistribution` && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("再散布判定"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "legal cleared");
  expect(button("送出判定").disabled).toBe(true);
  await type("#admin-redistribution-value", "blocked");
  expect(button("送出判定").disabled).toBe(false);

  await type("#admin-redistribution-value", "allowed");
  expect(button("送出判定").disabled).toBe(true);
  await type("#admin-license-expression", "MIT");
  expect(button("送出判定").disabled).toBe(true);
  await type("#admin-license-source", "package-license-file");
  expect(button("送出判定").disabled).toBe(false);
  await click(button("送出判定"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/skills/${SKILL}/redistribution`,
    body: {
      value: "allowed",
      note: "legal cleared",
      license_expression: "MIT",
      license_source: "package-license-file",
    },
  });
  await waitFor(has("「PDF Summariser」的再散布判定已改為「可以再散布」。"));
  expect(document.activeElement?.id).toBe("admin-governance-result");
  await type("#admin-license-expression", "Apache-2.0");
  expect(has("「PDF Summariser」的再散布判定已改為「可以再散布」。")()).toBe(false);
});

test("OPS-004: a restriction is set with the known reason code and lifted by the same form", async () => {
  let restricted: string | null = null;
  stub(true, (path, method) => {
    if (path === "/admin/skills")
      return {
        body: { skills: [{ ...ADMIN_SKILLS.skills[0], access_restriction: restricted }] },
        status: 200,
      };
    if (path.endsWith("/restriction")) {
      restricted = method === "PUT" ? "license-review" : null;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("設定受限展示"));
  await click(field<HTMLElement>("#admin-skill-restriction summary"));
  await type("#admin-restriction-note", "terms under review");
  await click(button("設定受限"));
  await waitFor(has("受限展示：授權審查中"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({
    reason: "license-review",
    note: "terms under review",
  });

  await waitFor(has("解除受限展示"));
  await waitFor(has("「PDF Summariser」已設定受限展示。"));
  expect(document.activeElement?.id).toBe("admin-governance-result");
  expect(field<HTMLTextAreaElement>("#admin-restriction-note").value).toBe("");
  expect(button("解除受限").disabled).toBe(true);
  await type("#admin-restriction-note", "cleared");
  expect(has("「PDF Summariser」已設定受限展示。")()).toBe(false);
  await click(button("解除受限"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({ note: "cleared" });
  await waitFor(has("沒有受限"));
  expect(has("受限展示：")()).toBe(false);
  expect(has("設定受限展示")()).toBe(true);
  await waitFor(has("「PDF Summariser」已解除受限展示。"));
  expect(field<HTMLTextAreaElement>("#admin-restriction-note").value).toBe("");
});

test("OPS-004: a confirmed redistribution remains visible when refreshing governance fails", async () => {
  let changed = false;
  stub(true, (path, method) => {
    if (path === "/admin/skills" && method === "GET")
      return changed
        ? { body: { error: "read unavailable" }, status: 503 }
        : { body: ADMIN_SKILLS, status: 200 };
    if (path === `/admin/skills/${SKILL}/redistribution` && method === "PUT") {
      changed = true;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("再散布判定"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "distribution blocked");
  await type("#admin-redistribution-value", "blocked");
  await click(button("送出判定"));

  await waitFor(has("「PDF Summariser」的再散布判定已改為「禁止再散布」。"));
  await waitFor(() => field<HTMLElement>("#admin-governance-result") === document.activeElement);
  await waitFor(() => container.querySelector("#admin-skill-redistribution") === null);
  expect(has("暫時無法讀取小工具")()).toBe(true);
});

test("an unknown restriction reason stays visible without looking unrestricted", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [{ ...ADMIN_SKILLS.skills[0], access_restriction: "policy-review" }],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("受限展示：其他原因（policy-review）"));
  expect(has("沒有受限")()).toBe(false);
});

test("OPS-004: a name matching several skills lists a way to pick each and offers no action yet", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [
              ADMIN_SKILLS.skills[0],
              {
                ...ADMIN_SKILLS.skills[0],
                skill_id: "cccccccc-4444-4444-4444-444444444444",
                workspace_id: "f3333333-3333-4333-8333-333333333333",
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: "pdf" });
  await waitFor(has("處理這一個"));
  expect(container.querySelectorAll('a[href*="/admin/skills?q="]')).toHaveLength(2);
  expect(has("的動作")()).toBe(false);
});

test("OPS-004: a taken-down skill shows when and why, and offers only lifting the takedown", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [
              {
                ...ADMIN_SKILLS.skills[0],
                takedown_at: "2026-09-10T00:00:00Z",
                takedown_reason: "DMCA",
                deleted_at: null,
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("理由：DMCA"));
  expect(field<HTMLElement>(".badge-row [role='status']").textContent).toBe("已下架");
  expect(has("重新上架須先完成審查")()).toBe(true);
  expect(
    field<HTMLLIElement>("li.download-item").querySelector(".badge-row")?.nextElementSibling
      ?.textContent,
  ).toContain("理由：DMCA");
  expect(
    Array.from(container.querySelectorAll("details[id^='admin-skill-']")).map((d) => d.id),
  ).toEqual(["admin-skill-lift-takedown"]);
});

test("a skill without a takedown has a visible neutral governance status", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("再散布："));
  const status = field<HTMLElement>(".badge-row [role='status']");
  expect(status.textContent).toBe("未下架");
  expect(status.className).toBe("badge");
  expect(has("擁有者已刪除")()).toBe(false);
});

test("a skill its owner deleted says so and still offers the operator's actions", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [{ ...ADMIN_SKILLS.skills[0], deleted_at: "2026-10-20T00:00:00Z" }],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("擁有者已刪除"));
  expect(has("內容會保留到你解除為止")()).toBe(true);
  expect(has("的動作")()).toBe(true);
});

test("OPS-004: a takedown with no reason on record says it was not recorded", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [
              {
                ...ADMIN_SKILLS.skills[0],
                takedown_at: "2026-09-10T00:00:00Z",
                takedown_reason: null,
                deleted_at: null,
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("理由：未記錄"));
});
