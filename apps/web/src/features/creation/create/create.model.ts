import type { CreationSnapshot } from "../creation.service";

export function diagramProblem(file: File): string | undefined {
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type))
    return "流程圖只收 PNG、JPEG 或 WebP。";
  const maxBytes = 4_000_000; // one-number: creationMaxDiagramBytes
  if (file.size === 0 || file.size > maxBytes)
    return "請選擇最多 4,000,000 位元組（約 3.8 MB）以內的流程圖。";
  return undefined;
}

export function readImage(file: File): Promise<{ media_type: string; data: string }> {
  const problem = diagramProblem(file);
  if (problem) return Promise.reject(new Error(problem));
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("流程圖無法讀取，請重新選擇。"));
    reader.onload = () =>
      resolve({ media_type: file.type, data: String(reader.result).split(",")[1] });
    reader.readAsDataURL(file);
  });
}

type RunObservation = {
  execution_status: string;
  evaluation?: {
    evaluation_available: boolean;
    overall?: string;
    criterion_results?: { result?: string }[];
  };
};

export function findRunObservation(
  messages: CreationSnapshot["messages"],
  runID: string,
): RunObservation | undefined {
  let found: RunObservation | undefined;
  for (const m of messages) {
    if (m.role !== "tool") continue;
    try {
      const parsed = JSON.parse(m.content) as RunObservation & { run_id?: string };
      if (parsed.run_id === runID) found = parsed;
    } catch {}
  }
  return found;
}

export type DiagramUnderstanding = {
  nodes: string[];
  conditions: string[];
  branches: string[];
  uncertainties: string[];
};

export const diagramSections: (keyof DiagramUnderstanding)[] = [
  "nodes",
  "conditions",
  "branches",
  "uncertainties",
];

export function parseDiagramUnderstanding(raw: string): DiagramUnderstanding | undefined {
  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
    const record = value as Record<string, unknown>;
    if (Object.keys(record).length !== diagramSections.length) return undefined;
    if (!diagramSections.every((key) => Array.isArray(record[key]))) return undefined;
    const sections = Object.fromEntries(
      diagramSections.map((key) => [key, record[key] as unknown[]]),
    ) as Record<keyof DiagramUnderstanding, unknown[]>;
    if (sections.nodes.length === 0) return undefined;
    if (!diagramSections.every((key) => sections[key].every((item) => typeof item === "string")))
      return undefined;
    return Object.fromEntries(
      diagramSections.map((key) => [key, sections[key] as string[]]),
    ) as DiagramUnderstanding;
  } catch {
    return undefined;
  }
}

export function stepDescription(p: CreationSnapshot): string {
  if (p.pending_fetch_url) return "正在讀你同意的那個網頁，讀完再繼續。";
  if (p.diagram_fingerprint && !p.diagram_understanding) return "正在讀你附上的流程圖。";
  if (!p.brief_confirmed) return "正在整理需求與驗收條件。";
  if (!p.draft) return "正在寫第一份草稿。";
  if (p.run_unmet) return "正在依試跑結果修訂草稿。";
  return "正在修訂草稿。";
}

export const FETCH_STATUS_LABEL: Record<string, string> = {
  ok: "已讀取",
  blocked: "被拒絕或被網路環境擋住（不重試）",
  not_found: "頁面不存在",
  unsupported: "不是文字頁面",
  network_error: "網路錯誤（重試一次仍失敗）",
  declined: "使用者不同意",
};

export const ROUND_OVERALL_LABEL: Record<string, string> = {
  met: "達成",
  partially_met: "部分達成",
  not_met: "未達成",
};

function truncateForTimeline(s: string, n = 120): string {
  return s.length > n ? s.slice(0, n) + "…" : s;
}

type TimelineItem = { key: string; text: string };

export function buildRoundTimeline(messages: CreationSnapshot["messages"]): TimelineItem[] {
  const items: TimelineItem[] = [];
  let round = 0;
  let afterQuestion = false;
  let afterTrialAnchor = false;
  messages.forEach((m, i) => {
    let setQuestion = m.role === "tool" && afterQuestion;
    let setTrialAnchor = m.role === "tool" && afterTrialAnchor;
    if (m.role === "tool" && m.content.startsWith('{"evaluation"')) {
      try {
        const parsed = JSON.parse(m.content) as RunObservation;
        if (parsed.evaluation) {
          round += 1;
          const overall = parsed.evaluation.overall ?? "";
          const results = parsed.evaluation.criterion_results ?? [];
          const count = (r: string) => results.filter((x) => x.result === r).length;
          items.push({
            key: `t-${i}`,
            text: `第 ${round} 次試跑：${ROUND_OVERALL_LABEL[overall] ?? overall}（通過 ${count("passed")}／不通過 ${count("failed")}／無法判定 ${count("undetermined")}）`,
          });
          setQuestion = false;
          setTrialAnchor = true;
        }
      } catch {}
    } else if (m.role === "tool" && m.content.startsWith('{"fetch"')) {
      try {
        const parsed = JSON.parse(m.content) as { fetch: { url: string; status: string } };
        items.push({
          key: `t-${i}`,
          text: `讀取網頁：${parsed.fetch.url}（${FETCH_STATUS_LABEL[parsed.fetch.status] ?? parsed.fetch.status}）`,
        });
      } catch {}
    } else if (m.role === "assistant" && m.content.startsWith("這次試跑有條件沒過")) {
      items.push({ key: `t-${i}`, text: `系統問你：${m.content}` });
      setQuestion = true;
    } else if (m.role === "user" && afterQuestion) {
      items.push({ key: `t-${i}`, text: `你回答：${truncateForTimeline(m.content)}` });
      setTrialAnchor = true;
    } else if (m.role === "assistant" && afterTrialAnchor) {
      items.push({ key: `t-${i}`, text: `模型建議：${m.content.slice(0, 120)}` });
    }
    afterQuestion = setQuestion;
    afterTrialAnchor = setTrialAnchor;
  });
  return items;
}

export interface NextStepBudget {
  costCredits: number;
  remainingCredits: number;
  roomForAnother: boolean;
}

export function nextStepBudget(
  progress: { budget_credits: number; reserved_credits: number; spent_credits?: number },
  perStepCredits: number,
): NextStepBudget {
  const remaining =
    progress.budget_credits - (progress.spent_credits ?? 0) - progress.reserved_credits;
  return {
    costCredits: perStepCredits,
    remainingCredits: Math.max(remaining, 0),
    roomForAnother: remaining >= perStepCredits,
  };
}

export function budgetChoices(min: number, max: number) {
  return [...new Set([min, 200, 500, 1000, 2000, 5000, max])]
    .filter((v) => v >= min && v <= max)
    .sort((a, b) => a - b);
}

export function newestMessageIn(pane: HTMLElement) {
  const messages = pane.querySelectorAll<HTMLElement>(".creation-log > li[data-index]");
  return messages[messages.length - 1] as HTMLElement | undefined;
}

export function isBelow(el: HTMLElement | undefined, pane: HTMLElement) {
  return !!el && el.getBoundingClientRect().top > pane.getBoundingClientRect().bottom;
}
