import AxeBuilder from "@axe-core/playwright";
import { test, type TestInfo } from "@playwright/test";
import type { AxeResults } from "axe-core";

const NOT_AUDITED_HERE = "axe runs on the Linux chromium project only";

export function auditsAccessibility(info: TestInfo): boolean {
  if (info.project.name !== "chromium") return false;
  return !process.env.CI || process.platform === "linux";
}

export default class ProjectScopedAxeBuilder extends AxeBuilder {
  override async analyze(): Promise<AxeResults> {
    const info = test.info();
    if (auditsAccessibility(info)) return super.analyze();
    if (!info.annotations.some((a) => a.description === NOT_AUDITED_HERE)) {
      info.annotations.push({ type: "accessibility", description: NOT_AUDITED_HERE });
    }
    return {
      testEngine: { name: "axe-core", version: "not run" },
      testRunner: { name: NOT_AUDITED_HERE },
      testEnvironment: { userAgent: "", windowWidth: 0, windowHeight: 0 },
      url: info.project.name,
      timestamp: new Date().toISOString(),
      toolOptions: {},
      passes: [],
      violations: [],
      incomplete: [],
      inapplicable: [],
    };
  }
}
