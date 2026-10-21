import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { onlineManager, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { PublicPublication } from "./PublicPublication.page";
import { AcquisitionFailure } from "./components/AcquisitionFailure";
import { PublisherSection } from "./components/PublisherSection";
import { PublishPanel } from "./components/PublishPanel";
import { PublishingWorkspace } from "./PublishingWorkspace.page";
import { DeclarableLicense } from "@skillhub/api-client-ts";
import {
  DECLARABLE_LICENSES,
  PUBLISHING_REFUSAL_LABEL,
  type PublishingRefusalReason,
  refusalSentence,
} from "./publishing.model";
import {
  OWN_PUBLICATION,
  OWN_PUBLICATIONS,
  OWN_PUBLISHER,
  PUBLIC_BUNDLE_PUBLICATION,
  PUBLIC_PUBLICATION,
  PUBLICATION,
  PUBLISHER,
  ARTIFACT,
  ARTIFACT_ROW,
  OWN_BUNDLE,
  SKILL,
  SKILL_VERSIONS,
  VERSION,
  skillDetail,
  platformResponse,
} from "../../testing/fixtures/platform";
import type { SkillDetail } from "../../core/api/types";
import type { BundleVersion, Publication } from "./publishing.service";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";

let container: HTMLDivElement;
let root: Root;
let publishingSearch: { artifact?: string; publication?: string; bundleVersion?: string } = {};
let publicParams = { publisher: PUBLISHER, name: PUBLICATION };

beforeEach(() => {
  queryClient.clear();
  publishingSearch = {};
  publicParams = { publisher: PUBLISHER, name: PUBLICATION };
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
  onlineManager.setOnline(true);
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    children,
    target,
    rel,
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    children?: unknown;
    target?: string;
    rel?: string;
  }) => {
    const path = Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to);
    const query = new URLSearchParams();
    Object.entries(search ?? {}).forEach(([key, value]) => {
      if (value !== undefined) query.set(key, value);
    });
    return (
      <a href={`${path}${query.size ? `?${query}` : ""}`} target={target} rel={rel}>
        {children as never}
      </a>
    );
  },
  useParams: () => publicParams,
  useSearch: () => publishingSearch,
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

async function render(node: ReactNode, settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(settled);
}

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const text = () => container.textContent ?? "";
function button(label: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes(label),
  );
}

function stub(routes: Record<string, { body: unknown; status?: number }>) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls.push(url);
    const path = url.replace(/^https?:\/\/[^/]+/, "").split("?")[0];
    const hit = routes[path];
    if (hit) return json(hit.body, hit.status ?? 200);
    return json({ error: "not found" }, 404);
  });
  return calls;
}

const PUB_ADDRESS = `/publications/${PUBLISHER}/${PUBLICATION}`;

for (const target of [
  "body",
  "main",
  "action",
  "header",
  "other control",
  "acknowledged",
] as const) {
  test(`acquisition feedback respects focus in ${target}`, async () => {
    container.id = "main-content";
    container.tabIndex = -1;
    const headerInput = document.createElement("input");
    headerInput.setAttribute("aria-label", "搜尋 Catalog");
    document.body.appendChild(headerInput);
    const view = (error?: Error) => (
      <>
        <input aria-label="其他操作" />
        <div>
          <button type="button">取得套件</button>
          <AcquisitionFailure
            error={error}
            what="這個下載"
            fallback="下載沒有成功，可以再試一次。"
            focusOnError={target !== "acknowledged"}
          />
        </div>
      </>
    );
    try {
      await render(view(), () => Boolean(button("取得套件")));
      const action = button("取得套件")!;
      const other = container.querySelector("input")!;
      if (target === "main") container.focus();
      if (target === "action") action.focus();
      if (target === "header") headerInput.focus();
      if (target === "other control") other.focus();
      const previous = document.activeElement;
      await act(async () => {
        root.render(
          <StrictMode>
            <QueryClientProvider client={queryClient}>
              {view(new Error("publishing failed"))}
            </QueryClientProvider>
          </StrictMode>,
        );
      });
      const alert = container.querySelector('[role="alert"]');
      expect(alert?.textContent).toBe("下載沒有成功，可以再試一次。");
      expect(action.isConnected).toBe(true);
      expect(document.activeElement).toBe(
        target === "body" || target === "main" || target === "action" ? alert : previous,
      );
    } finally {
      headerInput.remove();
    }
  });
}

function bundleOverview(latestVersion: BundleVersion, publication?: Publication) {
  const latestRelease = publication?.releases[0];
  return {
    bundles: [
      {
        bundle: latestVersion.bundle,
        latest_version: latestVersion,
        publication: publication
          ? {
              publisher: publication.publisher,
              name: publication.name,
              address: publication.address,
              status: publication.status,
              status_changed_at: publication.status_changed_at,
              latest_release: latestRelease
                ? {
                    bundle_version: latestRelease.bundle_version,
                    content_hash: latestRelease.content_hash,
                    released_at: latestRelease.released_at,
                  }
                : undefined,
              availability: publication.availability,
              acquisition: publication.acquisition,
            }
          : undefined,
      },
    ],
  };
}

test("the publishing space brings identity, Bundles, and delivery records into one lifecycle", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { publications: [] } },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("目前沒有可顯示的套件"));

  expect(container.querySelector("h1")?.textContent).toBe("發佈與交付");
  const workspaceMap = container.querySelector('nav[aria-label="發佈工作區導覽"]');
  expect(workspaceMap).not.toBeNull();
  expect(
    Array.from(workspaceMap?.querySelectorAll("a") ?? []).map((link) => [
      link.getAttribute("href"),
      link.textContent?.replace(/\s+/g, " ").trim(),
    ]),
  ).toEqual([
    ["#skill-publications", "單一小工具不可變版本 → Publication → Release"],
    ["#bundle-workspace", "Bundle 成員版本 → Bundle Version → Release"],
    ["#delivery-history", "交付 Artifact → 可得條件 → 下載紀錄"],
  ]);
  const sections = Array.from(container.querySelectorAll("h2")).map(
    (heading) => heading.textContent,
  );
  expect(sections).toEqual(
    expect.arrayContaining(["小工具發佈", "Bundle", "交付紀錄", "發佈者名稱", "從單一版本發佈"]),
  );
  expect(text()).toContain("公開位址不等於 Catalog 曝光");
  expect(text()).toContain("取得者身分與下載次數尚未提供");
  expect(container.querySelector('a[href="/library"]')?.textContent).toContain(
    "選擇要發佈的小工具",
  );
});

test("an empty Bundle collection keeps creation available without making the page a form", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { publications: [] } },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });
  await render(<PublishingWorkspace />, () => text().includes("建立第一個 Bundle"));

  const createBundle = Array.from(container.querySelectorAll("details")).find(
    (details) => details.querySelector("summary")?.textContent === "建立第一個 Bundle",
  );
  expect(createBundle?.open).toBe(false);
  expect(createBundle?.querySelector("form.bundle-form")).not.toBeNull();
});

test("an exact Version continuation preselects that immutable Bundle member instead of latest", async () => {
  const olderVersion = SKILL_VERSIONS.versions[1];
  publishingSearch = { bundleVersion: olderVersion.version_id };
  let created: Record<string, unknown> | undefined;
  let complete!: () => void;
  const hold = new Promise<void>((resolve) => {
    complete = resolve;
  });
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me/bundles" && init?.method === "POST") {
      created = JSON.parse(String(init.body)) as Record<string, unknown>;
      return hold.then(
        () => new Response(JSON.stringify({ ...OWN_BUNDLE, version: "1.0.0" }), { status: 201 }),
      );
    }
    const routes: Record<string, { body: unknown; status?: number }> = {
      "/me/publisher": { body: OWN_PUBLISHER },
      "/me/publications": { body: { publications: [] } },
      "/me/bundles": { body: { bundles: [] } },
      "/skills": {
        body: {
          skills: [{ skill_id: SKILL }],
          total: 1,
          limit: 100,
          truncated: false,
        },
      },
      [`/api/skills/${SKILL}`]: { body: skillDetail(SKILL, "PDF Summariser") },
      [`/skills/${SKILL}/versions`]: { body: SKILL_VERSIONS },
      "/downloads": { body: { downloads: [] } },
    };
    const hit = routes[path];
    return json(hit?.body ?? { error: "not found" }, hit?.status ?? (hit ? 200 : 404));
  });

  await render(
    <PublishingWorkspace />,
    () => document.activeElement?.getAttribute("data-bundle-version") === olderVersion.version_id,
  );

  const member = container.querySelector<HTMLSelectElement>(
    `select[data-bundle-version="${olderVersion.version_id}"]`,
  );
  expect(member?.value).toBe(olderVersion.version_id);
  expect(member?.selectedOptions[0]?.textContent).toContain("v1");
  expect(text()).toContain("從 v1 接續建立 Bundle");

  const fields = container.querySelectorAll<HTMLInputElement>("form.bundle-form input");
  const setInput = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setInput.call(fields[0], "pdf-toolkit");
    fields[0].dispatchEvent(new Event("input", { bubbles: true }));
    setInput.call(fields[1], "1.0.0");
    fields[1].dispatchEvent(new Event("input", { bubbles: true }));
    const description = container.querySelector<HTMLTextAreaElement>("form.bundle-form textarea")!;
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(
      description,
      "Exact versions",
    );
    description.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => button("建立")?.click());
  await waitFor(() => created !== undefined);
  await waitFor(() => button("建立中")?.disabled === true);

  expect(created).toMatchObject({ member_version_ids: [olderVersion.version_id] });
  expect(
    Array.from(
      container.querySelectorAll(
        "form.bundle-form input, form.bundle-form textarea, form.bundle-form select",
      ),
    ).every((field) => (field as HTMLInputElement).disabled),
  ).toBe(true);

  await act(async () => complete());
  await waitFor(() => text().includes("已建立 pdf-toolkit v1.0.0"));
  expect(
    Array.from(container.querySelectorAll('form.bundle-form [role="status"]')).some((node) =>
      node.textContent?.includes("已建立 pdf-toolkit v1.0.0"),
    ),
  ).toBe(true);
  await act(async () => {
    setInput.call(fields[0], "下一個 Bundle");
    fields[0].dispatchEvent(new Event("input", { bubbles: true }));
  });
  expect(text()).not.toContain("已建立 pdf-toolkit v1.0.0");
});

test("a Bundle member version read failure stays unknown and blocks creation", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { publications: [] } },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": {
      body: {
        skills: [{ skill_id: SKILL }],
        total: 1,
        limit: 100,
        truncated: false,
      },
    },
    [`/api/skills/${SKILL}`]: { body: skillDetail(SKILL, "PDF Summariser") },
    [`/skills/${SKILL}/versions`]: { body: { error: "version backend unavailable" }, status: 503 },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("暫時無法讀取可加入 Bundle 的版本"));

  expect(text()).not.toContain("還沒有可加入的小工具");
  expect(text()).not.toContain("version backend unavailable");
  expect(button("建立")?.disabled).toBe(true);
});

test("an older Bundle row exports and first publishes the immutable version shown on that row", async () => {
  const bundle = {
    bundle: "pdf-toolkit",
    version: "1.0.0",
    description: "PDF tools",
    content_hash: "sha256:bundle",
    created_at: "2026-09-20T00:00:00Z",
    members: [
      {
        skill_id: SKILL,
        version_id: SKILL_VERSIONS.versions[0].version_id,
        name: "PDF Summariser",
        version_number: 2,
        content_hash: "sha256:aa",
      },
    ],
  };
  const latest = {
    ...bundle,
    version: "2.0.0",
    content_hash: "sha256:bundle-latest",
    created_at: "2026-09-21T00:00:00Z",
  };
  let exportURL = "";
  let publicationReads = 0;
  let published: Record<string, unknown> | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    const path = url.replace(/^https?:\/\/[^/]+/, "").split("?")[0];
    if (path === "/me/bundles/pdf-toolkit/publication" && !init?.method) publicationReads++;
    if (path === "/me/bundles/pdf-toolkit/export" && init?.method === "POST") {
      exportURL = url;
      return json({
        artifact_id: ARTIFACT,
        file_name: "pdf-toolkit.zip",
        size_bytes: 100,
        content_hash: "sha256:artifact",
        expires_at: "2099-01-01T00:00:00Z",
        duplicate: false,
        content_url: `/downloads/${ARTIFACT}/content`,
      });
    }
    if (path === "/me/bundles/pdf-toolkit/publication" && init?.method === "POST") {
      published = JSON.parse(String(init.body)) as Record<string, unknown>;
      return json({
        kind: "bundle",
        publisher: PUBLISHER,
        name: "pdf-toolkit",
        address: `/p/${PUBLISHER}/pdf-toolkit`,
        status: "published",
        status_changed_at: "2026-09-22T00:00:00Z",
        releases: [],
      });
    }
    const routes: Record<string, { body: unknown; status?: number }> = {
      "/me/publisher": { body: OWN_PUBLISHER },
      "/me/publications": { body: { publications: [] } },
      "/me/bundles": { body: { bundles: [latest, bundle] } },
      "/me/bundles/overview": { body: bundleOverview(latest) },
      "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
      "/downloads": { body: { downloads: [] } },
      "/me/bundles/pdf-toolkit/publication": {
        body: { error: "not published" },
        status: 404,
      },
    };
    const hit = routes[path];
    return json(hit?.body ?? { error: "not found" }, hit?.status ?? (hit ? 200 : 404));
  });
  await render(<PublishingWorkspace />, () => text().includes("尚未發佈"));

  expect(
    container.querySelector(
      `a[href="/skills/${SKILL}/versions/${SKILL_VERSIONS.versions[0].version_id}"]`,
    )?.textContent,
  ).toBe("PDF Summariser v2");

  expect(container.querySelectorAll(".bundle-version-list")).toHaveLength(1);
  expect(container.querySelectorAll(".bundle-version-item")).toHaveLength(2);
  expect(publicationReads).toBe(0);

  await act(async () => button("匯出 v1.0.0 Plugin")?.click());
  await waitFor(() => text().includes("pdf-toolkit.zip"));
  expect(exportURL).toContain("/me/bundles/pdf-toolkit/export?version=1.0.0");

  const continuation = Array.from(container.querySelectorAll("a")).find((link) =>
    link.textContent?.includes("查看這次交付紀錄"),
  );
  expect(continuation?.getAttribute("href")).toBe(`/workspace/downloads?artifact=${ARTIFACT}`);

  await act(async () => button("首次發佈 v1.0.0")?.click());
  await waitFor(() => published !== undefined);
  expect(published).toMatchObject({
    name: "pdf-toolkit",
    version: "1.0.0",
    rights_attested: false,
  });
  expect(publicationReads).toBe(0);
});

test("an older published Bundle row republishes the immutable version shown on that row", async () => {
  const older = {
    ...OWN_BUNDLE,
    version: "1.0.0",
    content_hash: "sha256:bundle-1",
    created_at: "2026-09-01T00:00:00Z",
  };
  const publication: Publication = {
    kind: "bundle",
    publisher: PUBLISHER,
    name: OWN_BUNDLE.bundle,
    address: `/p/${PUBLISHER}/${OWN_BUNDLE.bundle}`,
    status: "published",
    status_changed_at: "2026-09-20T00:00:00Z",
    releases: [
      {
        bundle_version: OWN_BUNDLE.version,
        content_hash: OWN_BUNDLE.content_hash,
        released_at: "2026-09-20T00:00:00Z",
        rights_attested: false,
        findings: { errors: [], warnings: [], infos: [] },
      },
    ],
  };
  let published: Record<string, unknown> | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === `/me/bundles/${OWN_BUNDLE.bundle}/publication` && init?.method === "POST") {
      published = JSON.parse(String(init.body)) as Record<string, unknown>;
      return json(publication);
    }
    const routes: Record<string, { body: unknown; status?: number }> = {
      "/me/publisher": { body: OWN_PUBLISHER },
      "/me/publications": { body: { publications: [] } },
      "/me/bundles": { body: { bundles: [OWN_BUNDLE, older] } },
      "/me/bundles/overview": { body: bundleOverview(OWN_BUNDLE, publication) },
      "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
      "/downloads": { body: { downloads: [] } },
      [`/me/bundles/${OWN_BUNDLE.bundle}/publication`]: { body: publication },
    };
    const hit = routes[path];
    return json(hit?.body ?? { error: "not found" }, hit?.status ?? (hit ? 200 : 404));
  });

  await render(<PublishingWorkspace />, () => text().includes("目前 Release"));
  await act(async () => button("發佈 v1.0.0")?.click());
  await waitFor(() => published !== undefined);

  expect(published).toMatchObject({ version: "1.0.0", rights_attested: false });
});

test("the publishing overview keeps public identity and the exact latest release connected", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: OWN_PUBLICATIONS },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("最新 Release"));

  expect(container.querySelector(`a[href="/p/${PUBLISHER}/${PUBLICATION}"]`)).not.toBeNull();
  expect(
    container.querySelector(
      `a[href="/skills/${SKILL}/versions/${SKILL_VERSIONS.versions[0].version_id}"]`,
    )?.textContent,
  ).toBe("v2");
  expect(text()).toContain("Catalog 是否曝光仍由營運者另行審核");
  expect(text()).toContain("已發佈");
});

test("the publishing overview separates public reach, package eligibility, and Catalog discovery", async () => {
  const acquisitionNote =
    "登入後可以下載這一版的標準 Agent 小工具套件；這個部署目前只開放受邀者下載。";
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": {
      body: {
        publications: OWN_PUBLICATIONS.publications.map((publication) => ({
          ...publication,
          availability: { value: "available", label: "提供中", note: "" },
          acquisition: { available: true, note: acquisitionNote },
        })),
      },
    },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("已列入 Catalog"));

  expect(text()).toContain("公開頁面");
  expect(text()).toContain("任何人都能閱讀");
  expect(text()).toContain("套件取得");
  expect(text()).toContain("目前提供套件");
  expect(text()).toContain(acquisitionNote);
  expect(text()).toContain("Catalog 探索");
  expect(text()).not.toContain("任何人都能下載");
});

test("a published Bundle shows the server-owned package eligibility beside its exact release", async () => {
  const acquisitionNote = "登入後可以下載這一版的 Agent Plugin；這個部署目前只開放受邀者下載。";
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { publications: [] } },
    "/me/bundles": { body: { bundles: [OWN_BUNDLE] } },
    "/me/bundles/overview": {
      body: bundleOverview(OWN_BUNDLE, {
        kind: "bundle",
        publisher: PUBLISHER,
        name: "pdf-toolkit",
        address: `/p/${PUBLISHER}/pdf-toolkit`,
        status: "published",
        status_changed_at: "2026-09-20T00:00:00Z",
        releases: [
          {
            bundle_version: OWN_BUNDLE.version,
            content_hash: OWN_BUNDLE.content_hash,
            released_at: "2026-09-20T00:00:00Z",
            rights_attested: false,
            findings: { errors: [], warnings: [], infos: [] },
          },
        ],
        availability: { value: "available", label: "可提供", note: "" },
        acquisition: { available: true, note: acquisitionNote },
      }),
    },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
    "/me/bundles/pdf-toolkit/publication": {
      body: {
        kind: "bundle",
        publisher: PUBLISHER,
        name: "pdf-toolkit",
        address: `/p/${PUBLISHER}/pdf-toolkit`,
        status: "published",
        status_changed_at: "2026-09-20T00:00:00Z",
        releases: [
          {
            bundle_version: OWN_BUNDLE.version,
            content_hash: OWN_BUNDLE.content_hash,
            released_at: "2026-09-20T00:00:00Z",
            rights_attested: false,
            findings: { errors: [], warnings: [], infos: [] },
          },
        ],
        availability: { value: "available", label: "提供中", note: "" },
        acquisition: { available: true, note: acquisitionNote },
      },
    },
  });

  await render(<PublishingWorkspace />, () => text().includes("目前 Release"));

  expect(text()).toContain("交付對象");
  expect(text()).toContain("任何人都能閱讀");
  expect(text()).toContain("目前提供套件");
  expect(text()).toContain(acquisitionNote);
  expect(text()).not.toContain("任何人都能下載");
});

test("a missing Bundle overview row stays unknown instead of becoming unpublished", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { publications: [] } },
    "/me/bundles": { body: { bundles: [OWN_BUNDLE] } },
    "/me/bundles/overview": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("無法確認這個 Bundle 的發佈狀態"));

  expect(text()).not.toContain("尚未發佈。可從下方任一不可變版本建立第一個公開 Release。");
  expect(button("確認發佈狀態後可操作")?.disabled).toBe(true);
});

test("the owner view preserves an unavailable reason instead of calling it an empty audience", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": {
      body: {
        publications: OWN_PUBLICATIONS.publications.map((publication) => ({
          ...publication,
          availability: {
            value: "held",
            label: "已不提供",
            note: "這個小工具的內容因授權問題被保留，釐清之前不提供。",
          },
          acquisition: {
            available: false,
            note: "這個發佈物目前不提供下載，原因見上方。",
          },
        })),
      },
    },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("目前不提供套件"));

  expect(text()).toContain("已不提供");
  expect(text()).toContain("因授權問題被保留");
  expect(text()).not.toContain("0 人");
});

test("an older owner payload leaves delivery eligibility unknown rather than unavailable", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": {
      body: {
        publications: OWN_PUBLICATIONS.publications.map((publication) => ({
          ...publication,
          availability: undefined,
          acquisition: undefined,
        })),
      },
    },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("交付對象暫時無法確認"));

  expect(text()).not.toContain("目前不提供套件");
  expect(text()).not.toContain("已不提供");
});

test("a publication continuation link focuses only the exact owner row after it loads", async () => {
  publishingSearch = { publication: `${PUBLISHER}/${PUBLICATION}` };
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: OWN_PUBLICATIONS },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(
    <PublishingWorkspace />,
    () => document.activeElement?.getAttribute("aria-current") === "location",
  );

  const current = container.querySelector<HTMLElement>('li[aria-current="location"]');
  expect(current?.textContent).toContain(PUBLICATION);
  expect(current?.textContent).toContain("續接位置");
  expect(document.activeElement).toBe(current);
  expect(container.querySelector('nav[aria-label="發佈工作區導覽"]')).toBeNull();
});

test("an artifact continuation link focuses the exact package row after it loads", async () => {
  publishingSearch = { artifact: ARTIFACT };
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { publications: [] } },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [ARTIFACT_ROW] } },
  });

  await render(
    <PublishingWorkspace />,
    () => document.activeElement?.getAttribute("aria-current") === "location",
  );

  const current = container.querySelector<HTMLElement>('li[aria-current="location"]');
  expect(current?.textContent).toContain(ARTIFACT_ROW.file_name);
  expect(current?.textContent).toContain("續接位置");
  expect(document.activeElement).toBe(current);
});

test("a delayed continuation result does not take focus after the user starts elsewhere", async () => {
  publishingSearch = { publication: `${PUBLISHER}/${PUBLICATION}` };
  let resolvePublications!: (response: Response) => void;
  const publications = new Promise<Response>((resolve) => {
    resolvePublications = resolve;
  });
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me/publications") return publications;
    const body: Record<string, unknown> = {
      "/me/publisher": OWN_PUBLISHER,
      "/me/bundles": { bundles: [] },
      "/skills": { skills: [], total: 0, limit: 100, truncated: false },
      "/downloads": { downloads: [] },
    };
    return json(body[path] ?? { error: "not found" }, path in body ? 200 : 404);
  });

  await render(<PublishingWorkspace />, () => text().includes("載入小工具發佈清單中"));
  const userControl = document.createElement("button");
  document.body.appendChild(userControl);
  userControl.focus();

  resolvePublications(
    new Response(JSON.stringify(OWN_PUBLICATIONS), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  await waitFor(() => container.querySelector('[aria-current="location"]') !== null);

  expect(document.activeElement).toBe(userControl);
  userControl.remove();
});

test("a successful owner read names a missing continuation target without selecting another row", async () => {
  publishingSearch = { artifact: "ffffffff-ffff-ffff-ffff-ffffffffffff" };
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { publications: [] } },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [ARTIFACT_ROW] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("目前找不到這筆交付紀錄"));

  expect(container.querySelector('#delivery-history [role="status"]')?.textContent).toContain(
    "目前找不到這筆交付紀錄",
  );
  expect(container.querySelector('[aria-current="location"]')).toBeNull();
});

test("a link with multiple continuation targets explains the conflict and moves no focus", async () => {
  publishingSearch = {
    artifact: ARTIFACT,
    publication: `${PUBLISHER}/${PUBLICATION}`,
    bundleVersion: VERSION,
  };
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: OWN_PUBLICATIONS },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [ARTIFACT_ROW] } },
  });

  await render(<PublishingWorkspace />, () => text().includes("同時指定了多個續接位置"));

  expect(container.querySelector('[role="alert"]')?.textContent).toContain("一次只能續接一筆");
  expect(container.querySelector('[aria-current="location"]')).toBeNull();
  expect(container.contains(document.activeElement)).toBe(false);
});

test.each([
  ["listed", "已列入 Catalog", "任何人都能從搜尋與 Catalog 找到這個 Release"],
  ["awaiting_review", "等待 Catalog 審核", "這個 Release 還不會出現在搜尋與 Catalog"],
  ["revoked", "Catalog 曝光已撤銷", "公開位址仍可使用"],
  ["review_outdated", "需要重新審核", "Catalog 收錄所依據的搜尋內容已變更"],
  ["not_eligible", "目前不符合曝光條件", "可用性或散布條件不允許曝光"],
  ["search_not_ready", "搜尋內容尚未就緒", "搜尋內容尚未可列出"],
  ["unreleased", "尚無 Release", "建立第一個不可變 Release"],
] as const)("the publishing overview explains the %s Catalog state", async (state, label, note) => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": {
      body: {
        publications: OWN_PUBLICATIONS.publications.map((publication) => ({
          ...publication,
          catalog_exposure: { state },
        })),
      },
    },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => text().includes(label));

  expect(text()).toContain(note);
});

test("the publishing overview reports a failed read instead of claiming the collection is empty", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    "/me/publications": { body: { error: "database unavailable" }, status: 500 },
    "/me/bundles": { body: { bundles: [] } },
    "/skills": { body: { skills: [], total: 0, limit: 100, truncated: false } },
    "/downloads": { body: { downloads: [] } },
  });

  await render(<PublishingWorkspace />, () => container.querySelector('[role="alert"]') !== null);

  expect(container.querySelector('[role="alert"]')?.textContent).toContain(
    "暫時無法讀取小工具發佈清單",
  );
  expect(text()).not.toContain("還沒有任何小工具 Publication");
  expect(text()).not.toContain("database unavailable");
});

test("PACK-004 available 公開頁：每一個允收欄位都出現", async () => {
  stub({ [PUB_ADDRESS]: { body: PUBLIC_PUBLICATION } });
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(text()).toContain("PDF Summariser");
  expect(text()).toContain("把 PDF 整理成摘要");
  expect(text()).toContain(PUBLISHER);
  expect(text()).toContain("v2");
  expect(text()).toContain("sha256:aa");
  expect(text()).toContain("靜態掃描");
  expect(text()).toContain("MIT");
  expect(text()).toContain("可再散布");
  expect(text()).toContain("公開頁面");
  expect(text()).toContain("任何人都能閱讀");
  expect(text()).toContain("套件取得");
  expect(text()).toContain("目前提供套件");
  expect(text()).toContain("Catalog 探索");
  expect(text()).toContain("目前無法從 Catalog 找到");
  expect(text()).toContain("這個發佈物目前不在搜尋與目錄裡");
  expect(text()).toContain("登入後可以下載這一版的標準 Agent 小工具套件");
});

test("a public release with findings still states the limits of static scanning", async () => {
  stub({
    [PUB_ADDRESS]: {
      body: {
        ...PUBLIC_PUBLICATION,
        release: {
          ...PUBLIC_PUBLICATION.release,
          findings: {
            errors: [],
            warnings: [{ severity: "warning", code: "SCRIPT_FOUND", message: "套件含有 Script。" }],
            infos: [],
          },
        },
      },
    },
  });
  await render(<PublicPublication />, () => text().includes("SCRIPT_FOUND"));

  expect(text()).toContain("平台沒有執行套件 Script");
  expect(text()).toContain("不代表安全保證");
});

test("PACK-004 公開頁先交代信任證據，再提供主要取得動作", async () => {
  stub({ [PUB_ADDRESS]: { body: PUBLIC_PUBLICATION } });
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  const trust = container.querySelector('[data-role="publication-trust"]');
  const acquire = container.querySelector('[data-role="publication-acquire"]');
  expect(trust).not.toBeNull();
  expect(acquire).not.toBeNull();
  expect(trust!.compareDocumentPosition(acquire!) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
    Node.DOCUMENT_POSITION_FOLLOWING,
  );
  expect(acquire?.querySelector("button")?.classList.contains("action")).toBe(true);
});

// T3 等價類: 未登入（不呼叫 /me）也能讀到公開頁
test("PACK-004 公開頁不需要登入：從不呼叫 /me 也能顯示內容", async () => {
  const calls = stub({ [PUB_ADDRESS]: { body: PUBLIC_PUBLICATION } });
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(calls.some((u) => u.includes("/me"))).toBe(false);
});

// T2 決策表：五種不可用狀態只顯示理由，不顯示內容；delisted 額外顯示撤回時間
describe("PACK-004 不可用狀態", () => {
  const CASES: { value: string; label: string; note: string; delistedAt?: string }[] = [
    {
      value: "delisted",
      label: "作者已撤回",
      note: "作者撤回了這個發佈物，這一頁不再提供它的內容。",
      delistedAt: "2026-09-01T00:00:00Z",
    },
    {
      value: "withdrawn",
      label: "已不提供",
      note: "這個發佈物指向的小工具已經被作者刪除。",
    },
    { value: "taken_down", label: "已不提供", note: "這個小工具已被平台下架。" },
    {
      value: "held",
      label: "已不提供",
      note: "這個小工具的內容因授權問題被保留，釐清之前不提供。",
    },
    {
      value: "not_redistributable",
      label: "已不提供",
      note: "這個小工具目前的授權判定不允許再散布。",
    },
  ];

  for (const testCase of CASES) {
    test(`availability=${testCase.value} 只顯示理由，不顯示小工具內容`, async () => {
      stub({
        [PUB_ADDRESS]: {
          body: {
            ...PUBLIC_PUBLICATION,
            availability: { value: testCase.value, label: testCase.label, note: testCase.note },
            delisted_at: testCase.delistedAt,
            skill: undefined,
            release: undefined,
          },
        },
      });
      await render(<PublicPublication />, () => text().includes(testCase.label));

      expect(text()).toContain(testCase.note);
      expect(text()).not.toContain("PDF Summariser");
      expect(text()).not.toContain("sha256:aa");
      if (testCase.delistedAt) {
        expect(text()).toContain("撤回時間");
      } else {
        expect(text()).not.toContain("撤回時間");
      }
    });
  }
});

test("PACK-004 沒有這個發佈物：404 說出來，不是空白", async () => {
  stub({ [PUB_ADDRESS]: { body: { error: "no publication has this address" }, status: 404 } });
  await render(<PublicPublication />, () => text().includes("沒有這個發佈物"));
});

const ACQUIRE_ADDRESS = `${PUB_ADDRESS}/acquisitions`;

const DELISTED_NOTE = "作者撤回了這個發佈物，這一頁不再提供它的內容。";

function unavailablePublication(
  initial: typeof PUBLIC_PUBLICATION | typeof PUBLIC_BUNDLE_PUBLICATION,
) {
  return {
    kind: initial.kind,
    publisher: initial.publisher,
    name: initial.name,
    address: initial.address,
    availability: { value: "delisted", label: "作者已撤回", note: DELISTED_NOTE },
    delisted_at: "2026-10-20T18:00:00Z",
    releases: [],
    exposure: { available: false, note: DELISTED_NOTE },
    acquisition: { available: false, note: DELISTED_NOTE },
  };
}

function availabilityScenario(
  kind: "skill" | "bundle",
  reread: () => Promise<Response>,
  status = 409,
  reply: unknown = status === 404
    ? { error: "not found" }
    : { error: DELISTED_NOTE, reason: "delisted" },
) {
  const initial = kind === "skill" ? PUBLIC_PUBLICATION : PUBLIC_BUNDLE_PUBLICATION;
  publicParams = { publisher: PUBLISHER, name: initial.name };
  const address = `/publications/${PUBLISHER}/${initial.name}`;
  const state = { reads: 0, writes: 0, initial };
  container.id = "main-content";
  container.tabIndex = -1;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === address + "/acquisitions" && init?.method === "POST") {
      state.writes++;
      return json(reply, status);
    }
    if (path === address) {
      state.reads++;
      return state.reads === 1 ? json(initial) : reread();
    }
    const response = platformResponse(String(input));
    return json(response.body, response.status);
  });
  return state;
}

async function rejectedAcquisition(readFinished = false) {
  await pollUntil(
    () =>
      queryClient
        .getMutationCache()
        .getAll()
        .some((mutation) => mutation.state.status === "error") &&
      (!readFinished || queryClient.isFetching() === 0),
    text,
    DEFAULT_WAIT_MS,
    { flushBeforeFirstCheck: true },
  );
}

for (const kind of ["skill", "bundle"] as const) {
  for (const status of [404, 409]) {
    test(`a ${kind} acquisition rejected with ${status} removes obsolete content while its address is checked`, async () => {
      let finish!: (value: Response) => void;
      const held = new Promise<Response>((resolve) => {
        finish = resolve;
      });
      const state = availabilityScenario(kind, () => held, status);
      const ownKey = queryKeys.publishing.ownPublication(SKILL);
      queryClient.setQueryData(ownKey, OWN_PUBLICATION);
      const ownQuery = queryClient.getQueryCache().find({ queryKey: ownKey, exact: true })!;
      try {
        await render(<PublicPublication />, () => Boolean(button("下載")));
        await act(async () => button("下載")!.click());
        await rejectedAcquisition();
        expect(ownQuery.state.data).toBe(OWN_PUBLICATION);
        expect(state.reads).toBe(2);
        expect(text()).toContain("載入這個發佈物中");
        expect(container.querySelector("h1")?.textContent).toBe("公開發佈物");
        expect(container.querySelector(".publication-trust")).toBeNull();
        expect(button("下載")).toBeUndefined();
        expect(document.activeElement).toBe(container);
        await act(async () =>
          finish(
            await json(
              status === 404
                ? { error: "no publication has this address" }
                : unavailablePublication(state.initial),
              status === 404 ? 404 : 200,
            ),
          ),
        );
        await waitFor(() => text().includes(status === 404 ? "沒有這個發佈物" : "作者已撤回"));
        expect(container.querySelector(".publication-trust")).toBeNull();
        expect(button("下載")).toBeUndefined();
        expect(state.reads).toBe(2);
        expect(state.writes).toBe(1);
      } finally {
        finish(await json({ error: "no publication has this address" }, 404));
      }
    });
  }

  test(`a ${kind} availability reread failure withholds obsolete content and retries only the read`, async () => {
    const state = availabilityScenario(kind, () =>
      state.reads === 2
        ? json({ error: "publication lookup failed" }, 500)
        : json(unavailablePublication(state.initial)),
    );
    await render(<PublicPublication />, () => Boolean(button("下載")));
    await act(async () => button("下載")!.click());
    await rejectedAcquisition(true);
    expect(text()).toContain("暫時無法讀取這個發佈物");
    expect(container.querySelector("h1")?.textContent).toBe("公開發佈物");
    expect(container.querySelector(".publication-trust")).toBeNull();
    expect(button("下載")).toBeUndefined();
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
    await act(async () => button("再試一次")!.click());
    await waitFor(() => text().includes("作者已撤回"));
    expect(state.reads).toBe(3);
    expect(state.writes).toBe(1);
    expect(document.activeElement).toBe(container);
  });
}

test("a paused availability check withholds obsolete content and resumes without another acquisition", async () => {
  const state = availabilityScenario(
    "skill",
    () => json(unavailablePublication(state.initial)),
    409,
  );
  const fetch = globalThis.fetch;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (init?.method === "POST") onlineManager.setOnline(false);
    return fetch(input, init);
  });
  try {
    await render(<PublicPublication />, () => Boolean(button("下載")));
    await act(async () => button("下載")!.click());
    await rejectedAcquisition();
    expect(text()).toContain("等待連線以讀取這個發佈物");
    expect(button("下載")).toBeUndefined();
    expect(container.querySelector(".publication-trust")).toBeNull();
    expect(state.reads).toBe(1);
    expect(state.writes).toBe(1);
    await act(async () => onlineManager.setOnline(true));
    await waitFor(() => text().includes("作者已撤回"));
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
  } finally {
    onlineManager.setOnline(true);
  }
});

test.each([
  [401, "not authenticated", "這個下載需要登入"],
  [
    403,
    "Skill Hub 還在封測：瀏覽與小工具詳情對所有人開放，但複製一份、試跑與下載只開放給受邀的測試者。",
    "還在封測",
  ],
  [422, "這個小工具的內容因授權問題尚未釐清而被保留，所以不能發佈", "授權問題尚未釐清"],
  [500, "publishing failed", "下載結果尚未確認"],
])(
  "an acquisition rejected with unrelated status %s retains context without resetting availability",
  async (status, message, expected) => {
    const state = availabilityScenario("skill", () => json(PUBLIC_PUBLICATION), status, {
      error: message,
    });
    await render(<PublicPublication />, () => Boolean(button("下載")));
    await act(async () => button("下載")!.click());
    await rejectedAcquisition(true);
    expect(text()).toContain(expected);
    expect(text()).toContain("把 PDF 整理成摘要");
    expect(button("下載")).toBeDefined();
    expect(state.reads).toBe(1);
    expect(state.writes).toBe(1);
  },
);

test("a newly available reread preserves the acquisition refusal instead of losing the result", async () => {
  let finish!: (value: Response) => void;
  const held = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const state = availabilityScenario("skill", () => held);
  try {
    await render(<PublicPublication />, () => Boolean(button("下載")));
    await act(async () => button("下載")!.click());
    await rejectedAcquisition();
    expect(state.reads).toBe(2);
    expect(text()).toContain("載入這個發佈物中");
    await act(async () => finish(await json(PUBLIC_PUBLICATION)));
    await rejectedAcquisition(true);
    expect(text()).toContain(DELISTED_NOTE);
    expect(text()).toContain("上次下載的回饋");
    expect(text()).toContain("把 PDF 整理成摘要");
    expect(button("下載")?.disabled).toBe(false);
    expect(state.writes).toBe(1);
  } finally {
    finish(await json(PUBLIC_PUBLICATION));
  }
});

test("a different public address starts without the previous acquisition outcome", async () => {
  const state = availabilityScenario("skill", () => json(PUBLIC_PUBLICATION), 201, null);
  const fetch = globalThis.fetch;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) =>
    String(input).endsWith(`/publications/${PUBLISHER}/pdf-toolkit`)
      ? json(PUBLIC_BUNDLE_PUBLICATION)
      : fetch(input, init),
  );
  await render(<PublicPublication />, () => Boolean(button("下載")));
  await act(async () => button("下載")!.click());
  await waitFor(() => text().includes("下載結果尚未確認"));
  publicParams = { publisher: PUBLISHER, name: "pdf-toolkit" };
  await act(async () =>
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <PublicPublication />
        </QueryClientProvider>
      </StrictMode>,
    ),
  );
  await waitFor(() => text().includes("一組跟 PDF 有關的小工具"));
  expect(container.querySelector('[role="alert"]')).toBeNull();
  expect(text()).not.toContain("另開交付紀錄核對");
  expect(text()).not.toContain("上次下載的回饋");
  expect(button("下載")?.disabled).toBe(false);
  expect(state.writes).toBe(1);
});

for (const kind of ["publication", "bundle"] as const) {
  test.each([
    ["null", null],
    ["missing URL", { artifact_id: ARTIFACT }],
    [
      "foreign URL",
      {
        artifact_id: ARTIFACT,
        file_name: "pdf-toolkit.zip",
        size_bytes: 100,
        content_hash: "sha256:artifact",
        expires_at: "2099-01-01T00:00:00Z",
        duplicate: false,
        content_url: "/downloads/55555555-5555-4555-8555-555555555555/content",
      },
    ],
  ])(
    `an uncertain ${kind} acquisition with %s preserves context and offers record verification`,
    async (_, reply) => {
      const writes: string[] = [];
      const endpoint = kind === "publication" ? ACQUIRE_ADDRESS : "/me/bundles/pdf-toolkit/export";
      vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
        const path = String(input)
          .replace(/^https?:\/\/[^/]+/, "")
          .split("?")[0];
        if (path === endpoint && init?.method === "POST") {
          writes.push(String(input));
          return json(reply, 201);
        }
        if (path === "/me/bundles") return json({ bundles: [OWN_BUNDLE] });
        if (path === "/me/bundles/overview") return json(bundleOverview(OWN_BUNDLE));
        const response = platformResponse(String(input));
        return json(response.body, response.status);
      });
      await render(kind === "publication" ? <PublicPublication /> : <PublishingWorkspace />, () =>
        Boolean(button(kind === "publication" ? "下載" : "匯出 v1.1.0 Plugin")),
      );
      await act(async () =>
        button(kind === "publication" ? "下載" : "匯出 v1.1.0 Plugin")!.click(),
      );
      await waitFor(() => text().includes("下載結果尚未確認"));
      expect(text()).toContain(
        kind === "publication" ? "PDF Summariser" : "一組跟 PDF 有關的小工具",
      );
      const link = Array.from(container.querySelectorAll("a")).find(
        (item) => item.textContent === "另開交付紀錄核對",
      );
      expect(link?.getAttribute("href")).toBe("/workspace/downloads");
      expect(link?.getAttribute("target")).toBe("_blank");
      expect(link?.getAttribute("rel")).toBe("noopener noreferrer");
      expect(document.activeElement?.getAttribute("role")).toBe("alert");
      expect(container.querySelector('a[href="undefined"], a[href*="55555555-5555"]')).toBeNull();
      expect(writes).toHaveLength(1);
    },
  );
}

test.each(["network", "server error", "unreadable JSON"])(
  "a publication acquisition with %s keeps an uncertain outcome without resending",
  async (failure) => {
    let writes = 0;
    vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
      const path = String(input)
        .replace(/^https?:\/\/[^/]+/, "")
        .split("?")[0];
      if (path === ACQUIRE_ADDRESS && init?.method === "POST") {
        writes++;
        if (failure === "network") throw new TypeError("Failed to fetch");
        if (failure === "unreadable JSON")
          return Promise.resolve(new Response("{", { status: 201 }));
        return json({ error: "internal error" }, 500);
      }
      const response = platformResponse(String(input));
      return json(response.body, response.status);
    });
    await render(<PublicPublication />, () => Boolean(button("下載")));
    await act(async () => button("下載")!.click());
    await waitFor(() => text().includes("下載結果尚未確認"));
    expect(text()).toContain("另開交付紀錄核對");
    expect(text()).not.toContain("下載沒有成功");
    expect(text()).not.toContain("Failed to fetch");
    expect(writes).toBe(1);
  },
);

function stubAcquire(post: () => { body: unknown; status?: number }) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === ACQUIRE_ADDRESS && init?.method === "POST") {
      const { body, status } = post();
      return json(body, status ?? 201);
    }
    if (path === PUB_ADDRESS) return json(PUBLIC_PUBLICATION);
    return json({ error: "not found" }, 404);
  });
}

// T2 等價類: acquisition.available=false 時，即使 availability 是 available，也不顯示按鈕
test("PACK-006 acquisition.available=false 時不顯示下載按鈕，只顯示 note", async () => {
  stub({
    [PUB_ADDRESS]: {
      body: {
        ...PUBLIC_PUBLICATION,
        acquisition: { available: false, note: "這個發佈物目前不提供下載，原因見上方。" },
      },
    },
  });
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(text()).toContain("這個發佈物目前不提供下載，原因見上方。");
  expect(button("下載")).toBeUndefined();
});

test("PACK-006 acquisition.available=true 時顯示下載按鈕，按下後用 content_url 觸發下載", async () => {
  stubAcquire(() => ({
    body: {
      artifact_id: ARTIFACT,
      file_name: "pdf-summariser-v2.zip",
      size_bytes: 100,
      content_hash: "sha256:zz",
      expires_at: "2099-01-01T00:00:00Z",
      duplicate: false,
      content_url: `/downloads/${ARTIFACT}/content`,
    },
  }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(button("下載")).toBeDefined();
  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("pdf-summariser-v2.zip"));

  const link = Array.from(container.querySelectorAll("a")).find((a) =>
    (a.textContent ?? "").includes("pdf-summariser-v2.zip"),
  );
  expect(link?.getAttribute("href")).toBe(`/downloads/${ARTIFACT}/content`);
  const continuation = Array.from(container.querySelectorAll("a")).find((a) =>
    (a.textContent ?? "").includes("在交付紀錄查看這一份"),
  );
  expect(continuation?.getAttribute("href")).toBe(`/workspace/downloads?artifact=${ARTIFACT}`);
});

test("PACK-006 401：未登入按下下載，交給既有登入元件說一次", async () => {
  stubAcquire(() => ({ body: { error: "not authenticated" }, status: 401 }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("這個下載需要登入"));

  expect(text()).not.toContain("not authenticated");
});

test("PACK-006 403：未受邀，顯示伺服器的中文說明", async () => {
  const NOT_INVITED =
    "Skill Hub 還在封測：瀏覽與小工具詳情對所有人開放，但複製一份、試跑與下載只開放給受邀的測試者。";
  stubAcquire(() => ({ body: { error: NOT_INVITED }, status: 403 }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("還在封測"));

  expect(text()).toContain(NOT_INVITED);
});

test("PACK-006 409：理由是 availability，訊息就是伺服器的字串", async () => {
  stubAcquire(() => ({
    body: { error: "作者撤回了這個發佈物，這一頁不再提供它的內容。", reason: "delisted" },
    status: 409,
  }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("作者撤回了這個發佈物"));
});

test("PACK-006 422：打包器拒絕，訊息就是伺服器的字串", async () => {
  stubAcquire(() => ({
    body: {
      error: "這個小工具的內容因授權問題尚未釐清而被保留，所以不能發佈",
      reason: "license_hold",
    },
    status: 422,
  }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("授權問題尚未釐清"));
});

test("PACK-018 Bundle 公開頁列出成員，且每一次 Release 逐項說出誰升級、誰加入、誰移除", async () => {
  stub({ [PUB_ADDRESS]: { body: PUBLIC_BUNDLE_PUBLICATION } });
  await render(<PublicPublication />, () => text().includes("一組跟 PDF 有關的小工具"));

  expect(text()).toContain("summariser · v3");
  expect(text()).toContain("splitter · v1");
  expect(text()).toContain("summariser：v2 升到 v3");
  expect(text()).toContain("splitter：新增（v1）");
});

test("PACK-003/004 每一個 PublishingRefusal.reason 都有一句中文", () => {
  const REASONS: PublishingRefusalReason[] = [
    "no_publisher",
    "already_registered",
    "name_taken",
    "name_held",
    "name_is_permanent",
    "name_shape",
    "name_reserved",
    "license_hold",
    "not_redistributable",
    "license_unknown",
    "validation_blocked",
    "rights_not_attested",
    "license_not_declarable",
  ];
  expect(Object.keys(PUBLISHING_REFUSAL_LABEL).sort()).toEqual([...REASONS].sort());
  for (const reason of REASONS) {
    expect(PUBLISHING_REFUSAL_LABEL[reason], `${reason} 沒有句子`).toMatch(/\p{Script=Han}/u);
  }
});

// PublisherSection ----------------------------------------------------

test("PublisherSection：已註冊時顯示名稱與永久事實，不顯示表單", async () => {
  stub({ "/me/publisher": { body: OWN_PUBLISHER } });
  await render(<PublisherSection />, () => text().includes(PUBLISHER));

  expect(text()).toContain("沒有改名的功能");
  expect(container.querySelector("form")).toBeNull();
});

test("PublisherSection：未註冊時顯示名稱規則、永久警語與表單", async () => {
  stub({ "/me/publisher": { body: { error: "no publisher" }, status: 404 } });
  await render(<PublisherSection />, () => text().includes("還沒有註冊發佈者名稱"));

  expect(text()).toContain("小寫英文字母、數字與連字號");
  expect(text()).toContain("沒有改名的功能");
  expect(container.querySelector("form")).not.toBeNull();
});

test("PublisherSection：409 already_registered 顯示中文，不顯示英文原文", async () => {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    calls.push(url);
    if (init?.method === "POST") {
      return json(
        { error: "this account already has a publisher name", reason: "already_registered" },
        409,
      );
    }
    return json({ error: "no publisher" }, 404);
  });
  await render(<PublisherSection />, () => text().includes("還沒有註冊發佈者名稱"));

  const input = container.querySelector("input")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(input, "acme-tools");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => button("註冊")?.click());
  await waitFor(() => text().includes("已經有一個發佈者名稱了"));

  expect(text()).not.toContain("this account already has a publisher name");
});

test("PublisherSection：422 name_shape 顯示中文", async () => {
  vi.stubGlobal("fetch", (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      return json(
        {
          error: "a name is 1 to 64 lowercase letters, digits and single hyphens",
          reason: "name_shape",
        },
        422,
      );
    }
    return json({ error: "no publisher" }, 404);
  });
  await render(<PublisherSection />, () => text().includes("還沒有註冊發佈者名稱"));

  const input = container.querySelector("input")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(input, "Bad Name!");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => button("註冊")?.click());
  await waitFor(() => text().includes("名稱格式不對"));

  expect(text()).not.toContain("lowercase letters");
});

// PublishPanel ----------------------------------------------------

function detail(over: Partial<SkillDetail> = {}): SkillDetail {
  return { ...skillDetail(SKILL, "PDF Summariser"), ...over };
}

test("PublishPanel：未登入只說明需要登入，不顯示任何發佈狀態", async () => {
  await render(<PublishPanel skill={detail()} isLoggedIn={false} isOwner={false} />, () =>
    text().includes("發佈需要登入"),
  );

  expect(text()).not.toContain("發佈狀態");
});

test("PublishPanel：不是擁有者時整塊不顯示", async () => {
  const calls = stub({});
  await render(
    <div data-testid="wrap">
      <PublishPanel skill={detail()} isLoggedIn={true} isOwner={false} />
    </div>,
    () => container.querySelector("[data-testid=wrap]") !== null,
  );
  await waitFor(() => queryClient.isFetching() === 0);

  expect(text()).toBe("");
  expect(calls.some((url) => url.includes("/me/publications"))).toBe(false);
});

test("PublishPanel：還沒有發佈者時，在精確版本脈絡內提供註冊", async () => {
  stub({ "/me/publisher": { body: { error: "no publisher" }, status: 404 } });
  await render(<PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />, () =>
    text().includes("確認後會回到這一版繼續建立 Publication"),
  );

  expect(container.querySelector("form.publisher-form")).not.toBeNull();
  expect(container.querySelector('a[href="/workspace/account"]')).toBeNull();
});

test("PublishPanel：尚未發佈時，名稱欄預設為小工具名稱", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: { error: "not published" }, status: 404 },
  });
  await render(
    <PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />,
    () => container.querySelector("input") !== null,
  );

  const input = container.querySelector("input") as HTMLInputElement;
  expect(input.value).toBe("PDF Summariser");
});

test("PublishPanel：已發佈時顯示公開位址、狀態與最新 Release", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: OWN_PUBLICATION },
    "/me/publications": { body: OWN_PUBLICATIONS },
  });
  await render(<PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />, () =>
    text().includes("已列入 Catalog"),
  );

  expect(text()).toContain(`/p/${PUBLISHER}/${PUBLICATION}`);
  expect(text()).toContain("已發佈");
  expect(text()).toContain("v2");
  expect(
    Array.from(container.querySelectorAll("a"))
      .find((link) => link.textContent?.includes("在發佈與交付中查看這一筆"))
      ?.getAttribute("href"),
  ).toBe(`/workspace/downloads?publication=${encodeURIComponent(`${PUBLISHER}/${PUBLICATION}`)}`);
  expect(button("發佈 v2")).toBeDefined();
  expect(button("撤回")).toBeDefined();
  expect(text()).toContain("任何人都能從搜尋與 Catalog 找到這個 Release");
});

test("PublishPanel：重新整理 Catalog 曝光時保留目前狀態與操作位置", async () => {
  let blockOverview = false;
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me/publisher") return json(OWN_PUBLISHER);
    if (path === `/skills/${SKILL}/publication`) return json(OWN_PUBLICATION);
    if (path === "/me/publications") {
      return blockOverview ? new Promise<Response>(() => undefined) : json(OWN_PUBLICATIONS);
    }
    return json({ error: "not found" }, 404);
  });

  await render(<PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />, () =>
    text().includes("已列入 Catalog"),
  );

  blockOverview = true;
  await act(async () => button("重新整理 Catalog 曝光狀態")?.click());
  await waitFor(() => text().includes("重新整理中…"));

  expect(text()).toContain("已列入 Catalog");
  expect(text()).toContain("任何人都能從搜尋與 Catalog 找到這個 Release");
  expect(text()).not.toContain("載入Catalog 曝光狀態中…");
});

test("PublishPanel：歷史 Release 不借用最新 Release 的 Catalog 曝光", async () => {
  const older = SKILL_VERSIONS.versions[1];
  const publication = {
    ...OWN_PUBLICATION,
    releases: [
      ...OWN_PUBLICATION.releases,
      {
        ...OWN_PUBLICATION.releases[0],
        version_id: older.version_id,
        version_number: older.version_number,
        released_at: "2026-08-01T00:00:00Z",
      },
    ],
  };
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: publication },
    "/me/publications": { body: OWN_PUBLICATIONS },
  });

  await render(
    <PublishPanel skill={detail()} version={older} isLoggedIn={true} isOwner={true} />,
    () => text().includes("重新整理 Catalog 曝光狀態"),
  );

  expect(text()).toContain("這一版的目前狀態不適用");
  expect(text()).toContain("Catalog 曝光狀態只描述");
  expect(text()).not.toContain("已列入 Catalog");
  expect(
    container.querySelector(`a[href="/skills/${SKILL}/versions/${VERSION}"]`)?.textContent,
  ).toContain("最新 Release v2");
});

test("PublishPanel：尚無 Release 的版本不產生 Catalog 曝光判斷", async () => {
  const unreleased = SKILL_VERSIONS.versions[1];
  const calls = stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: OWN_PUBLICATION },
    "/me/publications": { body: OWN_PUBLICATIONS },
  });

  await render(
    <PublishPanel skill={detail()} version={unreleased} isLoggedIn={true} isOwner={true} />,
    () => text().includes("Catalog 曝光"),
  );

  expect(text()).toContain("這一版的目前狀態不適用");
  expect(text()).toContain("v1 還沒有 Release；Catalog 只審核不可變 Release");
  expect(text()).not.toContain("已列入 Catalog");
  expect(calls.some((url) => url.includes("/me/publications"))).toBe(false);
});

test("PublishPanel：Catalog owner projection 讀取失敗不冒充未曝光", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: OWN_PUBLICATION },
    "/me/publications": { body: { error: "database unavailable" }, status: 500 },
  });

  await render(<PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />, () =>
    text().includes("暫時無法讀取Catalog 曝光狀態"),
  );

  expect(text()).not.toContain("已列入 Catalog");
  expect(text()).not.toContain("尚無 Release");
  expect(text()).not.toContain("database unavailable");
});

test("PublishPanel：Publication 身分對不上時明示無法確認", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: OWN_PUBLICATION },
    "/me/publications": { body: { publications: [] } },
  });

  await render(<PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />, () =>
    text().includes("發佈資料與工作區清單目前對不上"),
  );

  expect(text()).not.toContain("已列入 Catalog");
  expect(button("重新整理 Catalog 曝光狀態")).toBeDefined();
});

test("PublishPanel：發佈明確送出畫面上的版本，不讓伺服器另選最新版本", async () => {
  let posted: Record<string, unknown> | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me/publisher") return json(OWN_PUBLISHER);
    if (path === `/skills/${SKILL}/publication` && init?.method === "POST") {
      posted = JSON.parse(String(init.body)) as Record<string, unknown>;
      return json(OWN_PUBLICATION);
    }
    if (path === `/skills/${SKILL}/publication`) {
      return json({ error: "not published" }, 404);
    }
    return json({ error: "not found" }, 404);
  });

  const skill = detail();
  const selected = SKILL_VERSIONS.versions[1];
  await render(<PublishPanel skill={skill} version={selected} isLoggedIn isOwner />, () =>
    Boolean(container.querySelector("form")),
  );
  await act(async () =>
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
  await waitFor(() => posted !== undefined);

  expect(posted).toMatchObject({
    version_id: selected.version_id,
    name: "PDF Summariser",
  });
});

describe("PublishPanel：勾選框只在 self_supplied／generated 出現，授權選單只在 generated 出現", () => {
  const CASES: {
    value: SkillDetail["redistribution"]["value"];
    needsCheckbox: boolean;
    declaresLicense: boolean;
  }[] = [
    { value: "allowed", needsCheckbox: false, declaresLicense: false },
    { value: "self_supplied", needsCheckbox: true, declaresLicense: false },
    { value: "generated", needsCheckbox: true, declaresLicense: true },
  ];

  for (const testCase of CASES) {
    test(`redistribution=${testCase.value}`, async () => {
      stub({
        "/me/publisher": { body: OWN_PUBLISHER },
        [`/skills/${SKILL}/publication`]: { body: { error: "not published" }, status: 404 },
      });
      const skill = detail({
        redistribution: { value: testCase.value, label: "", note: "" },
      });
      await render(
        <PublishPanel skill={skill} isLoggedIn={true} isOwner={true} />,
        () => container.querySelector("input") !== null,
      );

      const checkbox = container.querySelector('input[type="checkbox"]');
      if (testCase.needsCheckbox) {
        expect(checkbox).not.toBeNull();
        expect(button("發佈")?.disabled).toBe(true);
        expect(text()).toContain("先勾選下面的聲明才能發佈");
      } else {
        expect(checkbox).toBeNull();
        expect(button("發佈")?.disabled).toBe(false);
      }
      expect(container.querySelector("select") !== null).toBe(testCase.declaresLicense);
    });
  }
});

test("可選的授權條款就是契約列出的那一組", () => {
  expect([...DECLARABLE_LICENSES].sort()).toEqual(Object.values(DeclarableLicense).sort());
});

test("PublishPanel：生成的小工具送出選定的授權；不聲明時不送這個欄位", async () => {
  const posted: Record<string, unknown>[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me/publisher") return json(OWN_PUBLISHER);
    if (path === `/skills/${SKILL}/publication` && init?.method === "POST") {
      posted.push(JSON.parse(String(init.body)) as Record<string, unknown>);
      return json({ error: "not yet" }, 409);
    }
    if (path === `/skills/${SKILL}/publication`) return json({ error: "not published" }, 404);
    return json({ error: "not found" }, 404);
  });
  const skill = detail({ redistribution: { value: "generated", label: "", note: "" } });
  await render(<PublishPanel skill={skill} isLoggedIn isOwner />, () =>
    Boolean(container.querySelector("select")),
  );
  const submit = async () => {
    await act(async () =>
      container
        .querySelector("form")!
        .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
    );
    await waitFor(() => posted.length > 0);
  };

  await submit();
  expect(posted.pop()).not.toHaveProperty("declared_license");

  const select = container.querySelector("select")!;
  await act(async () => {
    select.value = "MIT";
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await submit();
  expect(posted.pop()).toMatchObject({ declared_license: "MIT" });
});

test("refusalSentence: 沒有 reason 欄位時回傳 undefined，不誤植成中文句", () => {
  expect(refusalSentence(new Error("boom"))).toBeUndefined();
});
