import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "../api/client";
import { getDatasetLimits, uploadDataset, type Dataset } from "../api/lab";

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
  const [uploadError, setUploadError] = useState<unknown>(null);
  const [uploaded, setUploaded] = useState<Dataset[]>([]);
  // `test_case` is a search param on this route, so it changes without remounting.
  useEffect(() => {
    setMessage("");
    setUploadError(null);
    setUploaded([]);
  }, [testCase]);

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
      setUploadError(null);
      if (fileInput.current) fileInput.current.value = "";
    },
    onError: (err) => setUploadError(err),
  });

  return (
    <section>
      <h1>Dataset</h1>

      {limits.isPending && <Loading what="上傳規則" />}
      <ReadFailure error={limits.error} what="上傳規則">
        <p role="alert">無法讀取上傳規則,因此暫時不能上傳:{limits.error?.message}</p>
      </ReadFailure>

      {limits.data && (
        <>
          <h2>上傳前請先確認</h2>
          <dl>
            <dt>大小限制</dt>
            <dd>
              單一檔案最大 {bytes(limits.data.max_file_bytes)};同一個 Test Case 合計最大{" "}
              {bytes(limits.data.max_test_case_bytes)}、最多 {limits.data.max_files_per_test_case}{" "}
              個檔案。
              <p className="note">
                這一頁不知道這個 Test Case 已經用掉多少：已上傳的檔案、每個檔案的大小與合計,在{" "}
                {testCase === "" ? (
                  "Test Case 頁的「測試資料」那一節"
                ) : (
                  <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId: testCase }}>
                    這個 Test Case 的「測試資料」那一節
                  </Link>
                )}
                。超過上限時伺服器會擋下來並說明原因。
              </p>
            </dd>

            <dt>支援格式</dt>
            <dd>
              <ul>
                {limits.data.allowed_kinds.map((k) => (
                  <li key={k}>{k}</li>
                ))}
              </ul>
              <p data-role="teaching">檔案類型以內容判定,不看副檔名。</p>
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
              這個頁面需要 <code>?test_case=</code>。請到{" "}
              <Link to="/lab/test-cases">Test Case 頁</Link> 建立或選一個 Test Case,再從那裡連過來。
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
                {upload.isPending ? "上傳中…" : "上傳"}
              </button>
            </>
          )}
        </>
      )}

      {message && <p role="alert">{message}</p>}
      <ReadFailure error={uploadError} what="上傳">
        <p role="alert">
          {uploadError instanceof ApiError && [400, 413, 415].includes(uploadError.status)
            ? uploadError.message
            : "上傳沒有成功，可以再按一次。"}
        </p>
      </ReadFailure>

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
