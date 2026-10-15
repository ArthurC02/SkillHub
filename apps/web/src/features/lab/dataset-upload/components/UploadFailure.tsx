import { useEffect, useRef } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { isUploadResultUnknown, type Dataset } from "../../lab.service";
import { roundedBytes, uploadErrorSentence } from "../upload.model";
import "./UploadFailure.css";

export function UploadFailure({
  error,
  fileName,
  datasets,
  onRetryRead,
  reading,
}: {
  error: unknown;
  fileName: string | undefined;
  datasets: Dataset[] | undefined;
  onRetryRead: () => void;
  reading: boolean;
}) {
  const unknown = Boolean(error) && isUploadResultUnknown(error);
  const message = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (unknown) message.current?.focus();
  }, [error, unknown]);
  return (
    <ReadFailure error={error} what="上傳">
      <section className="dataset-upload-outcome" aria-label="上傳結果">
        <p ref={message} role="alert" tabIndex={-1}>
          {uploadErrorSentence(error)}
        </p>
        {fileName && <p className="note">送出檔案：{fileName}</p>}
        {unknown && (
          <>
            <h2 id="dataset-upload-result-heading" tabIndex={-1}>
              伺服器目前回報的檔案
            </h2>
            <button
              type="button"
              disabled={reading}
              onClick={() => {
                onRetryRead();
                document.getElementById("dataset-upload-result-heading")?.focus();
              }}
            >
              重新確認檔案清單
            </button>
            {datasets ? (
              datasets.length > 0 ? (
                <ul className="file-tree" data-role="evidence">
                  {datasets.map((dataset) => (
                    <li key={dataset.dataset_id}>
                      {dataset.file_name}（{roundedBytes(dataset.size_bytes)}）
                    </li>
                  ))}
                </ul>
              ) : (
                <p role="status">目前清單沒有檔案。</p>
              )
            ) : (
              <p role="status">檔案清單尚未確認。</p>
            )}
          </>
        )}
      </section>
    </ReadFailure>
  );
}
