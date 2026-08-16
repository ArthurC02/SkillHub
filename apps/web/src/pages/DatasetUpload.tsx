import { useMutation, useQuery } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { getDatasetLimits, uploadDataset, type Dataset } from "../api/lab";

/**
 * 02:TEST-002 second criterion — "上傳前顯示大小限制、保存政策及資料使用範圍".
 *
 * The acceptance verb is *display*, and the display has to happen before the
 * upload, not as the text of a refusal. So the limits are fetched and rendered
 * first, unconditionally, and the file input sits underneath them: a user who
 * reads this page top to bottom has seen the rules before touching a file.
 *
 * The numbers are never written here. They come from GET /test-cases/limits,
 * which serves the same constants internal/testlab enforces — a second copy in
 * the UI is how a published limit and an enforced limit drift apart, and the
 * server-side test already asserts the two are the same.
 *
 * SCOPE, stated honestly: this is the upload step and nothing else. Creating a
 * test case, editing the prompt and acceptance criteria, and listing or deleting
 * files are still API-only (DESIGN-007). Hence the test case id comes from the
 * URL, as on the preflight screen.
 */

type UploadSearch = { test_case?: string };

function bytes(n: number): string {
  if (n >= 1 << 20) return `${Math.round(n / (1 << 20))} MB`;
  if (n >= 1 << 10) return `${Math.round(n / (1 << 10))} KB`;
  return `${n} B`;
}

export function DatasetUpload() {
  const { test_case: testCase = "" } = useSearch({ strict: false }) as UploadSearch;
  const fileInput = useRef<HTMLInputElement>(null);
  const [message, setMessage] = useState("");
  const [uploaded, setUploaded] = useState<Dataset[]>([]);

  const limits = useQuery({
    queryKey: ["dataset-limits"],
    queryFn: getDatasetLimits,
    retry: false,
  });

  const upload = useMutation({
    mutationFn: (file: File) => uploadDataset(testCase, file),
    onSuccess: (d) => {
      setUploaded((prev) => [...prev, d]);
      setMessage("");
      if (fileInput.current) fileInput.current.value = "";
    },
    // The server's message is shown as-is: 02:TEST-002 requires it to be
    // understandable and to leak nothing about the system, and it is written to
    // that rule. Rewriting it here would be a second place to get that wrong.
    onError: (err) => setMessage(err instanceof Error ? err.message : "上傳失敗。"),
  });

  return (
    <section className="page">
      <h1>上傳 Dataset</h1>

      {limits.isPending && <p>載入上傳規則中…</p>}
      {limits.error && (
        // Fail-closed in the UI too: without the rules on screen the "顯示" step
        // has not happened, so the file input is not offered.
        <p role="alert">無法讀取上傳規則,因此暫時不能上傳:{limits.error.message}</p>
      )}

      {limits.data && (
        <>
          <h2>上傳前請先確認</h2>
          <dl className="preflight">
            <dt>大小限制</dt>
            <dd>
              單一檔案最大 {bytes(limits.data.max_file_bytes)};同一個 Test Case 合計最大{" "}
              {bytes(limits.data.max_test_case_bytes)}、最多 {limits.data.max_files_per_test_case}{" "}
              個檔案。
            </dd>

            <dt>支援格式</dt>
            <dd>
              <ul>
                {limits.data.allowed_kinds.map((k) => (
                  <li key={k}>{k}</li>
                ))}
              </ul>
              <p>檔案類型以內容判定,不看副檔名。</p>
            </dd>

            <dt>保存政策</dt>
            <dd>上傳後保存 {limits.data.retention_days} 天,到期自動刪除;你也可以隨時自行刪除。</dd>

            <dt>資料使用範圍</dt>
            <dd>
              只有這個 Test Case 的 Run 讀得到,不會提供給其他使用者或其他 Run。 請不要上傳
              Secrets、憑證或個人資料。
            </dd>
          </dl>

          {testCase === "" ? (
            <p>
              這個頁面需要 <code>?test_case=</code>。Test Case 的建立介面尚未實作(DESIGN-007),
              目前請由 API 建立後帶入 ID。
            </p>
          ) : (
            <>
              <label htmlFor="dataset-file">選擇檔案</label>{" "}
              <input id="dataset-file" type="file" ref={fileInput} />{" "}
              <button
                type="button"
                disabled={upload.isPending}
                onClick={() => {
                  const file = fileInput.current?.files?.[0];
                  if (!file) {
                    setMessage("請先選擇一個檔案。");
                    return;
                  }
                  upload.mutate(file);
                }}
              >
                上傳
              </button>
            </>
          )}
        </>
      )}

      {message && <p role="alert">{message}</p>}

      {uploaded.length > 0 && (
        <ul>
          {uploaded.map((d) => (
            <li key={d.dataset_id}>
              已上傳 {d.file_name}（{bytes(d.size_bytes)}）
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
