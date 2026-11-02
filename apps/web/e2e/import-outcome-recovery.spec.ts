import { expect, test, type Page, type Request } from "@playwright/test";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import type { ImportResult } from "../src/core/api/types";
import { SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const RECEIPT: ImportResult = {
  shape: "skill",
  skills: [
    {
      path: "",
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 2,
      content_hash: "a".repeat(64),
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
};

function traceImportRequests(page: Page) {
  const timeline: string[] = [];
  const tracked = new WeakSet<Request>();
  const record = (event: string) => (request: Request) => {
    if (request.url().includes("/skills/import/")) tracked.add(request);
    if (tracked.has(request))
      timeline.push(
        `${new Date().toISOString()} page ${event} ${request.method()} ${request.failure()?.errorText ?? ""}`,
      );
  };
  page.on("request", record("request"));
  page.on("requestfinished", record("requestfinished"));
  page.on("requestfailed", record("requestfailed"));
  return timeline;
}

async function importReceiver(mode: string, requests: string[], timeline: string[]) {
  const receiver = createServer(async (request, response) => {
    timeline.push(`${new Date().toISOString()} receiver got ${request.method}`);
    response.on("finish", () =>
      timeline.push(`${new Date().toISOString()} receiver answered ${response.statusCode}`),
    );
    const headers = {
      "Access-Control-Allow-Origin": request.headers.origin ?? "http://127.0.0.1:4173",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Allow-Headers": "accept, content-type",
      "Content-Type": "application/json",
    };
    if (request.method === "OPTIONS") {
      response.writeHead(204, headers).end();
      return;
    }
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    requests.push(Buffer.concat(chunks).toString("utf8"));
    timeline.push(`${new Date().toISOString()} receiver read ${requests.at(-1)!.length} bytes`);
    if (requests.length > 1) {
      response.writeHead(201, headers).end(JSON.stringify(RECEIPT));
      return;
    }
    if (mode === "network") {
      response.writeHead(201, { ...headers, "Content-Length": "100" });
      response.write("{", () => response.destroy());
      return;
    }
    response.writeHead(201, headers).end(JSON.stringify(null));
  });
  await new Promise<void>((resolve, reject) => {
    receiver.once("error", reject);
    receiver.listen(0, "127.0.0.1", resolve);
  });
  return receiver;
}

for (const [mode, event] of [
  ["null", "requestfinished"],
  ["network", "requestfailed"],
] as const) {
  test(`import diagnostics retain ${event} after transfer without tracking unrelated requests`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.goto("/");
    const timeline = traceImportRequests(page);
    const requests: string[] = [];
    const receiver = await importReceiver(mode, requests, timeline);
    const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/import`;
    await page.route("**/skills/import/probe", (route) => route.continue({ url: endpoint }));
    await page.route("**/other/probe", (route) => route.continue({ url: endpoint }));
    try {
      const isProbe = (request: Request) =>
        request.method() === "POST" && request.url() === endpoint;
      const terminal =
        event === "requestfailed"
          ? page.waitForEvent("requestfailed", isProbe)
          : page.waitForEvent("requestfinished", isProbe);
      const result = await page.evaluate(() =>
        fetch("/skills/import/probe", { method: "POST", body: "diagnostic bytes" })
          .then((response) => response.text())
          .catch(() => "network error"),
      );
      expect((await terminal).url()).toBe(endpoint);
      expect(result).toBe(mode === "network" ? "network error" : "null");
      const unrelated = page.waitForEvent(
        "requestfinished",
        (request) => request.method() === "POST" && request.url() === endpoint,
      );
      await page.evaluate(() =>
        fetch("/other/probe", { method: "POST", body: "unrelated bytes" }).then((response) =>
          response.text(),
        ),
      );
      await unrelated;
      expect(requests).toEqual(["diagnostic bytes", "unrelated bytes"]);
      const recorded = timeline.filter((line) => line.includes(" page "));
      expect(recorded).toHaveLength(2);
      expect(recorded[0]).toContain("page request POST ");
      expect(recorded[1]).toContain(`page ${event} POST `);
      if (event === "requestfailed") expect(recorded[1]).toMatch(/POST .+/);
    } finally {
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}
