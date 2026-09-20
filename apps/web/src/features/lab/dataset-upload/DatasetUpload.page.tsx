import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Link, useSearch } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { ApiError } from "../../../core/api/client";
import { useDatasetLimits, useUploadDataset, type Dataset } from "../lab.service";
import { useTestCaseDatasets } from "../testcases.service";
import { roundedBytes, uploadRefusal, type TestCaseUsage } from "./upload.model";

type UploadSearch = { test_case?: string };

export function DatasetUpload() {
  const { test_case: testCase = "" } = useSearch({ strict: false }) as UploadSearch;
  // `test_case` is a search param, so the route does not remount; the key does.
  return <DatasetUploadForm key={testCase} testCase={testCase} />;
}

function DatasetUploadForm({ testCase }: { testCase: string }) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [message, setMessage] = useState("");
  const [uploaded, setUploaded] = useState<Dataset[]>([]);
  const limits = useDatasetLimits();
  const stored = useTestCaseDatasets(testCase);
  const upload = useUploadDataset(testCase);
  const uploadError = upload.error;
  const used: TestCaseUsage | undefined = stored.data && {
    fileCount: stored.data.datasets.length,
    totalBytes: stored.data.total_bytes,
  };

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
              單一檔案最大 {roundedBytes(limits.data.max_file_bytes)};同一個 Test Case 合計最大{" "}
              {roundedBytes(limits.data.max_test_case_bytes)}、最多{" "}
              {limits.data.max_files_per_test_case} 個檔案。
              {used ? (
                <p className="note">
                  這個 Test Case 已經用掉 {used.fileCount} 個檔案、
                  {roundedBytes(used.totalBytes)}，還可以再上傳{" "}
                  {limits.data.max_files_per_test_case - used.fileCount} 個檔案、
                  {roundedBytes(limits.data.max_test_case_bytes - used.totalBytes)}。 每個檔案在{" "}
                  {testCase === "" ? (
                    "Test Case 頁的「測試資料」那一節"
                  ) : (
                    <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId: testCase }}>
                      這個 Test Case 的「測試資料」那一節
                    </Link>
                  )}
                  可以逐一刪除。
                </p>
              ) : (
                <p className="note">正在讀這個 Test Case 已經用掉多少…</p>
              )}
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
                  if (used) {
                    const refusal = uploadRefusal(file, limits.data, used);
                    if (refusal !== "") {
                      setMessage(refusal);
                      return;
                    }
                  }
                  upload.mutate(file, {
                    onSuccess: (d) => {
                      setUploaded((prev) => [...prev, d]);
                      setMessage("");
                      if (fileInput.current) fileInput.current.value = "";
                    },
                  });
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
              已上傳 {d.file_name}（{roundedBytes(d.size_bytes)}）
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
