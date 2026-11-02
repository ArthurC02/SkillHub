import AxeBuilder from "./axe";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import type { CreationSession } from "../src/features/creation/creation.service";
import { ME, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const SESSION = "44444444-4444-4444-8444-444444444444";
const HASH = "e".repeat(64);

function session(validation: string, blocked: boolean): CreationSession {
  return {
    id: SESSION,
    revision: 7,
    state: "draft_ready",
    snapshot: {
      messages: [],
      brief: "整理輸入摘要",
      brief_confirmed: true,
      acceptance_criteria: ["列出三個重點"],
      diagram_understanding: "",
      diagram_confirmed: false,
      references: [],
      pending_action: "",
      budget_credits: 500,
      reserved_credits: 0,
      spent_credits: 10,
      usage_unknown: false,
      steps: 1,
      tool_calls: 0,
      draft: {
        revision: 7,
        content_hash: HASH,
        validation,
        blocked,
        skill: {
          name: "summary",
          description: "整理摘要",
          body: "# Summary\n\nRead and summarize.",
          compatibility: "",
          allowed_tools: "",
          files: [],
        },
      },
    },
    created_at: "2026-09-29T07:00:00Z",
    updated_at: "2026-09-29T08:00:00Z",
    expires_at: "2099-09-30T08:00:00Z",
    deadline: "2099-09-29T09:00:00Z",
  };
}

async function openDraft(page: Page, validation: string, blocked: boolean) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.addInitScript(() =>
    Object.defineProperty(window, "EventSource", { value: undefined, configurable: true }),
  );
  await page.route("**/me", (route) =>
    route.fulfill({ json: { ...ME, features: { generate_skill: true, creation_skill: true } } }),
  );
  await page.route("**/me/credits", (route) => route.fulfill({ json: { balance_credits: 2000 } }));
  let current = session(validation, blocked);
  let mode = "initial";
  let reads = 0;
  const writes: Record<string, unknown>[] = [];
  let release!: () => void;
  let started!: () => void;
  const heldRead = new Promise<void>((resolve) => {
    release = resolve;
  });
  const readStarted = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route("**/creation-sessions**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/limits"))
      return route.fulfill({
        json: {
          min_budget_credits: 130,
          max_budget_credits: 6500,
          max_steps: 20,
          max_tool_calls: 10,
          call_timeout_seconds: 120,
          session_timeout_seconds: 3600,
          retention_seconds: 604800,
        },
      });
    if (request.method() === "POST") {
      writes.push(request.postDataJSON());
      current = {
        ...current,
        revision: 8,
        state: "candidate_ready",
        snapshot: { ...current.snapshot, candidate: { skill_id: SKILL, version_id: VERSION } },
      };
      return route.fulfill({ json: current });
    }
    if (path.endsWith(SESSION)) {
      reads += 1;
      if (mode === "error") {
        started();
        await heldRead;
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      }
      if (mode === "recovered")
        current = session(JSON.stringify({ findings: null, blocked: false }), false);
      return route.fulfill({ json: current });
    }
    return route.fulfill({ json: [current] });
  });
  await page.goto(`/workspace/creations?session=${SESSION}`);
  return {
    writes,
    get reads() {
      return reads;
    },
    readStarted,
    release,
    failNextRead: () => {
      mode = "error";
    },
    recover: () => {
      mode = "recovered";
    },
  };
}

async function sampleComposerContrast(page: Page) {
  const monitor = await page.evaluateHandle(() => {
    const composer = document.querySelector<HTMLElement>(".composer")!;
    const button = composer.querySelector<HTMLButtonElement>(".composer-send")!;
    button.getAnimations().forEach((animation) => animation.finish());
    button.getBoundingClientRect();
    type Sample = { phase: number; foreground: string; background: string; ratio: number };
    const sampled = new Promise<{ transitions: string[]; samples: Sample[] }>((resolve) => {
      const observer = new MutationObserver(() => {
        if (composer.hasAttribute("data-empty")) return;
        observer.disconnect();
        button.getBoundingClientRect();
        const transitions = button
          .getAnimations()
          .filter(
            (animation): animation is CSSTransition =>
              "transitionProperty" in animation &&
              typeof animation.transitionProperty === "string" &&
              ["background-color", "color"].includes(animation.transitionProperty),
          );
        transitions.forEach((animation) => animation.pause());
        const canvas = document.createElement("canvas");
        canvas.width = canvas.height = 1;
        const context = canvas.getContext("2d", { willReadFrequently: true })!;
        const luminance = (color: string) => {
          context.fillStyle = color;
          context.fillRect(0, 0, 1, 1);
          const channels = [...context.getImageData(0, 0, 1, 1).data].slice(0, 3).map((value) => {
            const channel = value / 255;
            return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
          });
          return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
        };
        const samples = [0, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.4, 0.5, 0.75, 0.9].map((phase) => {
          transitions.forEach((animation) => {
            animation.currentTime = Number(animation.effect!.getTiming().duration) * phase;
          });
          const style = getComputedStyle(button);
          const foreground = style.color;
          const background = style.backgroundColor;
          const [high, low] = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
          return { phase, foreground, background, ratio: (high + 0.05) / (low + 0.05) };
        });
        const worst = samples.reduce((low, sample) => (sample.ratio < low.ratio ? sample : low));
        transitions.forEach((animation) => {
          animation.currentTime = Number(animation.effect!.getTiming().duration) * worst.phase;
        });
        resolve({
          transitions: transitions.map((animation) => animation.transitionProperty),
          samples,
        });
      });
      observer.observe(composer, { attributes: true, attributeFilter: ["data-empty"] });
    });
    return { sampled };
  });
  try {
    await page.locator("#creation-message").fill("保留目前的補充需求");
    return await monitor.evaluate(async ({ sampled }) => await sampled);
  } finally {
    await monitor.dispose();
  }
}

for (const colorScheme of ["light", "dark"] as const) {
  test(`the composer keeps readable text while becoming ready in ${colorScheme} mode`, async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme, reducedMotion: "no-preference" });
    const state = await openDraft(page, '{"findings":[', false);
    try {
      await expect(
        page.locator("#creation-draft-decision").getByText("檢查細節未確認", { exact: true }),
      ).toBeVisible();
      await expect(page.locator(".composer")).toHaveAttribute("data-empty", "true");
      const samples = await sampleComposerContrast(page);
      await writeFile(
        test.info().outputPath("composer-transition-contrast.json"),
        JSON.stringify(samples),
      );
      const report = await new AxeBuilder({ page }).analyze();
      await writeFile(
        test.info().outputPath("composer-transition-a11y.json"),
        JSON.stringify(report),
      );
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      expect(samples.samples.filter((sample) => sample.ratio < 4.5)).toEqual([]);
      expect(state.writes).toEqual([]);
      await page.screenshot({ path: test.info().outputPath("composer-ready-viewport.png") });
    } finally {
      state.release();
    }
  });
}
