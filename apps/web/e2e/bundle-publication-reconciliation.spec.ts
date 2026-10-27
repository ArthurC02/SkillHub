import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { OWN_BUNDLE, OWN_PUBLICATION, PUBLISHER } from "../src/testing/fixtures/platform";
import type { Publication } from "../src/features/publishing/publishing.service";
import { stubPlatform } from "./stub";

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

async function stubOperation(
  page: Page,
  action: "publish" | "delist",
  status: number,
  readFailure?: "unavailable" | "invalid",
) {
  await stubPlatform(page);
  const unexpectedWrites: string[] = [];
  await page.route("**/*", (route) => {
    if (["GET", "HEAD"].includes(route.request().method())) return route.fallback();
    unexpectedWrites.push(route.request().url());
    return route.fulfill({ status: 500, json: { error: "unexpected write" } });
  });
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
  const requests: unknown[] = [];
  await page.route("**/me/bundles", (route) =>
    route.fulfill({ json: { bundles: [OWN_BUNDLE, older] } }),
  );
  await page.route("**/me/bundles/overview", async (route) => {
    reads++;
    const current = reads;
    if (current === 2) await readGate;
    return current === 2 && readFailure
      ? route.fulfill({ status: readFailure === "invalid" ? 200 : 503, json: {} })
      : route.fulfill({ json: overview(current === 1 ? publication : updated) });
  });
  await page.route(`**/me/bundles/${OWN_BUNDLE.bundle}/publication`, async (route) => {
    expect(route.request().method()).toBe(action === "publish" ? "POST" : "DELETE");
    if (action === "publish") requests.push(route.request().postDataJSON());
    writes++;
    await writeGate;
    return route.fulfill({ status, json: status === 200 ? updated : { error: "unavailable" } });
  });
  return {
    releaseWrite,
    releaseRead,
    reads: () => reads,
    writes: () => writes,
    requests,
    unexpectedWrites,
  };
}

for (const width of [375, 1280]) {
  for (const action of ["publish", "delist"] as const) {
    for (const outcome of [200, 503, "unavailable", "invalid"] as const) {
      test(`Bundle ${action} ${outcome} reconciles immutable versions at ${width}px`, async ({
        page,
      }) => {
        const failure = typeof outcome === "string" ? outcome : undefined;
        const fixture = await stubOperation(
          page,
          action,
          typeof outcome === "number" ? outcome : 503,
          failure,
        );
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        await Promise.all([
          page.waitForResponse(
            (response) => new URL(response.url()).pathname === "/me/bundles/overview",
          ),
          page.goto("/workspace/downloads"),
        ]);
        const card = page.getByRole("article", { name: OWN_BUNDLE.bundle, exact: true });
        const publish = card.getByRole("button", { name: "發佈 v1.1.0", exact: true });
        const sibling = card.getByRole("button", { name: "發佈 v1.0.0", exact: true });
        await expect(publish).toBeEnabled();
        await expect(sibling).toBeEnabled();
        if (action === "delist")
          await card.getByRole("button", { name: "撤回", exact: true }).click();
        await card
          .getByRole("button", {
            name: action === "publish" ? "發佈 v1.1.0" : "確認撤回",
            exact: true,
          })
          .click();
        await expect.poll(fixture.writes).toBe(1);
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
        await expect.poll(fixture.reads).toBe(2);
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
          expect(
            await card
              .getByRole("alert")
              .evaluate((alert) =>
                Boolean(
                  alert.compareDocumentPosition(
                    alert.closest("article")!.querySelector(".bundle-version-list")!,
                  ) & Node.DOCUMENT_POSITION_FOLLOWING,
                ),
              ),
          ).toBe(true);
        }
        expect(fixture.reads()).toBe(failure ? 3 : 2);
        expect(fixture.writes()).toBe(1);
        expect(fixture.requests).toEqual(
          action === "publish" ? [{ version: "1.1.0", rights_attested: false }] : [],
        );
        expect(fixture.unexpectedWrites).toEqual([]);
        await expect(draft).toHaveValue("kept-draft");
        const accessibility = await new AxeBuilder({ page }).include("main").analyze();
        expect(accessibility.violations).toEqual([]);
        expect(accessibility.incomplete).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({
          path: test.info().outputPath(`bundle-${action}-${outcome}-${width}-viewport.png`),
        });
      });
    }
  }
}
