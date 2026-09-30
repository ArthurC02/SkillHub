import { ApiError } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { usePackagingPreview } from "../../packaging.service";
import { PreviewReport } from "./PreviewReport";
import { RetentionNotice } from "./RetentionNotice";

function PreviewErrorReason({ error }: { error: unknown }) {
  if (error instanceof ApiError && error.status === 404) {
    return <p role="alert">這個版本讀不到，可能已經不屬於這個 Skill。回上一步重新挑一次版本。</p>;
  }
  if (error instanceof ApiError && error.status === 503) {
    return <p role="alert">這個部署沒有設定任何打包目標，所以沒有預覽。</p>;
  }
  return <p role="alert">暫時無法讀取打包預覽。請重新整理，或稍後再試。</p>;
}

export function PackagingPreviewSection({
  preview,
  target,
}: {
  preview: ReturnType<typeof usePackagingPreview>;
  target: string;
}) {
  return (
    <>
      <h2>打包預覽</h2>
      {preview.isPending && target !== "" && <p>計算這些設定會產生什麼…</p>}
      <ReadFailure error={preview.error} what="打包預覽">
        <PreviewErrorReason error={preview.error} />
      </ReadFailure>
      {preview.data && <PreviewReport preview={preview.data} />}

      {preview.data?.allowed && <RetentionNotice preview={preview.data} />}
    </>
  );
}
