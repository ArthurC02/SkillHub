import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Findings } from "../components/Findings";
import { LoginRequired, ReadFailure, unauthenticated } from "../components/LoginRequired";
import { useMe } from "../api/me";
import { ApiError } from "../api/client";
import {
  importSkillFromURL,
  isCategorizedFindings,
  uploadSkillPackage,
  type CategorizedFindings,
  type ImportResult,
} from "../api/import";

export function ImportSkill() {
  const queryClient = useQueryClient();
  const me = useMe();
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
      await queryClient.invalidateQueries({ queryKey: ["own-skills"] });
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
    <section>
      <h1>匯入 Skill</h1>
      <p className="note">套件只會做靜態檢查；匯入期間不執行其中的 Script。</p>

      <ul className="note">
        <li>來源限 GitHub（PDM-002 的首批來源），其他網域一律拒絕。</li>
        <li>網址必須是 https，而且不得帶帳號密碼、查詢字串或錨點。</li>
        <li>大小上限見拒絕訊息——平台對 zip 與解壓後各強制一個上限，這一頁還讀不到它們的數字。</li>
        <li>
          zip 的最上層（或單一頂層資料夾）要有 <code>SKILL.md</code>，而且它的 frontmatter 要有{" "}
          <code>name</code> 與 <code>description</code>——名稱、描述與 License 都從那裡讀，
          不必在這一頁手打。
        </li>
      </ul>
      {unauthenticated(me.error) ? (
        <LoginRequired what="匯入 Skill" />
      ) : (
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
          {/* key props: without them React reuses one <input> across branches
              and warns controlled→uncontrolled on every mode switch. */}
          {source === "url" ? (
            <p className="field" key="url">
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
            <p className="field" key="file">
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
      )}

      {mutation.error && !rejected && (
        <ReadFailure error={mutation.error} what="匯入 Skill">
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
          <section>
            <h2>靜態檢查結果</h2>
            <Findings findings={result.findings} />
          </section>
        </>
      )}
    </section>
  );
}
