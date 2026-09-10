import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const distAssets = join(dirname(fileURLToPath(import.meta.url)), "..", "dist", "assets");

const STOCK = [
  "http://www.w3.org/1998/Math/MathML",
  "http://www.w3.org/1999/xlink",
  "http://www.w3.org/2000/svg",
  "http://www.w3.org/XML/1998/namespace",
  "https://react.dev/errors/",
  "http://localhost",
];

const relativeOrigin = /["'`](\/\/[A-Za-z0-9][A-Za-z0-9._-]*(?::\d+)?)(?=[/"'`?#])/g;

const origins = new Set();
const relative = new Set();
for (const file of readdirSync(distAssets).filter((f) => f.endsWith(".js"))) {
  const text = readFileSync(join(distAssets, file), "utf8");
  for (const match of text.matchAll(/https?:\/\/[^"'`,)\s]*/g)) {
    origins.add(match[0]);
  }
  for (const match of text.matchAll(relativeOrigin)) {
    relative.add(match[1]);
  }
}

const unexpected = [...origins].filter((o) => !STOCK.includes(o)).concat([...relative]);
if (unexpected.length > 0) {
  console.error(
    [
      "",
      "這個 build 裡有預期之外的絕對網址：",
      ...unexpected.map((o) => `  ${o}`),
      "",
      "（`//` 開頭的也算：瀏覽器只補上通訊協定，請求一樣離開這個部署。）",
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
