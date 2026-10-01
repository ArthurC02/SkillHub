import { useState, type FormEvent } from "react";
import { Link } from "@tanstack/react-router";
import { Findings } from "../../../shared/ui/Findings";
import { LoginRequired, ReadFailure } from "../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { useMe } from "../../../core/session/me.service";
import { ApiError } from "../../../core/api/client";
import {
  isImportResult,
  useImportSkill,
  useSkillImportLimits,
  type SkillImportLimits,
} from "../import.service";
import type { ImportResult, ImportedSkill, RefusedSkill } from "../../../core/api/types";
import "./ImportSkill.page.css";

function mb(bytes: number): string {
  return (bytes / (1 << 20)).toFixed(1).replace(/\.0$/, "") + " MB";
}

export function ImportSkill() {
  const me = useMe();
  const [source, setSource] = useState<"url" | "upload">("url");
  const [url, setURL] = useState("");
  const [file, setFile] = useState<File>();
  const mutation = useImportSkill();
  const limits = useSkillImportLimits();
  const rules = Array.isArray(limits.data?.allowed_hosts) ? limits.data : undefined;
  const result = mutation.data;
  const failure = mutation.error;
  const rejected =
    failure instanceof ApiError && isImportResult(failure.body) ? failure.body : undefined;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    mutation.mutate(source === "url" ? { url: url.trim() } : { file });
  };

  return (
    <section className="import-page">
      <header className="import-header">
        <p className="page-eyebrow">Package intake</p>
        <h1>匯入小工具</h1>
        <p>從允許的來源收進套件，先看懂平台會檢查什麼，再選擇取得方式。</p>
      </header>

      <div className="import-workspace">
        <section className="import-rules" aria-labelledby="import-rules-title">
          <p className="page-eyebrow">Static inspection</p>
          <h2 id="import-rules-title">匯入前檢查</h2>
          <p>套件只會做靜態檢查；匯入期間不執行其中的 Script。</p>
          <ImportRules rules={rules} />
        </section>
        <section className="import-source" aria-labelledby="import-source-title">
          <p className="page-eyebrow">Source</p>
          <h2 id="import-source-title">選擇套件來源</h2>
          {unauthenticated(me.error) ? (
            <LoginRequired what="匯入小工具" />
          ) : (
            <ImportSourceForm
              source={source}
              onSourceChange={setSource}
              url={url}
              onURLChange={setURL}
              onFileChange={setFile}
              mutation={mutation}
              onSubmit={submit}
            />
          )}
        </section>
      </div>

      {mutation.error && !rejected && (
        <ReadFailure error={mutation.error} what="匯入小工具">
          <p role="alert">
            {mutation.error instanceof ApiError && mutation.error.status === 400
              ? "這個檔案不是可用的 zip 套件，或網址抓不到內容。"
              : mutation.error instanceof ApiError && mutation.error.status === 413
                ? "檔案超過上限。"
                : "匯入失敗，可以再按一次。"}
          </p>
        </ReadFailure>
      )}

      {rejected && (
        <section role="alert">
          <h2>匯入失敗：這個來源沒有一個小工具進得來</h2>
          <p>
            這個工作區沒有新增任何小工具，也沒有建立新版本。
            下面每一則阻擋錯誤都要在來源裡修掉，再重新匯入一次；警告與資訊不擋匯入，一併列在後面。
          </p>
          <RefusedList refused={rejected.refused} />
        </section>
      )}

      {result && <ImportOutcome result={result} />}
    </section>
  );
}

function ImportSourceForm({
  source,
  onSourceChange,
  url,
  onURLChange,
  onFileChange,
  mutation,
  onSubmit,
}: {
  source: "url" | "upload";
  onSourceChange: (source: "url" | "upload") => void;
  url: string;
  onURLChange: (url: string) => void;
  onFileChange: (file: File | undefined) => void;
  mutation: ReturnType<typeof useImportSkill>;
  onSubmit: (event: FormEvent) => void;
}) {
  return (
    <form onSubmit={onSubmit}>
      <fieldset>
        <legend>來源</legend>
        <label>
          <input
            type="radio"
            name="skill-import-source"
            checked={source === "url"}
            onChange={() => onSourceChange("url")}
          />
          GitHub 或允許的 URL
        </label>{" "}
        <label>
          <input
            type="radio"
            name="skill-import-source"
            checked={source === "upload"}
            onChange={() => onSourceChange("upload")}
          />
          上傳 zip
        </label>
      </fieldset>
      {source === "url" ? (
        <p className="field" key="url">
          <label htmlFor="skill-import-url">URL</label>
          <input
            id="skill-import-url"
            type="url"
            required
            value={url}
            onChange={(event) => onURLChange(event.target.value)}
          />
        </p>
      ) : (
        <p className="field" key="file">
          <label htmlFor="skill-import-file">小工具 zip</label>
          <input
            id="skill-import-file"
            type="file"
            required
            accept=".zip,application/zip"
            onChange={(event) => onFileChange(event.target.files?.[0])}
          />
        </p>
      )}
      <button type="submit" className="action" disabled={mutation.isPending}>
        {mutation.isPending ? "匯入中…" : "開始匯入"}
      </button>
      {mutation.isPending && (
        <p role="status" className="note">
          正在取得套件並逐檔用靜態檢查看過它——這一步會自己結束，不需要你再按任何東西。
          <strong>請先不要關掉這個分頁</strong>：匯入是一個同步請求，關掉等於取消，
          而取消不會在你的工作區留下半成品版本。
        </p>
      )}
    </form>
  );
}

function ImportRules({ rules }: { rules: SkillImportLimits | undefined }) {
  return (
    <ul className="note">
      <li>
        來源限 {rules ? rules.allowed_hosts.join("、") : "GitHub（PDM-002 的首批來源）"}
        ，其他網域一律拒絕。
      </li>
      <li>網址必須是 https，而且不得帶帳號密碼、查詢字串或錨點。</li>
      {rules ? (
        <li>
          zip 最大 {mb(rules.max_zip_bytes)}，解壓後總量最大 {mb(rules.max_unpacked_bytes)}；最多{" "}
          {rules.max_files} 個檔案、 單一檔案最大 {mb(rules.max_file_bytes)}、路徑最深{" "}
          {rules.max_path_depth} 層。
        </li>
      ) : (
        <li>正在讀這個部署的大小上限…</li>
      )}
      <li>
        zip 的最上層（或單一頂層資料夾）要有 <code>SKILL.md</code>，而且它的 frontmatter 要有{" "}
        <code>name</code> 與 <code>description</code>——名稱、描述與 License 都從那裡讀，
        不必在這一頁手打。
      </li>
    </ul>
  );
}

function ImportOutcome({ result }: { result: ImportResult }) {
  return (
    <>
      <div role="status" className="notice">
        <p>
          匯入完成，這個來源帶進 {result.skills.length} 個小工具。
          {result.plugin && (
            <>
              {" "}
              它是一個 Agent Plugin（<code>{result.plugin.name}</code>
              {result.plugin.version ? ` ${result.plugin.version}` : ""}）。
            </>
          )}
        </p>
        {result.plugin && (
          <p className="note">
            整個 Plugin 以原樣存成一份套件，Plugin 裡的每個小工具都指向它，但
            <strong>試跑時只安裝該小工具自己的目錄</strong>
            ，下載得到的也只有那一個小工具的可攜套件。 平台不把整套還給你——要整套，回到原本的 Plugin
            來源。
          </p>
        )}
      </div>

      <section>
        <h2>進來的小工具（{result.skills.length}）</h2>
        <ul>
          {result.skills.map((skill) => (
            <li key={skill.version_id}>
              <SkillOutcome skill={skill} />
            </li>
          ))}
        </ul>
      </section>

      {result.refused.length > 0 && (
        <section role="alert">
          <h2>沒進來的小工具（{result.refused.length}）</h2>
          <p>
            這幾個資料夾沒有建立
            小工具，其餘的照樣進來了。每一則阻擋錯誤都要在來源裡修掉，再重新匯入一次。
          </p>
          <RefusedList refused={result.refused} />
        </section>
      )}

      {result.excluded_components.length > 0 && (
        <section>
          <h2>沒有匯入的部分（{result.excluded_components.length}）</h2>
          <p>這些是 Plugin 裡不是 Agent 小工具的部分。平台只把它們列出來，不匯入也不執行。</p>
          <Findings findings={{ errors: [], warnings: [], infos: result.excluded_components }} />
        </section>
      )}
    </>
  );
}

function SkillOutcome({ skill }: { skill: ImportedSkill }) {
  return (
    <>
      <p>
        {skill.path && (
          <>
            <code>{skill.path}</code>{" "}
          </>
        )}
        {skill.duplicate ? "相同內容已存在，沿用既有版本。" : "新版本已建立。"}版本 #
        {skill.version_number}{" "}
        <Link
          to="/skills/$skillId/versions/$versionId"
          params={{ skillId: skill.skill_id, versionId: skill.version_id }}
        >
          開啟 v{skill.version_number} 並繼續驗證
        </Link>
      </p>
      <details>
        <summary>靜態檢查結果</summary>
        <Findings findings={skill.findings} level={4} />
      </details>
    </>
  );
}

function RefusedList({ refused }: { refused: RefusedSkill[] }) {
  return (
    <ul>
      {refused.map((one, index) => (
        <li key={`${one.path}-${index}`}>
          {one.path && (
            <p>
              <code>{one.path}</code>
            </p>
          )}
          <Findings findings={one.findings} level={4} />
        </li>
      ))}
    </ul>
  );
}
