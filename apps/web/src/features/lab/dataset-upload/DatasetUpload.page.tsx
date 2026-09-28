import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Link, useSearch } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { ApiError } from "../../../core/api/client";
import { useDatasetLimits, useUploadDataset, type Dataset } from "../lab.service";
import { useTestCaseDatasets } from "../testcases.service";
import { roundedBytes, uploadRefusal, type TestCaseUsage } from "./upload.model";
import { UploadRulesFacts } from "./components/UploadRulesFacts";

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
          <UploadRulesFacts limits={limits.data} used={used} testCase={testCase} />

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
