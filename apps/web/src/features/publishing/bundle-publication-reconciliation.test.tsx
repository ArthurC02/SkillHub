import { expect, test } from "vitest";
import {
  OWN_BUNDLE,
  OWN_PUBLICATION,
  PUBLISHER,
  platformResponse,
} from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";
import type { Publication } from "./publishing.service";

const app = renderedApp();

const older = {
  ...OWN_BUNDLE,
  version: "1.0.0",
  content_hash: "sha256:bundle-1",
  created_at: "2026-09-01T00:00:00Z",
};
const publication: Publication = {
  ...OWN_PUBLICATION,
  kind: "bundle",
  name: OWN_BUNDLE.bundle,
  address: `/p/${PUBLISHER}/${OWN_BUNDLE.bundle}`,
  status: "published",
  status_changed_at: "2026-09-01T00:00:00Z",
  releases: [
    {
      bundle_version: "1.0.0",
      content_hash: older.content_hash,
      released_at: "2026-09-01T00:00:00Z",
      rights_attested: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
};

function overview(value: Publication) {
  return {
    bundles: [
      {
        bundle: OWN_BUNDLE.bundle,
        latest_version: OWN_BUNDLE,
        publication: {
          publisher: value.publisher,
          name: value.name,
          address: value.address,
          status: value.status,
          status_changed_at: value.status_changed_at,
          latest_release: value.releases[0],
          availability: value.availability,
          acquisition: value.acquisition,
        },
      },
    ],
  };
}

function stubOperation(
  page: App,
  action: "publish" | "delist",
  status: number,
  readFailure?: "unavailable" | "invalid",
) {
  const own = (platformResponse("/skills").body as { skills: object[] }).skills[0];
  page.route("**/skills?*", () => ({
    json: {
      skills: OWN_BUNDLE.members.map((member) => ({
        ...own,
        skill_id: member.skill_id,
        name: member.name,
      })),
      total: OWN_BUNDLE.members.length,
      limit: 24,
      truncated: false,
    },
  }));
  const publicationPath = `/me/bundles/${OWN_BUNDLE.bundle}/publication`;
  const updated: Publication = {
    ...publication,
    status: action === "delist" ? "delisted" : "published",
    status_changed_at: "2026-09-20T00:00:00Z",
    availability:
      action === "delist"
        ? {
            value: "delisted",
            label: "作者已撤回",
            note: "作者撤回了這個發佈物，這一頁不再提供它的內容。",
          }
        : publication.availability,
    acquisition:
      action === "delist"
        ? { available: false, note: "作者撤回了這個發佈物，這一頁不再提供它的內容。" }
        : publication.acquisition,
    releases:
      action === "publish"
        ? [
            {
              ...publication.releases[0],
              bundle_version: "1.1.0",
              content_hash: OWN_BUNDLE.content_hash,
              released_at: "2026-09-20T00:00:00Z",
            },
          ]
        : publication.releases,
  };
  let releaseWrite!: () => void;
  let releaseRead!: () => void;
  const writeGate = new Promise<void>((resolve) => {
    releaseWrite = resolve;
  });
  const readGate = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  let reads = 0;
  let writes = 0;
  const methods: string[] = [];
  const requests: unknown[] = [];
  page.route("**/me/bundles", () => ({ json: { bundles: [OWN_BUNDLE, older] } }));
  page.route("**/me/bundles/overview", async () => {
    reads++;
    const current = reads;
    if (current === 2) await readGate;
    return current === 2 && readFailure
      ? { status: readFailure === "invalid" ? 200 : 503, json: {} }
      : { json: overview(current === 1 ? publication : updated) };
  });
  page.route(`**${publicationPath}`, async ({ method, body }) => {
    methods.push(method);
    if (action === "publish") requests.push(body);
    writes++;
    await writeGate;
    return { status, json: status === 200 ? updated : { error: "unavailable" } };
  });
  return {
    releaseWrite,
    releaseRead,
    reads: () => reads,
    writes: () => writes,
    methods,
    requests,
    unexpectedWrites: () =>
      page.requests.filter(
        (request) =>
          !["GET", "HEAD"].includes(request.method) && request.url.pathname !== publicationPath,
      ),
  };
}

for (const action of ["publish", "delist"] as const) {
  for (const outcome of [200, 503, "unavailable", "invalid"] as const) {
    test(`Bundle ${action} ${outcome} reconciles immutable versions`, async () => {
      const page = app();
      const failure = typeof outcome === "string" ? outcome : undefined;
      const fixture = stubOperation(
        page,
        action,
        typeof outcome === "number" ? outcome : 503,
        failure,
      );
      try {
        await page.goto("/workspace/downloads");
        const card = page.locator(`article[aria-labelledby="bundle-${OWN_BUNDLE.bundle}"]`);
        const publish = card.getByRole("button", { name: "發佈 v1.1.0", exact: true });
        const sibling = card.getByRole("button", { name: "發佈 v1.0.0", exact: true });
        await expect(publish).toBeEnabled();
        await expect(sibling).toBeEnabled();
        expect(fixture.reads()).toBe(1);
        if (action === "delist")
          await card.getByRole("button", { name: "撤回", exact: true }).click();
        await card
          .getByRole("button", {
            name: action === "publish" ? "發佈 v1.1.0" : "確認撤回",
            exact: true,
          })
          .click();
        await page.eventually(() => expect(fixture.writes()).toBe(1));
        await expect(sibling).toBeDisabled();
        await expect(
          card.getByRole("button", {
            name: action === "publish" ? "撤回" : "發佈 v1.1.0",
            exact: true,
          }),
        ).toBeDisabled();
        await expect(
          card.getByRole("button", { name: "匯出 v1.0.0 Plugin", exact: true }),
        ).toBeEnabled();
        await expect(
          card.getByRole("heading", { name: OWN_BUNDLE.bundle, exact: true }),
        ).toBeFocused();
        await page.locator(".bundle-create summary").click();
        const draft = page
          .locator(".bundle-form")
          .getByRole("textbox", { name: "名稱", exact: true });
        await draft.fill("kept-draft");
        fixture.releaseWrite();
        await page.eventually(() => expect(fixture.reads()).toBe(2));
        await expect(sibling).toBeDisabled();
        await expect(card).toContainText("核對 Bundle 發佈狀態");
        fixture.releaseRead();
        if (failure) {
          const section = page.locator("#bundle-workspace");
          await expect(section.getByRole("alert")).toContainText("暫時無法讀取Bundle 發佈概覽");
          await expect(card.locator(".bundle-publication")).toHaveCount(0);
          await expect(card).not.toContainText("上面的狀態已重新讀取");
          await section.getByRole("button", { name: "再試一次", exact: true }).click();
          await expect(page.locator("#bundle-workspace-heading")).toBeFocused();
        }
        await expect(sibling).toBeEnabled();
        if (!failure) await expect(draft).toBeFocused();
        await expect(card.locator(".bundle-overview-header [role=status]")).toHaveText(
          action === "publish" ? "已發佈" : "已撤回",
        );
        await expect(
          card.getByText(action === "delist" ? "目前不提供套件" : "目前提供套件", { exact: true }),
        ).toBeVisible();
        await expect(
          card.getByText(action === "delist" ? "目前提供套件" : "目前不提供套件", { exact: true }),
        ).toHaveCount(0);
        await expect(
          card
            .locator(".bundle-publication-grid div")
            .filter({ has: page.getByText("目前 Release", { exact: true }) }),
        ).toContainText(action === "publish" ? "v1.1.0" : "v1.0.0");
        if (outcome !== 200) {
          await expect(card.getByRole("alert")).toContainText(
            `尚未確認${action === "publish" ? "發佈" : "撤回"}是否已完成`,
          );
          const alert = card.getByRole("alert").element();
          const versions = card.locator(".bundle-version-list").element();
          expect(
            Boolean(alert.compareDocumentPosition(versions) & Node.DOCUMENT_POSITION_FOLLOWING),
          ).toBe(true);
        }
        expect(fixture.reads()).toBe(failure ? 3 : 2);
        expect(fixture.writes()).toBe(1);
        expect(fixture.methods).toEqual([action === "publish" ? "POST" : "DELETE"]);
        expect(fixture.requests).toEqual(
          action === "publish" ? [{ version: "1.1.0", rights_attested: false }] : [],
        );
        expect(fixture.unexpectedWrites()).toEqual([]);
        await expect(draft).toHaveValue("kept-draft");
        expect(await page.axeViolations("main")).toEqual([]);
      } finally {
        fixture.releaseWrite();
        fixture.releaseRead();
      }
    });
  }
}
