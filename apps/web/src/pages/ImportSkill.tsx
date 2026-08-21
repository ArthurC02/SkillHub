import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
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
        {source === "url" ? (
          <p>
            <label htmlFor="skill-import-url">URL</label>{" "}
            <input
              id="skill-import-url"
              type="url"
              required
              value={url}
              onChange={(event) => setURL(event.target.value)}
            />
          </p>
        ) : (
          <p>
            <label htmlFor="skill-import-file">Skill zip</label>{" "}
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

      {mutation.error && !rejected && <p role="alert">匯入失敗：{mutation.error.message}</p>}
      {rejected && <Findings findings={rejected} />}
      {result && (
        <div role="status" className="notice">
          <p>
            {result.duplicate ? "相同內容已存在，沿用既有版本。" : "匯入完成。"}版本 #
            {result.version_number}
          </p>
          <Findings findings={result.findings} />
          <Link to="/skills/$skillId" params={{ skillId: result.skill_id }}>
            查看 Skill
          </Link>
        </div>
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
  return (
    <div>
      {groups.map(([label, items]) =>
        items.length ? (
          <section key={label}>
            <h2>{label}</h2>
            <ul>
              {items.map((finding, index) => (
                <li key={`${finding.code}-${index}`}>
                  <code>{finding.code}</code> {finding.path ? `${finding.path}：` : ""}
                  {finding.message}
                </li>
              ))}
            </ul>
          </section>
        ) : null,
      )}
    </div>
  );
}
