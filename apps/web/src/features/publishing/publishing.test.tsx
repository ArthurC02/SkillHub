import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { PublicPublication } from "./PublicPublication.page";
import { PublisherSection } from "./components/PublisherSection";
import { PublishPanel } from "./components/PublishPanel";
import { PublishingWorkspace } from "./PublishingWorkspace.page";
import {
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
} from "../../testing/fixtures/platform";
import type { SkillDetail } from "../../core/api/types";
import type { BundleVersion, Publication } from "./publishing.service";

let container: HTMLDivElement;
let root: Root;
let publishingSearch: { artifact?: string; publication?: string; bundleVersion?: string } = {};

beforeEach(() => {
  queryClient.clear();
  publishingSearch = {};
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    children?: unknown;
  }) => {
    const path = Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to);
    const query = new URLSearchParams();
    Object.entries(search ?? {}).forEach(([key, value]) => {
      if (value !== undefined) query.set(key, value);
    });
    return <a href={`${path}${query.size ? `?${query}` : ""}`}>{children as never}</a>;
  },
  useParams: () => ({ publisher: PUBLISHER, name: PUBLICATION }),
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

  await render(<PublishingWorkspace />, () => text().includes("還沒有打包過任何套件"));

  expect(container.querySelector("h1")?.textContent).toBe("發佈與交付");
  const workspaceMap = container.querySelector('nav[aria-label="發佈工作區導覽"]');
  expect(workspaceMap).not.toBeNull();
  expect(
    Array.from(workspaceMap?.querySelectorAll("a") ?? []).map((link) => [
      link.getAttribute("href"),
      link.textContent?.replace(/\s+/g, " ").trim(),
    ]),
  ).toEqual([
    ["#skill-publications", "單一 Skill 不可變版本 → Publication → Release"],
    ["#bundle-workspace", "Bundle 成員版本 → Bundle Version → Release"],
    ["#delivery-history", "交付 Artifact → 可得條件 → 下載紀錄"],
  ]);
  const sections = Array.from(container.querySelectorAll("h2")).map(
    (heading) => heading.textContent,
  );
  expect(sections).toEqual(
    expect.arrayContaining(["Skill 發佈", "Bundle", "交付紀錄", "發佈者名稱", "從單一版本發佈"]),
  );
  expect(text()).toContain("公開位址不等於 Catalog 曝光");
  expect(text()).toContain("取得者身分與下載次數尚未提供");
  expect(container.querySelector('a[href="/library"]')?.textContent).toContain(
    "選擇要發佈的 Skill",
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
  await render(<PublishingWorkspace />, () => text().includes("交付紀錄"));

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
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me/bundles" && init?.method === "POST") {
      created = JSON.parse(String(init.body)) as Record<string, unknown>;
      return json(OWN_BUNDLE, 201);
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

  expect(created).toMatchObject({ member_version_ids: [olderVersion.version_id] });
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

  expect(text()).not.toContain("還沒有可加入的 Skill");
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
    "登入後可以下載這一版的標準 Agent Skill 套件；這個部署目前只開放受邀者下載。";
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
            note: "這個 Skill 的內容因授權問題被保留，釐清之前不提供。",
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

  await render(<PublishingWorkspace />, () => text().includes("載入Skill 發佈清單中"));
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

  expect(container.querySelector('[role="status"]')?.textContent).toContain(
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
    "暫時無法讀取Skill 發佈清單",
  );
  expect(text()).not.toContain("還沒有任何 Skill Publication");
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
  expect(text()).toContain("登入後可以下載這一版的標準 Agent Skill 套件");
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
      note: "這個發佈物指向的 Skill 已經被作者刪除。",
    },
    { value: "taken_down", label: "已不提供", note: "這個 Skill 已被平台下架。" },
    {
      value: "held",
      label: "已不提供",
      note: "這個 Skill 的內容因授權問題被保留，釐清之前不提供。",
    },
    {
      value: "not_redistributable",
      label: "已不提供",
      note: "這個 Skill 目前的授權判定不允許再散布。",
    },
  ];

  for (const testCase of CASES) {
    test(`availability=${testCase.value} 只顯示理由，不顯示 Skill 內容`, async () => {
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
    "Skill Hub 還在封測：瀏覽與 Skill 詳情對所有人開放，但 Fork、試跑與下載只開放給受邀的測試者。";
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
      error: "這個 Skill 的內容因授權問題尚未釐清而被保留，所以不能發佈",
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
  await render(<PublicPublication />, () => text().includes("一組跟 PDF 有關的 Skill"));

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
    "name_is_permanent",
    "name_shape",
    "name_reserved",
    "license_hold",
    "not_redistributable",
    "license_unknown",
    "validation_blocked",
    "rights_not_attested",
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

test("PublishPanel：尚未發佈時，名稱欄預設為 Skill 名稱", async () => {
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

describe("PublishPanel：勾選框只在 self_supplied／generated 出現", () => {
  const CASES: { value: SkillDetail["redistribution"]["value"]; needsCheckbox: boolean }[] = [
    { value: "allowed", needsCheckbox: false },
    { value: "self_supplied", needsCheckbox: true },
    { value: "generated", needsCheckbox: true },
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
    });
  }
});

test("refusalSentence: 沒有 reason 欄位時回傳 undefined，不誤植成中文句", () => {
  expect(refusalSentence(new Error("boom"))).toBeUndefined();
});
