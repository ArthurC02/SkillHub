import type { CreditBalance } from "../../../core/session/credits.service";
import type {
  CreationAction,
  CreationLimits,
  CreationSession,
  CreationSnapshot,
} from "../creation.service";
import type { CommandExtra } from "./create.commands";

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

export type CreationFocus = {
  title: string;
  description: string;
  target: string;
};

const CREATION_FOCUS_BY_ACTION: Partial<Record<string, CreationFocus>> = {
  confirm_brief: {
    title: "確認任務與成功條件",
    description: "核對小工具要完成的任務、驗收條件與範例輸入，再讓 Agent 繼續建構。",
    target: "creation-brief-decision",
  },
  confirm_diagram: {
    title: "確認流程理解",
    description: "檢查 Agent 對圖片流程的理解是否正確，再決定是否繼續。",
    target: "creation-diagram-decision",
  },
  answer_diagram_uncertainties: {
    title: "釐清流程中的不確定處",
    description: "回答圖片裡仍不明確的條件，讓 Agent 能依真實流程建構小工具。",
    target: "creation-diagram-decision",
  },
  confirm_diagram_interpretation: {
    title: "確認圖片的流程解讀",
    description: "核對節點、條件與分支，避免錯誤理解進入草稿。",
    target: "creation-diagram-decision",
  },
  confirm_fetch: {
    title: "決定是否讀取外部資料",
    description: "先確認 Agent 想讀取的網址；只有你同意後，平台才會擷取內容。",
    target: "creation-fetch-decision",
  },
  confirm_references: {
    title: "確認參考小工具",
    description: "檢查 Agent 找到的參考內容，決定哪些可以作為這次創作的依據。",
    target: "creation-references-decision",
  },
  confirm_duplicate: {
    title: "處理可能重複的小工具",
    description: "比較現有小工具與目前草稿，再決定沿用、調整或繼續建立。",
    target: "creation-duplicate-decision",
  },
};

export function creationFocus(session: CreationSession | undefined): CreationFocus | undefined {
  if (!session || ["saved", "cancelled", "failed"].includes(session.state)) return undefined;
  const pendingAction = session.snapshot.pending_action;
  if (pendingAction) return CREATION_FOCUS_BY_ACTION[pendingAction];
  if (session.state === "needs_reupload") {
    return {
      title: "重新上傳圖片",
      description: "先前的圖片無法沿用；請重新附上圖片，才能繼續這次創作。",
      target: "creation-message",
    };
  }
  if (session.state === "waiting_input") {
    return {
      title: "補充創作方向",
      description: "回覆 Agent 的問題或補上限制，讓這次創作繼續往前。",
      target: "creation-message",
    };
  }
  if (session.snapshot.draft) {
    return {
      title: "審閱草稿與檢查結果",
      description: "先看驗證與試跑狀態，再決定要修訂或保存成不可變版本。",
      target: "creation-draft-decision",
    };
  }
  return undefined;
}

export type CreationJourneyStatus = "complete" | "current" | "upcoming";

export type CreationJourneyItem = {
  id: "explore" | "define" | "build" | "version";
  title: string;
  description: string;
  status: CreationJourneyStatus;
};

const CREATION_JOURNEY = [
  { id: "explore", title: "探索", description: "描述任務與參考" },
  { id: "define", title: "定義", description: "確認目標與驗收" },
  { id: "build", title: "建構", description: "產生並修訂草稿" },
  { id: "version", title: "版本", description: "檢查後保存版本" },
] as const;

export function creationJourney(session: CreationSession | undefined): CreationJourneyItem[] {
  const snapshot = session?.snapshot;
  let current = 0;
  if (snapshot?.brief) current = 1;
  if (snapshot?.brief_confirmed) current = 2;
  if (snapshot?.draft) current = 3;
  const complete = session?.state === "saved" ? CREATION_JOURNEY.length : current;
  return CREATION_JOURNEY.map((item, index) => ({
    ...item,
    status: index < complete ? "complete" : index === current ? "current" : "upcoming",
  }));
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

export const MAX_MESSAGE_RUNES = 4000;

export const points = (v: number) => v + " 點";

export function budgetChoices(min: number, max: number) {
  return [...new Set([min, 200, 500, 1000, 2000, 5000, max])]
    .filter((v) => v >= min && v <= max)
    .sort((a, b) => a - b);
}

export function sessionPhase(session: CreationSession | undefined) {
  return {
    terminal: !!session && ["saved", "cancelled"].includes(session.state),
    working: !!session && ["queued", "working"].includes(session.state),
  };
}

export function startGate(
  hasSession: boolean,
  credits: CreditBalance | undefined,
  limits: CreationLimits | undefined,
  budget: string,
) {
  const creditsBlocked = !hasSession && !!credits && !credits.can_start;
  const budgetCredits = Number(budget) || undefined;
  return {
    creditsBlocked,
    choices: limits ? budgetChoices(limits.min_budget_credits, limits.max_budget_credits) : [],
    budgetCredits,
    frozen: !hasSession && (budgetCredits === undefined || creditsBlocked),
  };
}

export function raiseBudgetProblem(raw: string, current: number, max: number) {
  const amount = Number(raw);
  if (!Number.isInteger(amount) || amount <= current || amount > max)
    return `請填寫高於目前上限 ${current} 點且不超過 ${max} 點的點數。`;
  return undefined;
}

export type CompositionMode = "diagram" | "references" | "message";

export function compositionMode(file: File | undefined, referenceCount: number): CompositionMode {
  return file ? "diagram" : referenceCount > 0 ? "references" : "message";
}

export function compositionProblem(note: string, hasFile: boolean, referenceCount: number) {
  if (!hasFile && referenceCount === 0 && !note)
    return "還沒有要送出的內容：寫一句話，或附上流程圖、挑一個參考小工具。";
  if ([...note].length > MAX_MESSAGE_RUNES)
    return `文字說明最多 ${MAX_MESSAGE_RUNES} 字，目前 ${[...note].length} 字，請先剪短。`;
  if (hasFile && referenceCount > 0)
    return "流程圖和參考小工具一次只能送一種。先送其中一種，Agent 讀完之後再送另一種；文字說明可以跟著任一種一起送。";
  return undefined;
}

export function compositionCommand(
  message: string,
  diagram: CommandExtra["diagram"],
  referenceIDs: string[],
): [CreationAction["kind"], CommandExtra] {
  const note = message.trim();
  const withNote = note ? { message: note } : {};
  if (diagram) return ["diagram", { diagram, ...withNote }];
  if (referenceIDs.length > 0)
    return ["select_references", { reference_skill_ids: referenceIDs, ...withNote }];
  return ["message", { message }];
}

export function newestMessageIn(pane: HTMLElement) {
  const messages = pane.querySelectorAll<HTMLElement>(".creation-log > li[data-index]");
  return messages[messages.length - 1] as HTMLElement | undefined;
}

export function isBelow(el: HTMLElement | undefined, pane: HTMLElement) {
  return !!el && el.getBoundingClientRect().top > pane.getBoundingClientRect().bottom;
}
