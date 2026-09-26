import { useState, type FormEvent } from "react";
import type { SearchCorrection, SearchInterpretation } from "../../../../core/api/types";
import {
  emptySearchIntent,
  correctedSearchText,
  INTENT_FIELDS,
  INTENT_FIELD_LIMIT,
  INTENT_KEYWORD_LIMIT,
  INTENT_KEYWORD_LENGTH,
  readSearchCorrection,
} from "../../../../core/api/searchIntent";
import "./IntentInterpretation.css";

const labels = { input: "輸入", output: "輸出", tools: "工具", data: "資料", environment: "環境" };

export function IntentInterpretation({
  interpretation,
  onCorrect,
}: {
  interpretation: SearchInterpretation;
  onCorrect: (correction: SearchCorrection, clearFilters?: boolean) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [intent, setIntent] = useState(interpretation.intent);
  const [keywords, setKeywords] = useState(interpretation.keywords.join("\n"));
  const [error, setError] = useState("");
  const analyzed = interpretation.status === "analyzed" || interpretation.status === "corrected";

  function submit(event: FormEvent) {
    event.preventDefault();
    try {
      const correction = readSearchCorrection(
        JSON.stringify({
          intent,
          keywords: keywords
            .split("\n")
            .map((text) => text.trim())
            .filter(Boolean),
        }),
      );
      setError("");
      onCorrect(correction);
      setEditing(false);
    } catch (error) {
      setError(error instanceof Error ? error.message : "搜尋修正內容無效。");
    }
  }

  return (
    <section className="intent-interpretation" aria-label="搜尋理解">
      <h2>{interpretation.status === "corrected" ? "你修正的搜尋理解" : "系統如何理解任務"}</h2>
      {interpretation.status === "fallback" && (
        <p role="status" className="notice">
          意圖分析暫時無法使用，這次以原句搜尋。你仍可自行修正。
        </p>
      )}
      <dl>
        {INTENT_FIELDS.map((field) => (
          <div key={field}>
            <dt>{labels[field]}</dt>
            <dd>{analyzed ? (interpretation.intent[field] ?? "未提及") : "未測量"}</dd>
          </div>
        ))}
      </dl>
      <p>
        檢索內容：
        {(interpretation.status === "corrected"
          ? correctedSearchText(interpretation)
          : interpretation.keywords.join("、")) || "使用原句"}
      </p>
      <button type="button" aria-expanded={editing} onClick={() => setEditing(!editing)}>
        修正搜尋理解
      </button>
      <button
        type="button"
        onClick={() => onCorrect({ intent: emptySearchIntent(), keywords: [] }, true)}
      >
        捨棄理解與篩選，以原句搜尋
      </button>
      {editing && (
        <form onSubmit={submit}>
          <p className="note">
            五欄理解與關鍵詞合計最多 {INTENT_FIELD_LIMIT}{" "}
            字，留白表示未提及；搜尋仍保留原始任務描述。
          </p>
          {INTENT_FIELDS.map((field) => (
            <label key={field}>
              {labels[field]}
              <input
                value={intent[field] ?? ""}
                onChange={(event) => setIntent({ ...intent, [field]: event.target.value || null })}
              />
            </label>
          ))}
          <label>
            檢索關鍵詞（一行一組）
            <textarea value={keywords} onChange={(event) => setKeywords(event.target.value)} />
          </label>
          <p className="note">
            最多 {INTENT_KEYWORD_LIMIT} 組，每組最多 {INTENT_KEYWORD_LENGTH}{" "}
            字；五欄與關鍵詞全部留白才使用原句。
          </p>
          {error && <p role="alert">{error}</p>}
          <button type="submit">套用修正並搜尋</button>
        </form>
      )}
    </section>
  );
}
