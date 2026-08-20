import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import type { PreflightResponse } from "./api/lab";

// 03:TEST-008/009 gate screen. Same hand-rolled DOM plumbing as disc.test.tsx —
// @testing-library is not a dependency of this app.

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

const SKILL = "11111111-1111-1111-1111-111111111111";
const VERSION = "22222222-2222-2222-2222-222222222222";
const OLDER_VERSION = "44444444-4444-4444-4444-444444444444";
const TEST_CASE = "33333333-3333-3333-3333-333333333333";

// Newest first, the order GET /skills/{id}/versions serves.
const VERSIONS = {
  versions: [
    {
      version_id: VERSION,
      version_number: 2,
      content_hash: "sha256:bb",
      created_at: "2026-08-02T00:00:00Z",
    },
    {
      version_id: OLDER_VERSION,
      version_number: 1,
      content_hash: "sha256:aa",
      created_at: "2026-08-01T00:00:00Z",
    },
  ],
};

function summary(hash: string, files: string[]): PreflightResponse {
  return {
    summary_hash: hash,
    estimated_cost: {
      currency: "USD",
      low: 0.01,
      typical: 0.06,
      high: 0.3,
      basis: "估計值,非報價。來源:M2 基準試跑 45 個 Skill 的閘道實付分布。",
    },
    notes: ["以上任何一項變更都會產生新的摘要,必須重新確認。"],
    summary: {
      skill_version_id: VERSION,
      skill_content_hash: "sha256:abc",
      test_case_id: TEST_CASE,
      datasets: files.map((name, i) => ({
        dataset_id: `d${i}`,
        file_name: name,
        content_type: "text/plain",
        size_bytes: 1024,
        content_hash: `h${i}`,
      })),
      dataset_total_bytes: files.length * 1024,
      scripts: { status: "unavailable", findings: [] },
      tools: ["sandbox filesystem (/work, /out)"],
      mcp_servers: [],
      network: { mode: "default_deny", allow: [] },
      injected_secrets: ["ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"],
      provider: { name: "unassigned", rootless: false },
      resource_limits: {
        vcpu: 2,
        memory_bytes: 4 << 30,
        disk_bytes: 8 << 30,
        max_pids: 256,
        max_open_files: 1024,
        wall_clock_soft_seconds: 600,
        wall_clock_hard_seconds: 900,
        artifact_total_bytes: 100 << 20,
        artifact_file_bytes: 25 << 20,
        token_budget: { max_input_tokens: 300000, max_output_tokens: 60000 },
      },
    },
  };
}

/**
 * A fake platform whose permissions change under the page: the first summary
 * lists one file, and after `changePermissions()` it lists two and refuses the
 * old hash with 422 — which is exactly what the server does.
 */
function stubPlatform() {
  const calls: { url: string; body?: string }[] = [];
  let current = summary("hash-one", ["rows.csv"]);
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );

  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, body: init?.body as string | undefined });
    if (url.endsWith("/versions")) return json(VERSIONS);
    if (url.includes("/runs/preflight/confirm")) {
      const sent = JSON.parse(String(init?.body)) as { summary_hash: string };
      return sent.summary_hash === current.summary_hash
        ? json({ confirmed: true, summary_hash: sent.summary_hash }, 201)
        : json({ error: "summary_hash does not match the current permission summary" }, 422);
    }
    if (url.includes("/runs/preflight")) return json(current);
    if (url.includes("/runs")) {
      const sent = JSON.parse(String(init?.body)) as { confirmed_summary_hash: string };
      return sent.confirmed_summary_hash === current.summary_hash
        ? json({ run_id: "run-1", status: "queued", provider: "unassigned" }, 201)
        : json({ error: "the permissions changed since it was confirmed" }, 422);
    }
    return json({ error: "not found" }, 404);
  });

  return {
    calls,
    changePermissions() {
      current = summary("hash-two", ["rows.csv", "extra.csv"]);
    },
  };
}

async function renderLab(
  search: {
    skill: string | undefined;
    version: string | undefined;
    test_case: string | undefined;
  } = {
    skill: SKILL,
    version: VERSION,
    test_case: TEST_CASE,
  },
) {
  const params = new URLSearchParams(
    Object.entries(search).filter(([, v]) => v !== undefined) as [string, string][],
  );
  window.history.pushState({}, "", `/lab/run?${params.toString()}`);
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
  await act(async () => {
    await router.navigate({ to: "/lab/run", search });
  });
}

async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

function versionSelect(): HTMLSelectElement {
  const select = container.querySelector<HTMLSelectElement>("select");
  if (!select) throw new Error(`no version picker; DOM was:\n${container.textContent}`);
  return select;
}

async function pickVersion(versionId: string) {
  const select = versionSelect();
  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(select, versionId);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function confirmButton(): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll("button")).find((b) =>
    b.textContent?.includes("開始 Run"),
  );
  if (!button) throw new Error(`no confirm button; DOM was:\n${container.textContent}`);
  return button;
}

test("02:TEST-005 the summary discloses every required item before the run starts", async () => {
  stubPlatform();
  await renderLab();

  const text = container.textContent ?? "";
  for (const label of [
    "Dataset",
    "Script",
    "工具",
    "MCP Server",
    "網路",
    "Secrets",
    "Provider",
    "資源上限",
  ]) {
    expect(text).toContain(label);
  }
  expect(text).toContain("rows.csv");
  // MVP has no MCP: shown as an explicit 無 rather than left out.
  expect(text).toContain("MCP Server");
  expect(text).toContain("無");
  // A secret is named, never valued.
  expect(text).toContain("ANTHROPIC_AUTH_TOKEN");
  expect(text).not.toContain("sk-");
});

// 設計 §1 原則 3: every field inside summary_hash has to be readable, or the
// user is re-confirming something they have never seen. The four resource
// limits and the provider details below the fold are in the hash, so they are
// on the page — collapsed, but present and reachable.
test("02:TEST-005 the fields inside summary_hash are all on screen, the quiet ones behind a disclosure", async () => {
  stubPlatform();
  await renderLab();

  const details = container.querySelector("details");
  expect(details, `no disclosure; DOM was:\n${container.textContent}`).not.toBeNull();
  const text = details?.textContent ?? "";
  expect(text).toContain("256"); // max_pids
  expect(text).toContain("1024"); // max_open_files
  expect(text).toContain("100.0 MB"); // artifact_total_bytes
  expect(text).toContain("25.0 MB"); // artifact_file_bytes
  expect(text).toContain("600 秒"); // wall_clock_soft_seconds
  // provider.rootless is false here and says so; runtime is absent and is
  // 未回報 rather than an empty gap that reads as "none".
  expect(text).toContain("rootless：否");
  expect(text).toContain("未回報");
});

// PDM-005 §5.3/§5.2a-6. Both ends of the range have to be on screen, and the word
// "估計" has to be too — it sits outside summary_hash, so it must not read like
// one of the facts the user is agreeing to.
test("PDM-005 §5.3 the pre-run screen shows an estimated cost range, labelled as an estimate", async () => {
  stubPlatform();
  await renderLab();

  const text = container.textContent ?? "";
  expect(text).toContain("預估成本");
  expect(text).toContain("估計值");
  expect(text).toContain("$0.01");
  expect(text).toContain("$0.30");
});

test("02:TEST-005 confirming sends the hash that was shown, then starts the run", async () => {
  const platform = stubPlatform();
  await renderLab();

  await act(async () => confirmButton().click());

  const confirm = platform.calls.find((c) => c.url.includes("/preflight/confirm"));
  expect(confirm?.body).toContain("hash-one");
  const started = platform.calls.find((c) => c.url.endsWith("/runs"));
  expect(started?.body).toContain("hash-one");
  expect(container.textContent).toContain("run-1");
});

test("02:TEST-005 a permission change forces a fresh confirmation instead of reusing the old one", async () => {
  const platform = stubPlatform();
  await renderLab();

  // The dataset changes after the page was rendered — the summary on screen is
  // now stale, and the confirmation built from it must not be accepted.
  platform.changePermissions();

  await act(async () => confirmButton().click());
  expect(container.textContent).toContain("權限內容已變更");
  expect(platform.calls.some((c) => c.url.endsWith("/runs"))).toBe(false);
  // The page re-read the summary, so the second file is now on screen.
  expect(container.textContent).toContain("extra.csv");

  // Confirming the new summary sends the new hash, never the old one.
  await act(async () => confirmButton().click());
  const sent = platform.calls.filter((c) => c.url.endsWith("/runs")).map((c) => c.body ?? "");
  expect(sent).toHaveLength(1);
  expect(sent[0]).toContain("hash-two");
  expect(container.textContent).toContain("run-1");
});

// --- 04 丙-14: the version picker -------------------------------------------

test("04 丙-14 the version comes from a picker, and a ?version= link is what it opens on", async () => {
  const platform = stubPlatform();
  await renderLab();

  // The URL named a version, so that is the selection — not the first row of
  // the list, and not the newest. An existing link must keep meaning what it
  // said (there are two versions here, so "the default happens to be right"
  // cannot be what passes this).
  expect(versionSelect().value).toBe(VERSION);
  expect(container.textContent).toContain("v2（最新）");
  expect(container.textContent).toContain("v1");

  // Picking the older one re-reads the permission summary for that version:
  // 02:TEST-005's summary is per-version, so the screen must not keep showing
  // the previous one.
  await pickVersion(OLDER_VERSION);
  await waitFor(() =>
    platform.calls.some(
      (c) => c.url.includes("/runs/preflight") && c.url.includes(`version_id=${OLDER_VERSION}`),
    ),
  );
  expect(versionSelect().value).toBe(OLDER_VERSION);
});

test("04 丙-14 with no version in the URL the page asks for one instead of demanding an id", async () => {
  const platform = stubPlatform();
  await renderLab({ skill: SKILL, version: undefined, test_case: TEST_CASE });

  // The old copy told the reader to supply a version id by hand; there is now a
  // list, and nothing is read until something is chosen.
  await waitFor(() => (container.textContent ?? "").includes("請先在上面選一個 Skill Version"));
  // Asserted as "never asked for an empty version" rather than "never asked":
  // the router is a module singleton these tests share, so a request left over
  // from another case's location proves nothing either way. What must not exist
  // is a preflight read for no version at all.
  expect(platform.calls.some((c) => c.url.includes("version_id=&"))).toBe(false);

  await pickVersion(VERSION);
  await waitFor(() => (container.textContent ?? "").includes("資源上限"));
  expect(container.textContent).toContain("rows.csv");
});
