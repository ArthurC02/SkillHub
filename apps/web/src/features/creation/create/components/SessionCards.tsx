import type {
  CreationDiagramInterpretation,
  CreationFetch,
  CreationReference,
  CreationSnapshot,
} from "../../creation.service";
import { Reveal } from "../../../../shared/ui/Reveal";
import { FETCH_STATUS_LABEL, parseDiagramUnderstanding } from "../create.model";
import type { Perform } from "../create.commands";
import { DiagramUnderstandingView } from "./DiagramUnderstandingView";
import { ReferenceList } from "./ReferenceList";

export function RoundTimeline({ items }: { items: { key: string; text: string }[] }) {
  return (
    <section>
      <h4>回合時間線</h4>
      <ol className="round-timeline">
        {items.map((item) => (
          <li key={item.key}>{item.text}</li>
        ))}
      </ol>
    </section>
  );
}

export function BriefCard({
  p,
  locked,
  perform,
}: {
  p: CreationSnapshot;
  locked: boolean;
  perform: Perform;
}) {
  const decisionTarget = p.pending_action === "confirm_brief";
  return (
    <section
      id={decisionTarget ? "creation-brief-decision" : undefined}
      tabIndex={decisionTarget ? -1 : undefined}
    >
      <header className="card-header">
        <h4>需求摘要</h4>
        <span className="card-tag" data-tone={p.brief_confirmed ? "done" : undefined}>
          {p.brief_confirmed ? "已確認" : "尚未確認"}
        </span>
      </header>
      <p>{p.brief}</p>
      {p.model_changed?.brief !== undefined && (
        <p className="note">模型改過這一段（需求摘要），原本是：{p.model_changed.brief}</p>
      )}
      <h5>驗收條件</h5>
      {p.acceptance_criteria.length > 0 ? (
        <ol>
          {p.acceptance_criteria.map((c, i) => (
            <li key={i}>{c}</li>
          ))}
        </ol>
      ) : (
        <p>模型尚未提出驗收條件</p>
      )}
      {p.model_changed?.acceptance_criteria !== undefined && (
        <>
          <p className="note">模型改過這一段（驗收條件），原本是：</p>
          <ol className="note">
            {p.model_changed.acceptance_criteria.map((c, i) => (
              <li key={i}>{c}</li>
            ))}
          </ol>
        </>
      )}
      {p.sample_input && (
        <>
          <h5>試跑用的範例輸入</h5>
          <pre className="skill-md">
            <Reveal text={p.sample_input} />
          </pre>
        </>
      )}
      {p.model_changed?.sample_input !== undefined && (
        <>
          <p className="note">模型改過這一段（範例輸入），原本是：</p>
          <pre className="skill-md">
            <Reveal text={p.model_changed.sample_input} />
          </pre>
        </>
      )}
      {p.pending_action === "confirm_brief" && (
        <div className="card-actions">
          <button
            className="card-primary"
            disabled={locked}
            onClick={() => void perform("confirm_brief")}
          >
            {p.model_changed ? "我看過差異，確認新的需求摘要" : "確認需求摘要與驗收條件"}
          </button>
        </div>
      )}
    </section>
  );
}

export function DiagramUnderstandingCard({
  understanding,
  confirmed,
  pendingAction,
  decisionTarget,
  locked,
  perform,
}: {
  understanding: string;
  confirmed: boolean;
  pendingAction: string;
  decisionTarget?: boolean;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <section
      id={decisionTarget ? "creation-diagram-decision" : undefined}
      tabIndex={decisionTarget ? -1 : undefined}
    >
      <header className="card-header">
        <h4>流程圖理解</h4>
        <span className="card-tag" data-tone={confirmed ? "done" : undefined}>
          {confirmed ? "已確認" : "尚未確認"}
        </span>
      </header>
      <DiagramUnderstandingView raw={understanding} />
      {pendingAction === "confirm_diagram" && (
        <div className="card-actions">
          <button
            className="card-primary"
            disabled={locked || !parseDiagramUnderstanding(understanding)}
            onClick={() => void perform("confirm_diagram")}
          >
            確認流程圖理解
          </button>
        </div>
      )}
    </section>
  );
}

export function DiagramDescriptionCard({
  description,
  confirmed,
  pendingAction,
  decisionTarget,
  locked,
  perform,
}: {
  description: string;
  confirmed: boolean | undefined;
  pendingAction: string;
  decisionTarget?: boolean;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <section
      id={decisionTarget ? "creation-diagram-decision" : undefined}
      tabIndex={decisionTarget ? -1 : undefined}
    >
      <header className="card-header">
        <h4>流程圖描述</h4>
        <span className="card-tag" data-tone={confirmed ? "done" : undefined}>
          {confirmed ? "已確認" : "等待確認"}
        </span>
      </header>
      <p>{description}</p>
      {pendingAction === "confirm_diagram" && (
        <div className="card-actions">
          <button
            className="card-primary"
            disabled={locked}
            onClick={() => void perform("confirm_diagram")}
          >
            確認這是流程圖要表達的內容
          </button>
        </div>
      )}
    </section>
  );
}

export function DiagramInterpretationCard({
  interpretation,
  confirmed,
  pendingAction,
  decisionTarget,
  answers,
  onAnswer,
  locked,
  perform,
}: {
  interpretation: CreationDiagramInterpretation;
  confirmed: boolean;
  pendingAction: string;
  decisionTarget?: boolean;
  answers: Record<string, string>;
  onAnswer: (uncertaintyID: string, answer: string) => void;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <section
      id={decisionTarget ? "creation-diagram-decision" : undefined}
      tabIndex={decisionTarget ? -1 : undefined}
    >
      <header className="card-header">
        <h4>流程圖拆解</h4>
        <span className="card-tag" data-tone={confirmed ? "done" : undefined}>
          {confirmed ? "已確認" : "等待確認"}
        </span>
      </header>
      {(["nodes", "conditions", "branches"] as const).map((section) => (
        <div key={section}>
          <h5>{{ nodes: "節點", conditions: "條件", branches: "分支" }[section]}</h5>
          {interpretation[section].length ? (
            <ul>
              {interpretation[section].map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          ) : (
            <p>未列出</p>
          )}
        </div>
      ))}
      {interpretation.uncertainties.map((uncertainty) => (
        <div key={uncertainty.id}>
          <label>
            <span>{uncertainty.question}</span>
            <textarea
              aria-label={uncertainty.question}
              disabled={locked || pendingAction !== "answer_diagram_uncertainties"}
              value={answers[uncertainty.id] ?? uncertainty.answer ?? ""}
              onChange={(event) => onAnswer(uncertainty.id, event.target.value)}
            />
          </label>
          {pendingAction === "answer_diagram_uncertainties" && (
            <button
              disabled={locked || !(answers[uncertainty.id] ?? uncertainty.answer ?? "").trim()}
              onClick={() =>
                void perform("answer_diagram_uncertainty", {
                  diagram_uncertainty_id: uncertainty.id,
                  diagram_answer: answers[uncertainty.id] ?? uncertainty.answer ?? "",
                })
              }
            >
              確認這一題的答案
            </button>
          )}
        </div>
      ))}
      {pendingAction === "confirm_diagram_interpretation" && (
        <div className="card-actions">
          <button
            className="card-primary"
            disabled={locked}
            onClick={() => void perform("confirm_diagram_interpretation")}
          >
            確認完整流程圖拆解
          </button>
        </div>
      )}
    </section>
  );
}

export function FetchConsentCard({
  url,
  locked,
  perform,
}: {
  url: string;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <section id="creation-fetch-decision" tabIndex={-1}>
      <h4>連網讀取確認</h4>
      <p>
        模型想連到 <code>{url}</code>{" "}
        讀取內容來補資料。你的網路環境可能擋住這個網站；被擋住時會直接回報，不會重試。
      </p>
      <div className="card-actions">
        <button disabled={locked} onClick={() => void perform("decline_fetch")}>
          不連網
        </button>
        <button
          className="card-primary"
          disabled={locked}
          onClick={() => void perform("confirm_fetch")}
        >
          同意連網
        </button>
      </div>
    </section>
  );
}

export function FetchedPages({ fetches }: { fetches: CreationFetch[] }) {
  return (
    <section>
      <h4>已讀取的網頁</h4>
      <ul className="ref-facts">
        {fetches.map((f, i) => (
          <li key={i}>
            {f.url}：{FETCH_STATUS_LABEL[f.status] ?? f.status}
            {f.bytes !== undefined && `（${f.bytes} 位元組）`}
          </li>
        ))}
      </ul>
    </section>
  );
}

export function ReferencesCard({
  references,
  pendingAction,
  catalogChecked,
  locked,
  perform,
}: {
  references: CreationReference[];
  pendingAction: string;
  catalogChecked: boolean | undefined;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <section
      id={pendingAction === "confirm_references" ? "creation-references-decision" : undefined}
      tabIndex={pendingAction === "confirm_references" ? -1 : undefined}
    >
      <h4>參考小工具</h4>
      {pendingAction === "confirm_references" && catalogChecked && (
        <p>目錄裡已有相近的小工具；你可以直接採用其中一個、以它們為參考，或從頭寫。</p>
      )}
      <ReferenceList
        items={references}
        adoptable={pendingAction === "confirm_references"}
        showStatus
        locked={locked}
        onAdopt={(skillID) => void perform("adopt_reference", { reference_skill_ids: [skillID] })}
      />
      {pendingAction === "confirm_references" && (
        <div className="card-actions">
          <button disabled={locked} onClick={() => void perform("decline_references")}>
            都不是，從頭寫
          </button>
          <button
            className="card-primary"
            disabled={locked || references.some((r) => !r.available)}
            onClick={() => void perform("confirm_references")}
          >
            以這些為參考
          </button>
        </div>
      )}
    </section>
  );
}

export function DuplicatesCard({
  duplicates,
  contentHash,
  locked,
  perform,
}: {
  duplicates: CreationReference[];
  contentHash: string | undefined;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <section id="creation-duplicate-decision" tabIndex={-1}>
      <h4>目錄已有相近的小工具</h4>
      <p>
        保存前 Go
        查了一次目錄：下面這些和你的草稿很接近。你可以直接採用其中一個，或仍然建立自己的版本。
      </p>
      <ReferenceList
        items={duplicates}
        adoptable
        showStatus={false}
        locked={locked}
        onAdopt={(skillID) => void perform("adopt_reference", { reference_skill_ids: [skillID] })}
      />
      <div className="card-actions">
        <button
          className="caution"
          disabled={locked || !contentHash}
          onClick={() => void perform("confirm_duplicate", { content_hash: contentHash })}
        >
          仍然建立
        </button>
      </div>
    </section>
  );
}
