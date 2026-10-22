import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import { useRef, useState, type ReactNode } from "react";
import { ApiError, SessionNotReadyError } from "../../../core/api/client";
import { useMe } from "../../../core/session/me.service";
import {
  useDatasetLimits,
  useUploadDataset,
  isWriteResultUnknown,
  type Dataset,
  type DatasetLimits,
} from "../lab.service";
import { useTestCase, useTestCaseDatasets, type TestCase } from "../testcases.service";
import { roundedBytes, uploadRefusal, type TestCaseUsage } from "./upload.model";
import { UploadRulesFacts } from "./components/UploadRulesFacts";
import { UploadFailure } from "./components/UploadFailure";
import { SkillWorkspaceNav } from "../../skill";
import "./DatasetUpload.page.css";

type UploadSearch = { version?: string };
type DatasetUploadFailure = {
  error: unknown;
  fileName?: string;
  blockedError?: SessionNotReadyError;
};

export function DatasetUpload() {
  const { testCaseId } = useParams({ from: "/lab/test-cases/$testCaseId/datasets" });
  const { version } = useSearch({ strict: false }) as UploadSearch;
  const testCase = useTestCase(testCaseId);
  const data = testCase.error ? undefined : testCase.data;

  return (
    <section className="dataset-upload-page">
      <h1 id="dataset-heading" tabIndex={-1}>
        {data ? `${data.name} 的 Dataset` : "測試資料"}
      </h1>
      {testCase.isPending && <Loading what="測試題" />}
      <ReadFailure
        error={testCase.error}
        what="測試題"
        retry={() => void testCase.refetch()}
        retrying={testCase.isFetching}
        focusTarget="dataset-heading"
      >
        {testCase.error instanceof ApiError && testCase.error.status === 404 && (
          <p role="alert">找不到這個測試題。</p>
        )}
      </ReadFailure>
      {data ? (
        <DatasetUploadForm key={testCaseId} testCase={data} version={version} />
      ) : (
        <p>
          <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId }} search={{ version }}>
            回到這個測試題
          </Link>
        </p>
      )}
    </section>
  );
}

function DatasetUploadForm({ testCase, version }: { testCase: TestCase; version?: string }) {
  const limits = useDatasetLimits();
  const stored = useTestCaseDatasets(testCase.test_case_id);
  const rules = limits.isSuccess && limits.fetchStatus === "idle" ? limits.data : undefined;
  const storedData = stored.isSuccess && stored.fetchStatus === "idle" ? stored.data : undefined;
  const used: TestCaseUsage | undefined = storedData && {
    fileCount: storedData.datasets.length,
    totalBytes: storedData.total_bytes,
  };

  return (
    <>
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
          回到這個測試題
        </Link>
      </p>
      {limits.isPending && <Loading what="上傳規則" />}
      <ReadFailure
        error={limits.error}
        what="上傳規則"
        retry={() => void limits.refetch()}
        retrying={limits.fetchStatus !== "idle"}
        focusTarget="dataset-heading"
      >
        <p role="alert">暫時無法讀取上傳規則；規則恢復前不會送出檔案。</p>
      </ReadFailure>
      <ReadFailure
        error={stored.error}
        what="測試資料已用容量"
        retry={() => void stored.refetch()}
        retrying={stored.fetchStatus !== "idle"}
        focusTarget="dataset-heading"
      />
      <DatasetIntake
        testCaseId={testCase.test_case_id}
        limits={rules}
        used={used}
        datasets={storedData?.datasets}
        onRetryRead={() => void stored.refetch()}
        reading={stored.fetchStatus !== "idle"}
        waiting={limits.isPending || stored.isPending || limits.isFetching || stored.isFetching}
        paused={limits.fetchStatus === "paused" || stored.fetchStatus === "paused"}
      >
        {rules && (
          <section className="dataset-upload-guide" aria-labelledby="dataset-guide-heading">
            <h3 id="dataset-guide-heading">上傳前請先確認</h3>
            <UploadRulesFacts
              limits={rules}
              used={used}
              usageFailed={Boolean(stored.error)}
              testCase={testCase.test_case_id}
            />
          </section>
        )}
      </DatasetIntake>
    </>
  );
}

type DatasetIntakeProps = {
  testCaseId: string;
  limits: DatasetLimits | undefined;
  used: TestCaseUsage | undefined;
  datasets: Dataset[] | undefined;
  onRetryRead: () => void;
  reading: boolean;
  waiting: boolean;
  paused: boolean;
  children: ReactNode;
};

function DatasetIntake({
  testCaseId,
  limits,
  used,
  datasets,
  onRetryRead,
  reading,
  waiting,
  paused,
  children,
}: DatasetIntakeProps) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [message, setMessage] = useState("");
  const [uploaded, setUploaded] = useState<Dataset[]>([]);
  const [unknownUpload, setUnknownUpload] = useState<DatasetUploadFailure>();
  const me = useMe();
  const upload = useUploadDataset(testCaseId);
  const identityConfirmed = me.isSuccess && me.fetchStatus === "idle";
  const readUnavailable = !limits || !used;
  const failedUpload = readUploadFailure(upload, unknownUpload);

  return (
    <>
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
        {children}
        {readUnavailable && (
          <p id="upload-read-why" role="status" className="note">
            {paused
              ? "等待連線以確認上傳規則與已用容量；確認後才能上傳。"
              : waiting
                ? "正在確認上傳規則與已用容量；確認後才能上傳。"
                : "目前無法確認上傳規則或已用容量；請先重試讀取。"}
          </p>
        )}
        {!identityConfirmed && (
          <p id="upload-session-why" role="status" className="note">
            登入狀態尚未確認；請先在頁首確認，再上傳測試資料。
          </p>
        )}
        <button
          type="button"
          disabled={upload.isPending || readUnavailable || !identityConfirmed}
          aria-describedby={
            !identityConfirmed
              ? "upload-session-why"
              : readUnavailable
                ? "upload-read-why"
                : undefined
          }
          onClick={() => {
            if (!limits || !used || !identityConfirmed || upload.isPending) return;
            const file = fileInput.current?.files?.[0];
            if (!file) {
              setMessage("請先選擇一個檔案。");
              return;
            }
            const refusal = uploadRefusal(file, limits, used);
            if (refusal !== "") {
              setMessage(refusal);
              return;
            }
            upload.mutate(file, {
              onError: (error) => {
                if (error instanceof SessionNotReadyError) return;
                setUnknownUpload(
                  isWriteResultUnknown(error) ? { error, fileName: file.name } : undefined,
                );
              },
              onSuccess: (d) => {
                setUnknownUpload(undefined);
                setUploaded((prev) => [...prev, d]);
                setMessage("");
                if (fileInput.current?.files?.[0] === file) fileInput.current.value = "";
              },
            });
          }}
        >
          {upload.isPending ? "上傳中…" : "上傳"}
        </button>
      </section>
      {message && <p role="alert">{message}</p>}
      <UploadFailure
        error={failedUpload.error}
        fileName={failedUpload.fileName}
        blockedError={failedUpload.blockedError}
        datasets={datasets}
        onRetryRead={onRetryRead}
        reading={reading}
      />
      {uploaded.length > 0 && (
        <section className="dataset-upload-success" role="status" aria-label="上傳完成">
          <h2>已加入的測試資料</h2>
          <ul>
            {uploaded.map((d) => (
              <li key={d.dataset_id}>
                已上傳 {d.file_name}（{roundedBytes(d.size_bytes)}）
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

function readUploadFailure(
  upload: ReturnType<typeof useUploadDataset>,
  previous?: DatasetUploadFailure,
): DatasetUploadFailure {
  return upload.error instanceof SessionNotReadyError && previous
    ? { ...previous, blockedError: upload.error }
    : { error: upload.error, fileName: upload.variables?.name };
}
