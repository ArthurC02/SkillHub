import { expect, test } from "vitest";
import { OWN_BUNDLE, platformResponse } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

function describedText(element: Element | null): string {
  return (element?.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ");
}

for (const missing of ["one member", "all members"]) {
  test(`Bundle redistribution stays unconfirmed with ${missing} absent and recovers through refresh`, async () => {
    const page = app();
    const base = (platformResponse("/skills").body as { skills: object[] }).skills[0];
    const rows = OWN_BUNDLE.members.map((member) => ({
      ...base,
      skill_id: member.skill_id,
      name: member.name,
    }));
    let reads = 0;
    const writes = () =>
      page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
    page.route("**/me/bundles", () => ({ json: { bundles: [OWN_BUNDLE] } }));
    page.route("**/me/bundles/overview", () => ({
      json: { bundles: [{ bundle: OWN_BUNDLE.bundle, latest_version: OWN_BUNDLE }] },
    }));
    page.route("**/skills?*", () => {
      reads++;
      const skills = reads > 1 ? rows : missing === "all members" ? [] : rows.slice(0, 1);
      return { json: { skills, total: skills.length, limit: 24, truncated: false } };
    });
    await page.goto("/workspace/downloads");
    const section = page.locator("#bundle-workspace");
    const articleSelector = `article[aria-labelledby="bundle-${OWN_BUNDLE.bundle}"]`;
    const article = section.locator(articleSelector);
    const publish = article.getByRole("button", {
      name: `首次發佈 v${OWN_BUNDLE.version}`,
      exact: true,
    });
    await expect(publish).toHaveCount(1);
    await expect(publish).toBeDisabled();
    expect(describedText(publish.element())).toMatch(/散布需求尚未確認/);
    await expect(article.getByText("散布需求尚未確認", { exact: false })).toBeVisible();
    await expect(section.getByRole("alert")).toHaveCount(0);
    expect(await page.axeViolations(articleSelector)).toEqual([]);
    await section.getByRole("button", { name: "重新整理", exact: true }).click();
    await expect(publish).toBeEnabled();
    await expect(article).not.toContainText("散布需求尚未確認");
    expect(reads).toBe(2);
    expect(writes()).toEqual([]);
    expect(await page.axeViolations(articleSelector)).toEqual([]);
  });
}
