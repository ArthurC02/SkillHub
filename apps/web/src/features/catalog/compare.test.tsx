import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { onlineManager, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { RunCompare } from "../runs/compare/RunCompare.page";
import { Compare, CompareTable } from "./compare/Compare.page";
import {
  COMPARISON,
  comparisonSide,
  OTHER_RUN,
  RUN,
  RUNS,
  SKILL,
  SKILL_B,
  SKILL_C,
  SKILL_VERSIONS,
  TEST_CASE,
  platformResponse,
  skillDetail,
} from "../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";

const FOREIGN_RUN = {
  ...RUNS.runs[1],
  run_id: "0f0f0f0f-1111-4222-8333-444455556666",
  status_reason: "這是別的測試題的試跑紀錄。",
  created_at: "2026-01-01T00:00:00Z",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  search = {};
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
  navigations.length = 0;
});

const navigations: { search?: { against?: string } }[] = [];

let search: { against?: string; ids?: string } = {};

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ runId: RUN }),
  useSearch: () => search,
  useNavigate: () => (options: { search?: { against?: string } }) => {
    navigations.push(options);
    return Promise.resolve();
  },
  Link: ({ children }: { children?: unknown }) => children as never,
}));

const pathOf = (url: string) => url.replace(/^https?:\/\/[^/]+/, "").split("?")[0];

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

function stubPlatform(calls?: string[]) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls?.push(url);
    if (pathOf(url) === "/runs") {
      return json(url.includes(`test_case_id=${TEST_CASE}`) ? RUNS : { runs: [FOREIGN_RUN] });
    }
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
}

test("EVAL-003 waits for the 測試題 before loading candidate runs", async () => {
  const calls: string[] = [];
  stubPlatform(calls);
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  const runLists = calls.filter((url) => pathOf(url) === "/runs");
  expect(runLists.length).toBeGreaterThan(0);
  expect(runLists.every((url) => url.includes(`test_case_id=${TEST_CASE}`))).toBe(true);
});

test.each([
  ["canonical source", "canonical", false, "比較基準：v1"],
  ["case-equivalent source", "uppercase", false, "比較基準：v1"],
  ["unmapped source", "missing", false, "比較基準：版本編號未知"],
  ["unreadable metadata", "canonical", true, "比較基準：版本編號未知"],
] as const)(
  "comparison identifies its %s without borrowing a candidate version",
  async (_name, identity, failed, expected) => {
    const sourceVersion = {
      ...SKILL_VERSIONS.versions.find((version) => version.version_number === 1)!,
      version_id: "abcdefab-1111-4111-8111-111111111111",
    };
    const versionId =
      identity === "missing"
        ? "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
        : identity === "uppercase"
          ? sourceVersion.version_id.toUpperCase()
          : sourceVersion.version_id;
    const reads: string[] = [];
    vi.stubGlobal("fetch", (input: string) => {
      const url = String(input);
      reads.push(url);
      const response = platformResponse(url);
      if (pathOf(url) === `/runs/${RUN}`)
        return json({ ...(response.body as object), skill_version_id: versionId });
      if (failed && pathOf(url) === `/skills/${SKILL}/versions`)
        return json({ error: "service unavailable" }, 503);
      if (pathOf(url) === `/skills/${SKILL}/versions`)
        return json({
          versions: SKILL_VERSIONS.versions.map((version) =>
            version.version_number === 1 ? sourceVersion : version,
          ),
        });
      return json(response.body, response.status);
    });
    await render(<RunCompare />);
    await waitFor(() =>
      Array.from(container.querySelectorAll("p")).some((node) => node.textContent === expected),
    );
    expect(
      Array.from(container.querySelectorAll("p")).some((node) => node.textContent === expected),
    ).toBe(true);
    expect(text()).not.toContain("比較基準：v2");
    expect(reads.filter((url) => pathOf(url) === `/skills/${SKILL}/versions`)).toHaveLength(1);
  },
);

test("EVAL-003 comparison does not fetch a version diff outside the platform", async () => {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls.push(url);
    if (pathOf(url).endsWith("/comparison"))
      return json({ ...COMPARISON, version_diff_url: "https://example.test/version-diff" });
    if (pathOf(url) === "/runs") {
      return json(url.includes(`test_case_id=${TEST_CASE}`) ? RUNS : { runs: [FOREIGN_RUN] });
    }
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("無法讀取版本差異"));

  expect(calls.some((url) => url.includes("example.test/version-diff"))).toBe(false);
});

async function render(node: ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() => queryClient.isFetching() === 0 && (container.textContent ?? "").length > 0);
}

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs, { flushBeforeFirstCheck: true });
}

const text = () => container.textContent ?? "";

function rowOf(heading: string) {
  const row = Array.from(container.querySelectorAll("tbody tr")).find(
    (candidate) => candidate.querySelector("th")?.textContent?.replace("有差異", "") === heading,
  );
  expect(row, `${heading} row`).toBeDefined();
  return row!;
}

const cellOf = (heading: string, column: number) =>
  rowOf(heading).querySelectorAll("td")[column].textContent;

const candidateButtons = () =>
  Array.from(container.querySelectorAll("button")).filter((b) =>
    (b.textContent ?? "").startsWith("與這一次比較"),
  );

test("DISC-009 comparison gives absent fields their actual state", async () => {
  const missingURL = skillDetail("a", "A");
  missingURL.source = { ...missingURL.source!, url: undefined };
  missingURL.version = undefined;
  missingURL.enrichment.tags = undefined;
  const pending = skillDetail("b", "B");
  pending.enrichment = { status: "pending", note: "正在處理" };
  await render(<CompareTable skills={[missingURL, pending]} />);

  expect(text()).toContain("來源網址：未測量");
  expect(text()).not.toContain("未提供");
  for (const heading of ["輸入", "輸出", "依賴"]) {
    const row = Array.from(container.querySelectorAll("tbody tr")).find((candidate) =>
      candidate.querySelector("th")?.textContent?.startsWith(heading),
    );
    expect(row, `${heading} row`).toBeDefined();
    const cells = row!.querySelectorAll("td");
    expect(cells[0].textContent, `${heading} enriched absence`).toBe("未測量");
    expect(cells[1].textContent, `${heading} pending absence`).toBe("處理中");
  }
  const versionRow = Array.from(container.querySelectorAll("tbody tr")).find((row) =>
    row.querySelector("th")?.textContent?.includes("版本與時間"),
  );
  expect(versionRow).toBeDefined();
  expect(versionRow?.querySelector("td")?.textContent).toBe("不適用");
});

test("DISC-009 source URLs become links only for HTTPS", async () => {
  const unsafe = skillDetail("a", "Unsafe");
  unsafe.source = { ...unsafe.source!, url: "http://example.test/source" };
  const safe = skillDetail("b", "Safe");
  safe.source = { ...safe.source!, url: "https://example.test/source" };
  await render(<CompareTable skills={[unsafe, safe]} />);

  expect(container.querySelector('a[href="http://example.test/source"]')).toBeNull();
  expect(container.querySelector('a[href="https://example.test/source"]')).not.toBeNull();
  expect(text()).toContain("http://example.test/source");
});

test("DISC-009 §2.9 量過而且是零的欄位印 0，不是未測量", async () => {
  const measuredZero = skillDetail("z", "Z");
  measuredZero.enrichment.tags = { inputs: [], outputs: ["markdown"], tools: [], dependencies: [] };
  measuredZero.limitations = [];
  await render(<CompareTable skills={[measuredZero, skillDetail("o", "O")]} />);

  expect(cellOf("輸入", 0), "模型量過、這個套件沒有輸入").toBe("0 項");
  expect(cellOf("依賴", 0), "模型量過、這個套件沒有依賴").toBe("0 項");
  expect(cellOf("限制", 0), "模型與掃描都跑過、兩邊都沒有發現").toBe("0 項");
});

test("DISC-009 §2.9 一邊處理中、一邊未測量：那一列要說有差異", async () => {
  const pending = skillDetail("p", "P");
  pending.enrichment = { status: "pending", note: "正在處理" };
  const enriched = skillDetail("e", "E");
  enriched.enrichment = { ...enriched.enrichment, tags: undefined };
  await render(<CompareTable skills={[pending, enriched]} />);

  expect(cellOf("輸入", 0)).toBe("處理中");
  expect(cellOf("輸入", 1)).toBe("未測量");
  expect(rowOf("輸入").querySelector("th")?.textContent).toContain("有差異");
});

test("DISC-009 §2.9 每一列的缺席各自說出自己的型別", async () => {
  const bare = skillDetail("b", "B");
  bare.summary = "";
  bare.allowed_tools = undefined;
  bare.source = undefined;
  bare.version = undefined;
  bare.enrichment = { status: "pending", note: "正在處理" };
  const scannedWithoutTools = skillDetail("s", "S");
  scannedWithoutTools.allowed_tools = undefined;
  await render(<CompareTable skills={[bare, scannedWithoutTools]} />);

  expect(cellOf("套件自述摘要", 0), "套件自己沒寫 description").toBe("不適用");
  expect(cellOf("白話摘要（AI 產生）", 0)).toBe("處理中");
  expect(cellOf("套件宣告可用的工具（權限）", 0), "沒有保存版本，套件從來沒被掃過").toBe("未測量");
  expect(cellOf("套件宣告可用的工具（權限）", 1), "掃過了，套件沒有宣告這個欄位").toBe("不適用");
  expect(cellOf("來源", 0)).toBe("不適用");
  expect(cellOf("版本與時間", 0)).toBe("不適用");
});

test("DISC-009 最小匯入的小工具：不印表外詞「未提供」", async () => {
  const minimal = skillDetail("m", "M");
  minimal.summary = "";
  minimal.allowed_tools = undefined;
  minimal.source = undefined;
  minimal.version = undefined;
  minimal.limitations = [];
  const full = skillDetail("f", "F");
  await render(<CompareTable skills={[minimal, full]} />);

  expect(text()).not.toContain("未提供");
  expect(text()).toContain("未測量");
});

test("DISC-009 生成的套件：來源網址是不適用，不是未測量", async () => {
  const generated = skillDetail("g", "G");
  generated.source = {
    ...generated.source!,
    url: undefined,
    trust: { value: "generated", label: "平台生成", note: "" },
  };
  const imported = skillDetail("i", "I");
  imported.source = { ...imported.source!, url: undefined };
  await render(<CompareTable skills={[generated, imported]} />);

  expect(text()).toContain("來源網址：不適用");
  expect(text()).toContain("來源網址：未測量");
});

test("DISC-009 相容性不同的兩個小工具,那一列要說有差異", async () => {
  const runs = skillDetail("a", "A");
  const doesNot = skillDetail("b", "B");
  doesNot.compatibility = {
    ...doesNot.compatibility,
    spec_validation: { value: "failed", label: "未通過", note: "" },
    capability: { value: "unverified", label: "未驗證", note: "" },
  };
  await render(<CompareTable skills={[runs, doesNot]} />);

  const row = Array.from(container.querySelectorAll("tbody tr")).find((candidate) =>
    candidate.querySelector("th")?.textContent?.startsWith("相容性"),
  );
  expect(row, "相容性 row").toBeDefined();
  expect(row!.querySelector("th")?.textContent).toContain("有差異");
  expect(row!.className).toContain("compare-differs");
});

test("EVAL-003 到站時就有同一個測試題的候選,而且認得出它的不是 uuid", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  expect(text()).toContain("任務判定：不符合");
  expect(
    candidateButtons()[0].querySelector('time[datetime="2026-08-16T00:00:00Z"]'),
  ).not.toBeNull();

  expect(text()).not.toContain("這是別的測試題的試跑紀錄。");

  const list = container.querySelector("ul.download-list");
  const identity = Array.from(list!.querySelectorAll("code")).find(
    (node) => node.textContent === OTHER_RUN,
  );
  expect(identity).toBeDefined();
  const disclosure = identity!.closest("details");
  expect(disclosure).not.toBeNull();
  expect(disclosure!.open).toBe(false);
  expect(disclosure!.querySelector("summary")?.textContent).toBe("試跑紀錄 ID");
  const surface = list!.cloneNode(true) as HTMLElement;
  surface.querySelectorAll("details").forEach((node) => node.remove());
  expect(surface.textContent).not.toContain(OTHER_RUN);

  expect(container.querySelector("#against")).not.toBeNull();
  expect(text()).not.toContain("這個測試題目前只有這一次試跑紀錄");
});

test("EVAL-003 自己不在候選裡:自比自伺服器會回 400", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  const buttons = candidateButtons();
  expect(buttons.length, "候選清單把這一邊自己也列了進去").toBe(1);
  expect(buttons.map((b) => b.textContent).join("|")).not.toContain("2026-08-17");
});

test("EVAL-003 選了候選就寫進網址,比較仍然可以被連結分享", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  await act(async () => candidateButtons()[0].click());

  expect(navigations.length).toBe(1);
  expect(navigations[0].search?.against).toBe(OTHER_RUN);
});

test("EVAL-003 只跑過一次的測試題:說原因,不是把控制項拿掉(設計 §2.4)", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === "/runs") return json({ runs: [RUNS.runs[0]] });
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  await render(<RunCompare />);
  await waitFor(() => text().includes("這個測試題目前只有這一次試跑紀錄"));

  expect(candidateButtons().length).toBe(0);
  expect(container.querySelector("#against")).not.toBeNull();
  expect(text()).toContain("輸入另一筆試跑紀錄的 ID 後開始比較。");
});

test("EVAL-003 目前試跑紀錄讀取失敗不會冒充沒有其他試跑紀錄", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === `/runs/${RUN}`) return json({ error: "boom" }, 500);
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  await render(<RunCompare />);
  await waitFor(() => text().includes("無法讀取目前這次試跑"));

  expect(text()).toContain("無法讀取目前這次試跑");
  expect(text()).not.toContain("這個測試題目前只有這一次試跑紀錄");
});

test("EVAL-003 候選歷史讀取失敗不會冒充空歷史", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === "/runs") return json({ error: "boom" }, 500);
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  await render(<RunCompare />);
  await waitFor(() => text().includes("無法讀取可比較的試跑紀錄"));

  expect(text()).toContain("無法讀取可比較的試跑紀錄");
  expect(text()).not.toContain("這個測試題目前只有這一次試跑紀錄");
});

const occurrences = (needle: string) => text().split(needle).length - 1;

const RUN_COST_AUTHORITY = "模型閘道對這次試跑的 per-key 實付";
const COST_NOTE = `這是下界，不是總額。權威來源：${RUN_COST_AUTHORITY}`;

test("§2.13 去重 1：兩側相同的成本但書印在列首，一次", async () => {
  stubPlatform();
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("試跑用掉的點數"));

  expect(occurrences(COST_NOTE), "同一句但書在左右兩格各印了一次").toBe(1);
  expect(occurrences("與上一列分開列，不相加。")).toBe(1);

  const heads = Array.from(container.querySelectorAll("th[scope=row]")).map(
    (th) => th.textContent ?? "",
  );
  expect(heads.some((h) => h.includes(COST_NOTE))).toBe(true);
  expect(heads.some((h) => h.includes("與上一列分開列，不相加。"))).toBe(true);
});

test("§2.13 去重 1：兩側的權威來源不同時，每一格各自留著自己的那一句", async () => {
  const mine = comparisonSide(RUN, true);
  const theirs = comparisonSide(OTHER_RUN, false);
  const differing = {
    ...COMPARISON,
    runs: [mine, { ...theirs, cost: { ...theirs.cost, authoritative_source: "另一個閘道的帳單" } }],
  };
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === "/runs") {
      return json(url.includes(`test_case_id=${TEST_CASE}`) ? RUNS : { runs: [FOREIGN_RUN] });
    }
    if (pathOf(url).endsWith("/comparison")) return json(differing);
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("試跑用掉的點數"));

  expect(text()).toContain(`權威來源：${RUN_COST_AUTHORITY}`);
  expect(text()).toContain("權威來源：另一個閘道的帳單");
  const heads = Array.from(container.querySelectorAll("th[scope=row]")).map(
    (th) => th.textContent ?? "",
  );
  expect(heads.some((h) => h.includes("權威來源："))).toBe(false);
});

test("§2.13：回答沒有人問的問題那一句刪了；重跑的但書縮了", async () => {
  stubPlatform();
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("試跑用掉的點數"));

  expect(text()).not.toContain("比較只是讀取");
  expect(text()).not.toContain("仍須在那裡確認一次才會開始試跑");
  expect(text()).toContain("（會先經過權限確認）");
});

test("§3 第 14 條：挑另一個試跑紀錄的說明在同一屏上只有一段", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  expect(occurrences("後開始比較")).toBe(1);
});

test("DISC-009 帶不到 2 個 id 到站是正常狀態，不是失敗", async () => {
  search = { ids: "a" };
  stubPlatform();
  await render(<Compare />);

  const notice = Array.from(container.querySelectorAll('[role="status"]')).find((el) =>
    (el.textContent ?? "").includes("請從首頁的搜尋結果或目錄選擇"),
  );
  expect(notice).toBeDefined();
  expect(
    Array.from(container.querySelectorAll('[role="alert"]')).some((el) =>
      (el.textContent ?? "").includes("請從首頁的搜尋結果或目錄選擇"),
    ),
  ).toBe(false);
});

function stubSkillReads(goneStatus: number, goneIds: string[]) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    const id = [SKILL, SKILL_B, SKILL_C].find((each) => pathOf(url) === `/api/skills/${each}`);
    if (id && goneIds.includes(id)) return json({ error: "gone" }, goneStatus);
    if (id) return json(skillDetail(id, `小工具 ${id.slice(0, 4)}`));
    const response = platformResponse(url);
    return json(response.body, response.status);
  });
}

const goneSentence =
  "小工具已從目錄下架、已被刪除，或你沒有權限看到，未列入下表；重新讀取也不會出現。";

test.each([410, 404])(
  "DISC-009 a compared skill answering %i is named as gone, without a retry that cannot work",
  async (status) => {
    search = { ids: [SKILL, SKILL_B, SKILL_C].join(",") };
    stubSkillReads(status, [SKILL_C]);
    await render(<Compare />);
    await waitFor(() => text().includes(goneSentence));

    expect(text()).toContain(`有 1 個${goneSentence}`);
    expect(text()).not.toContain("剩下的不夠 2 個");
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(
      Array.from(container.querySelectorAll("button")).map((b) => b.textContent),
    ).not.toContain("再試一次");
    expect(container.querySelectorAll("thead th")).toHaveLength(3);
  },
);

test("DISC-009 when the gone skills leave fewer than two, the page says to choose again", async () => {
  search = { ids: [SKILL, SKILL_B, SKILL_C].join(",") };
  stubSkillReads(410, [SKILL_B, SKILL_C]);
  await render(<Compare />);
  await waitFor(() => text().includes(goneSentence));

  expect(text()).toContain(
    `有 2 個${goneSentence}剩下的不夠 2 個可以比較，請從首頁的目錄重新選擇。`,
  );
  expect(container.querySelector(".compare-table")).toBeNull();
});

test("DISC-009 a transient read failure beside a gone skill still offers the retry", async () => {
  search = { ids: [SKILL, SKILL_B, SKILL_C].join(",") };
  stubSkillReads(503, [SKILL_B]);
  await render(<Compare />);
  await waitFor(() => container.querySelector('[role="alert"]') !== null);

  expect(container.querySelector('[role="alert"]')?.textContent).toContain("有 1 個小工具讀取失敗");
  expect(text()).not.toContain(goneSentence);
});

test("comparison names its horizontal scroll region and shows how to reveal hidden columns", async () => {
  search = { ids: "skill-a,skill-b" };
  stubPlatform();
  await render(<Compare />);
  await waitFor(() => container.querySelector(".compare-table") !== null);

  const region = container.querySelector<HTMLElement>('.table-scroll[role="region"]');
  expect(region?.getAttribute("aria-label")).toContain("左右捲動");
  expect(container.querySelector(".table-scroll-hint")?.textContent).toContain("左右捲動");
});

test.each(["initial", "cached", "retry"] as const)(
  "skill comparison explains its paused %s read and resumes without changing selection or focus",
  async (phase) => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    const ids = [SKILL, SKILL_B, SKILL_C];
    const names = ["更新摘要工具", "更新拆分工具", "更新清理工具"];
    const reads: string[] = [];
    let recovered = false;
    search = { ids: ids.join(",") };
    vi.stubGlobal("fetch", (input: string) => {
      const url = String(input);
      const index = ids.findIndex((id) => pathOf(url) === `/api/skills/${id}`);
      if (index >= 0) {
        reads.push(url);
        if (phase === "retry" && index === 1 && !recovered)
          return json({ error: "service unavailable" }, 503);
        return json(skillDetail(ids[index], names[index]));
      }
      const response = platformResponse(url);
      return json(response.body, response.status);
    });
    try {
      if (phase === "cached")
        ids.forEach((id) =>
          queryClient.setQueryData(queryKeys.skills.embedded(id), skillDetail(id, "舊小工具")),
        );
      if (phase === "retry")
        queryClient.setQueryData(
          queryKeys.skills.embedded(ids[1]),
          skillDetail(ids[1], "舊小工具"),
        );
      if (phase !== "retry") onlineManager.setOnline(false);
      await render(<Compare />);
      if (phase === "retry") {
        await waitFor(() => container.querySelector('[role="alert"]') !== null);
        expect(container.querySelectorAll("thead th")).toHaveLength(3);
        await act(async () => {
          onlineManager.setOnline(false);
          Array.from(container.querySelectorAll<HTMLButtonElement>("button"))
            .find((button) => button.textContent === "再試一次")!
            .click();
        });
      }
      await waitFor(() => text().includes("等待連線以讀取小工具比較資料"));
      expect(container.querySelector(".compare-table")).toBeNull();
      expect(container.querySelector('[role="alert"]')).toBeNull();
      const refresh = Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
        (button) => button.textContent === "等待連線…",
      )!;
      expect(refresh.getAttribute("aria-disabled")).toBe("true");
      refresh.focus();
      await act(async () => refresh.click());
      expect(reads).toHaveLength(phase === "retry" ? 3 : 0);
      await act(async () => {
        recovered = true;
        onlineManager.setOnline(true);
      });
      await waitFor(() => container.querySelectorAll("thead th").length === 4);
      expect(
        Array.from(container.querySelectorAll("thead th")).map((node) => node.textContent),
      ).toEqual(["比較項目", ...names]);
      expect(text()).not.toContain("等待連線以讀取小工具比較資料");
      expect(refresh.getAttribute("aria-disabled")).toBe("false");
      expect(document.activeElement).toBe(refresh);
      expect(search.ids).toBe(ids.join(","));
      expect(reads).toHaveLength(phase === "retry" ? 4 : 3);
    } finally {
      onlineManager.setOnline(true);
    }
  },
);
