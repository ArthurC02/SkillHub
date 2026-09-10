import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const dir = join(import.meta.dirname);

function tsxFiles(at: string): Array<[string, string]> {
  const out: Array<[string, string]> = [];
  for (const entry of readdirSync(at, { withFileTypes: true })) {
    const path = join(at, entry.name);
    if (entry.isDirectory()) out.push(...tsxFiles(path));
    else if (entry.name.endsWith(".tsx") && !entry.name.endsWith(".test.tsx")) {
      out.push([path, readFileSync(path, "utf8")]);
    }
  }
  return out;
}

const UNTRUSTED_PRE = /<pre className="(?:skill-md|diff)"[^>]*>\s*([^\s])/g;

test("every <pre> that shows somebody else's text goes through Reveal (04 丙-210)", () => {
  const offenders: string[] = [];
  for (const [path, source] of tsxFiles(dir)) {
    for (const m of source.matchAll(UNTRUSTED_PRE)) {
      if (m[1] !== "<") {
        offenders.push(path + ": " + m[0].replace(/\s+/g, " "));
      }
    }
  }
  expect(offenders, "這些地方把別人寫的文字直接倒進 <pre>，隱藏字元會照原樣消失在畫面上").toEqual(
    [],
  );
});

test("the roster is not empty — the scan must actually be finding these sites", () => {
  const found = tsxFiles(dir).filter(([, s]) => [...s.matchAll(UNTRUSTED_PRE)].length > 0).length;
  expect(found, "掃描找不到任何一處，那上面那條就是永遠綠的").toBeGreaterThanOrEqual(5);
});
