import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { ApiError } from "../../../core/api/client";
import { useDatasetLimits, useUploadDataset, type Dataset } from "../lab.service";
import { useTestCase, useTestCaseDatasets, type TestCase } from "../testcases.service";
import { roundedBytes, uploadRefusal, type TestCaseUsage } from "./upload.model";
import { UploadRulesFacts } from "./components/UploadRulesFacts";
import { SkillWorkspaceNav } from "../../skill";
import "./DatasetUpload.page.css";

type UploadSearch = { version?: string };

export function DatasetUpload() {
  const { testCaseId } = useParams({ from: "/lab/test-cases/$testCaseId/datasets" });
  const { version } = useSearch({ strict: false }) as UploadSearch;
  const testCase = useTestCase(testCaseId);

  if (testCase.isPending) return <Loading what="Test Case" />;
  if (testCase.error instanceof ApiError && testCase.error.status === 404) {
    return <p role="alert">找不到這個 Test Case。</p>;
  }
  if (testCase.error) return <ReadFailure error={testCase.error} what="Test Case" />;

  return <DatasetUploadForm key={testCaseId} testCase={testCase.data} version={version} />;
}

function DatasetUploadForm({ testCase, version }: { testCase: TestCase; version?: string }) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [message, setMessage] = useState("");
  const [uploaded, setUploaded] = useState<Dataset[]>([]);
  const limits = useDatasetLimits();
  const stored = useTestCaseDatasets(testCase.test_case_id);
  const upload = useUploadDataset(testCase.test_case_id);
  const uploadError = upload.error;
  const used: TestCaseUsage | undefined = stored.data && {
    fileCount: stored.data.datasets.length,
    totalBytes: stored.data.total_bytes,
  };

  return (
    <section className="dataset-upload-page">
      <h1>{testCase.name} 的 Dataset</h1>
      <SkillWorkspaceNav
        skillId={testCase.skill_id}
        versionId={version}
        testCaseId={testCase.test_case_id}
      />
      <p className="note">
        <Link
          to="/lab/test-cases/$testCaseId"
          params={{ testCaseId: testCase.test_case_id }}
          search={{ version }}
        >
          回到這個 Test Case
        </Link>
      </p>

      {limits.isPending && <Loading what="上傳規則" />}
      <ReadFailure error={limits.error} what="上傳規則">
        <p role="alert">無法讀取上傳規則,因此暫時不能上傳:{limits.error?.message}</p>
      </ReadFailure>

      {limits.data && (
        <>
          <section className="dataset-upload-guide" aria-labelledby="dataset-guide-heading">
            <h2 id="dataset-guide-heading">上傳前請先確認</h2>
            <UploadRulesFacts limits={limits.data} used={used} testCase={testCase.test_case_id} />
          </section>

          <section className="dataset-upload-picker" aria-labelledby="dataset-picker-heading">
            <div className="dataset-upload-picker-copy">
              <p className="page-eyebrow">Dataset intake</p>
              <h2 id="dataset-picker-heading">加入測試資料</h2>
              <p>每次選擇一個檔案；平台會在送出前先檢查目前可用的數量與容量。</p>
            </div>
            <div className="field">
              <label htmlFor="dataset-file">選擇檔案</label>
              <input id="dataset-file" type="file" ref={fileInput} />
            </div>
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
          </section>
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
