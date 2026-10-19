import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { expect, test } from "@playwright/test";
import { serveStatic } from "./stub";

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
