import { existsSync, readFileSync } from "node:fs";

export function parseDotEnv(text) {
  const out = {};
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const eq = trimmed.indexOf("=");
    if (eq < 1) continue;
    const key = trimmed
      .slice(0, eq)
      .replace(/^export\s+/, "")
      .trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) continue;
    let value = trimmed.slice(eq + 1).trim();
    if (value.length > 1 && /^(".*"|'.*')$/.test(value)) {
      value = value.slice(1, -1);
    } else {
      // A `#` only starts a comment when preceded by whitespace, so `p#ss`
      // stays a value while `X=on # note` strips the trailing note.
      value = value.replace(/\s+#.*$/, "").trim();
    }
    out[key] = value;
  }
  return out;
}

export function readDotEnv(path) {
  return existsSync(path) ? parseDotEnv(readFileSync(path, "utf8")) : {};
}

// `||`, not `??`: an exported-but-empty shell variable must fall through to
// the file rather than winning as an explicit empty override.
export function resolve(dotEnv, shellEnv, name) {
  return shellEnv[name] || dotEnv[name] || "";
}

// `!shellEnv[k]`, not `=== undefined`, for the same reason resolve() uses
// `||`: an exported-but-empty shell variable must not shadow the file's value.
export function childOverlay(dotEnv, shellEnv) {
  return Object.fromEntries(
    Object.entries(dotEnv).filter(([k, v]) => !shellEnv[k] && v !== ""),
  );
}

export function releasePath(dotEnv, shellEnv, fallback) {
  return resolve(dotEnv, shellEnv, "SKILLHUB_CLEAN_MODE_RELEASES") || fallback;
}
