import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import {
  importSkillFromURL,
  isCategorizedFindings,
  uploadSkillPackage,
  type CategorizedFindings,
  type ImportFinding,
  type ImportResult,
} from "../api/import";

/**
 * 02:SKILL-002 / INGEST-008 — the one screen third-party code enters the
 * platform through, which makes it where design §1.1 costs the most: 每個畫面必須
 * 讓證據看得見，也必須讓證據的缺席看得見. Three of those absences rendered as blank
 * space here, and blank reads as 通過:
 *
 * 1. **A rejection never said it was rejected.** The 422 path suppressed the
 *    generic alert, so a blocked package produced an `<h2>阻擋錯誤</h2>` and a
 *    list — the words 匯入失敗 and 未匯入 appeared nowhere, nothing was in a live
 *    region, and the `<h1>` still read 匯入 Skill, which names the widget rather
 *    than answering. Restoring that alert alone would not have fixed it: a 422
 *    carries `CategorizedFindings` and no `error` key, so `ApiError.message`
 *    falls back to `response.statusText` and the line would have read
 *    「匯入失敗：Unprocessable Entity」. The rejection states the three things a
 *    rejection owes the reader instead — what was rejected, that nothing was
 *    imported, and what to do next.
 * 2. **A clean scan rendered as silence** (§2.1). Every empty group returned
 *    `null`, so a package with no findings showed 匯入完成 and nothing else,
 *    which is indistinguishable from a page that never scanned. The absence is
 *    written now, including what the scan is *not*: not a human review, and not
 *    a signature check — MVP has no signature mechanism to run (ADR-027 決策 3
 *    is an explicit 不做, not a pending item), so that gap is permanent until
 *    that ADR is reopened and the sentence says so rather than implying a check
 *    that is merely late.
 * 3. **`details` was carried and never rendered** (§1.1). One `external-url`
 *    finding aggregates per host and holds every `<file>: <url>` reference, so
 *    the page could say 「320 external URL」 with no way to see which. It folds
 *    into a `<details>` (§2.6). Those are paths the scanner derived and URLs it
 *    parsed out — never file contents, which 02:NFR-002 keeps off this path.
 */
export function ImportSkill() {
  const queryClient = useQueryClient();
  const [source, setSource] = useState<"url" | "upload">("url");
  const [url, setURL] = useState("");
  const [file, setFile] = useState<File>();
  const [result, setResult] = useState<ImportResult>();
  const [rejected, setRejected] = useState<CategorizedFindings>();

  const mutation = useMutation({
    mutationFn: () => {
      if (source === "url") return importSkillFromURL(url.trim());
      if (!file) return Promise.reject(new Error("請選擇 zip 套件。"));
      return uploadSkillPackage(file);
    },
    onSuccess: async (data) => {
      setRejected(undefined);
      setResult(data);
      await queryClient.invalidateQueries({ queryKey: ["skills"] });
    },
    onError: (error) => {
      setResult(undefined);
      setRejected(
        error instanceof ApiError && isCategorizedFindings(error.body) ? error.body : undefined,
      );
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    mutation.mutate();
  };

  return (
    <section className="page">
      <h1>匯入 Skill</h1>
      <p className="note">套件只會做靜態檢查；匯入期間不執行其中的 Script。</p>
      <form onSubmit={submit}>
        <fieldset>
          <legend>來源</legend>
          <label>
            <input
              type="radio"
              name="skill-import-source"
              checked={source === "url"}
              onChange={() => setSource("url")}
            />
            GitHub 或允許的 URL
          </label>{" "}
          <label>
            <input
              type="radio"
              name="skill-import-source"
              checked={source === "upload"}
              onChange={() => setSource("upload")}
            />
            上傳 zip
          </label>
        </fieldset>
        {/*
          `.field` rather than a bare `<p>`: an inline `<label> <input>` gets the
          UA's intrinsic ~180px on a 1126px page, so a GitHub URL was unreadable
          in the field you paste it into — the highest-stakes input in the app.
          Same defect index.css records fixing for the Home search box, and the
          class already exists for exactly this (design §4.5, checklist 8).
        */}
        {source === "url" ? (
          <p className="field">
            <label htmlFor="skill-import-url">URL</label>
            <input
              id="skill-import-url"
              type="url"
              required
              value={url}
              onChange={(event) => setURL(event.target.value)}
            />
          </p>
        ) : (
          <p className="field">
            <label htmlFor="skill-import-file">Skill zip</label>
            <input
              id="skill-import-file"
              type="file"
              required
              accept=".zip,application/zip"
              onChange={(event) => setFile(event.target.files?.[0])}
            />
          </p>
        )}
        <button type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "匯入中…" : "開始匯入"}
        </button>
      </form>

      {/* A failure the server did not categorise: no findings to show, so the
          message is all there is. The categorised 422 renders below instead. */}
      {mutation.error && !rejected && <p role="alert">匯入失敗：{mutation.error.message}</p>}

      {rejected && (
        <section role="alert">
          <h2>匯入失敗：套件被擋下，沒有匯入任何東西</h2>
          <p>
            這個工作區沒有新增任何 Skill，也沒有建立新版本。
            下面每一則阻擋錯誤都要在套件裡修掉，再重新匯入一次；警告與資訊不擋匯入，一併列在後面。
          </p>
          <Findings findings={rejected} />
        </section>
      )}

      {result && (
        <>
          <div role="status" className="notice">
            <p>
              {result.duplicate ? "相同內容已存在，沿用既有版本。" : "匯入完成。"}版本 #
              {result.version_number}
            </p>
            <Link to="/skills/$skillId" params={{ skillId: result.skill_id }}>
              查看 Skill
            </Link>
          </div>
          {/* The three groups used to be `h2` siblings of the page `h1`, i.e.
              children of a section nobody had named. Named here, and the groups
              demoted to `h3` under it (checklist 6; axe never sees this — it
              fails a skipped level, not a level that should have gone down). */}
          <section>
            <h2>靜態檢查結果</h2>
            <Findings findings={result.findings} />
          </section>
        </>
      )}
    </section>
  );
}

function Findings({ findings }: { findings: CategorizedFindings }) {
  const groups = [
    ["阻擋錯誤", findings.errors],
    ["警告", findings.warnings],
    ["資訊", findings.infos],
  ] as const;

  // §2.1. Nothing found is a result, and it was the one this page rendered as
  // an empty div. What the scan does not cover is stated with it, because a
  // clean report is exactly when a reader is most likely to read 沒有發現 as
  // 有人看過並認可.
  if (groups.every(([, items]) => items.length === 0)) {
    return (
      <p className="note">
        靜態檢查跑完了，沒有任何發現——這是「掃過了，沒掃到」，不是「沒掃」。它讀套件內容、不執行其中的
        Script，<strong>既不是人工審查，也不是簽章驗證</strong>
        ：MVP 的套件不帶數位簽章，平台也不驗簽（ADR-027 決策 3
        是明文的「不做」），所以簽章這一項不是還沒驗，是這裡永遠不會有人替你驗。
      </p>
    );
  }

  return (
    <div>
      {groups.map(([label, items]) =>
        items.length ? (
          <section key={label}>
            <h3>
              {label}（{items.length}）
            </h3>
            {/* §4.3: a finding is the definition of 一則可以被單獨採信或拒絕的
                東西 — a code, a path and a claim the reader weighs before
                trusting the package — and it was a bare UA `<ul>` with discs.
                `.finding-list` + `.criterion` are the pair RunEvaluation's
                equivalent list uses; the rule is 套用既有的卡片，不是發明第五種
                樣式. */}
            <ul className="finding-list">
              {items.map((finding, index) => (
                <li className="criterion" key={`${finding.code}-${index}`}>
                  <FindingBody finding={finding} />
                </li>
              ))}
            </ul>
          </section>
        ) : null,
      )}
    </div>
  );
}

function FindingBody({ finding }: { finding: ImportFinding }) {
  return (
    <>
      {/* checklist 5 / §2.6: the sentence leads and the identifier follows it.
          `license-unknown` used to be the first thing on every line — an
          identifier ahead of the answer. Same shape as RunEvaluation's
          執行完成（succeeded） and the finding rows on Packaging. */}
      <p>
        {finding.message}（<span className="risk-code">{finding.code}</span>）
        {finding.path && (
          <>
            {" "}
            <code>{finding.path}</code>
          </>
        )}
      </p>
      {finding.details && finding.details.length > 0 && (
        <details>
          <summary>逐項列出（{finding.details.length} 筆）</summary>
          <ul className="risk-list">
            {finding.details.map((detail, index) => (
              <li key={`${detail}-${index}`}>{detail}</li>
            ))}
          </ul>
        </details>
      )}
    </>
  );
}
