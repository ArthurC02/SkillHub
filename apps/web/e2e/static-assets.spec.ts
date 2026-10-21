import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { expect, type Page, test } from "@playwright/test";
import { retryTransientNavigation, serveStatic, stubPlatform } from "./stub";

function droppingOrigin(drops: number, hits: number[]): Server {
  return createServer((request, response) => {
    hits.push(hits.length + 1);
    if (hits.length <= drops) {
      request.socket.destroy();
      return;
    }
    response.writeHead(200, { "Content-Type": "text/plain" }).end("served");
  });
}

async function listening(server: Server): Promise<string> {
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  return `http://127.0.0.1:${(server.address() as AddressInfo).port}/asset.txt`;
}

test("a static asset the origin drops twice is served on the third attempt", async ({ page }) => {
  const hits: number[] = [];
  const server = droppingOrigin(2, hits);
  const origin = await listening(server);
  try {
    await page.route("**/probe.txt", (route) => serveStatic(route, origin));
    await page.goto("/");
    const body = await page.evaluate(() => fetch("/probe.txt").then((reply) => reply.text()));
    expect([body, hits]).toEqual(["served", [1, 2, 3]]);
  } finally {
    server.close();
  }
});

test("a static asset the origin keeps dropping is refused with every failure named", async ({
  page,
}) => {
  const hits: number[] = [];
  const server = droppingOrigin(Infinity, hits);
  const origin = await listening(server);
  let served!: Promise<void>;
  try {
    await page.route("**/probe.txt", (route) => {
      served = serveStatic(route, origin);
      return served.catch(() => undefined);
    });
    await page.goto("/");
    const outcome = await page.evaluate(() =>
      fetch("/probe.txt").then(
        (reply) => `status ${reply.status}`,
        () => "network error",
      ),
    );
    await expect(served).rejects.toThrow(/probe\.txt could not be served after 3 attempts: /);
    expect([outcome, hits]).toEqual(["network error", [1, 2, 3]]);
  } finally {
    server.close();
  }
});

function navigatorFailing(failures: string[]) {
  const calls: string[] = [];
  const page = {
    goto: async (url: string) => {
      calls.push(url);
      const failure = failures.shift();
      if (failure) throw new Error(`page.goto: ${failure} at ${url}`);
      return null;
    },
  };
  retryTransientNavigation(page);
  return { page, calls };
}

test("a navigation the runner fails twice for lack of buffer space lands on the third attempt", async () => {
  const { page, calls } = navigatorFailing([
    "net::ERR_NO_BUFFER_SPACE",
    "net::ERR_NO_BUFFER_SPACE",
  ]);
  await page.goto("/workspace/runs");
  expect(calls).toEqual(["/workspace/runs", "/workspace/runs", "/workspace/runs"]);
});

test("a navigation that keeps failing for lack of buffer space is refused after three attempts", async () => {
  const { page, calls } = navigatorFailing(Array(5).fill("net::ERR_NO_BUFFER_SPACE"));
  await expect(page.goto("/workspace/runs")).rejects.toThrow("net::ERR_NO_BUFFER_SPACE");
  expect(calls).toHaveLength(3);
});

test("a navigation that fails for any other reason is not retried", async () => {
  const { page, calls } = navigatorFailing(["net::ERR_CONNECTION_REFUSED"]);
  await expect(page.goto("/workspace/runs")).rejects.toThrow("net::ERR_CONNECTION_REFUSED");
  expect(calls).toHaveLength(1);
});

test("every page the platform stub serves retries a navigation that ran out of buffer space", async () => {
  const failures = ["net::ERR_NO_BUFFER_SPACE"];
  let calls = 0;
  const page = {
    route: async () => {},
    goto: async (url: string) => {
      calls += 1;
      const failure = failures.shift();
      if (failure) throw new Error(`page.goto: ${failure} at ${url}`);
      return null;
    },
  };
  await stubPlatform(page as unknown as Page);
  await page.goto("/");
  expect(calls).toBe(2);
});
