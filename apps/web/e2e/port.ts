import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";

const SHARED_PORT = 4173;
const PORT_SPREAD = 800;

export function previewPort(
  checkout: string = process.cwd(),
  ci: boolean = Boolean(process.env.CI),
): number {
  if (ci) return SHARED_PORT;
  const digest = createHash("sha256").update(checkout.toLowerCase()).digest();
  return SHARED_PORT + 1 + (digest.readUInt16BE(0) % PORT_SPREAD);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  console.log(previewPort());
}
