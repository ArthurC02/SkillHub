// Refuses a production bundle that talks to an absolute origin.
//
// # The failure this exists for
//
// `api/client.ts` resolves API_BASE_URL at BUILD time. It used to default to
// `http://localhost:8080` — the dev-server shape — and nothing on the clean
// test mode's path set VITE_API_BASE_URL. The resulting deployment rendered
// perfectly and sent every request to a port with nothing on it. The first
// symptom was `Failed to fetch` on a search.
//
// Nothing caught it, and the reason is a boundary rather than an oversight:
// every existing check measures the process and its environment. `envx`'s four
// idioms decide what an unset deployment variable means; the R-36 capability
// table declares what each one blocks and CI fails a variable that declares
// nothing; `/readyz` probes measure whether a capability actually works. This
// value is in none of those — it is not in .env.example, no Go code reads it,
// and the platform's own catalogue_search probe answered `ready` (truthfully:
// the server's search was healthy the whole time) while every browser was
// broken. The table measures what the deployment IS. This measures what it
// HANDS OUT.
//
// # Why it runs from `npm run build`
//
// Not from a CI step: `npm run build` is the one path every caller goes
// through — task build:web, task ci, the Playwright tier's own
// `npm run build && npm run preview`, and a developer building by hand. A check
// wired into CI only would let all four of the others produce the bad artifact.
//
// # The allowlist is stock, not an extension point
//
// Same shape as db/query-owners.yaml's `allow:` and the capability table's
// exemption ledger: every entry was measured in the bundle on 2026-09-02, is
// named with the library that emits it, and the list may only get shorter. A
// new absolute origin means somebody introduced one — that is the whole point,
// so it belongs in the diff, not in this list.
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const distAssets = join(dirname(fileURLToPath(import.meta.url)), "..", "dist", "assets");

/** Measured 2026-09-02. Each entry names what emits it. */
const STOCK = [
  // React DOM writes these as XML namespaces for SVG/MathML elements.
  "http://www.w3.org/1998/Math/MathML",
  "http://www.w3.org/1999/xlink",
  "http://www.w3.org/2000/svg",
  "http://www.w3.org/XML/1998/namespace",
  // React's minified-error decoder link.
  "https://react.dev/errors/",
  // @tanstack/react-router's fallback when `window.origin` is null (a sandboxed
  // or opaque origin). Bare, no port — the shape this check refuses always
  // carries a host:port or a real hostname.
  "http://localhost",
];

const origins = new Set();
for (const file of readdirSync(distAssets).filter((f) => f.endsWith(".js"))) {
  const text = readFileSync(join(distAssets, file), "utf8");
  for (const match of text.matchAll(/https?:\/\/[^"'`,)\s]*/g)) {
    origins.add(match[0]);
  }
}

const unexpected = [...origins].filter((o) => !STOCK.includes(o));
if (unexpected.length > 0) {
  console.error(
    [
      "",
      "這個 build 裡有預期之外的絕對網址：",
      ...unexpected.map((o) => `  ${o}`),
      "",
      "如果它是 API 的位址，這個 build 會把每一個請求送出這個部署——",
      "畫面照常顯示，第一個症狀是搜尋回 Failed to fetch，中間沒有任何紅燈。",
      "",
      "api/client.ts 的 API_BASE_URL 預設是空字串（同源），因為那是 cmd/api",
      "自己送出 SPA 時、以及 ADR-018 E1 的正式部署的形狀。開發伺服器的例外",
      "宣告在 apps/web/.env.development，Vite 只在 npm run dev 讀它。",
      "",
      "若它確實是函式庫發出的、與 API 無關，把它加進本檔的 STOCK 並寫明是誰發的。",
      "",
    ].join("\n"),
  );
  process.exit(1);
}
