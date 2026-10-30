import { ApiError, userFacingReason } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { usePackagingPreview } from "../../packaging.service";
import { PreviewReport } from "./PreviewReport";
import { RetentionNotice } from "./RetentionNotice";

function PreviewErrorReason({ error }: { error: unknown }) {
  if (error instanceof ApiError && error.status === 404) {
    return <p role="alert">這個版本讀不到，可能已經不屬於這個小工具。回上一步重新挑一次版本。</p>;
  }
  if (error instanceof ApiError && error.status === 503) {
    return (
      <p role="alert">
        {userFacingReason(error)
          ? `無法確認預覽：${userFacingReason(error)}。`
          : "打包服務目前不可用，無法確認預覽。請再試一次。"}
      </p>
    );
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
  const versionGone = preview.error instanceof ApiError && preview.error.status === 404;
  return (
    <>
      <h2 id="packaging-preview-heading" tabIndex={-1}>
        打包預覽
      </h2>
      {preview.isFetching && target !== "" && (
        <p role="status">
          {preview.isPending ? "計算這些設定會產生什麼…" : "正在重新讀取打包預覽…"}
        </p>
      )}
      <ReadFailure
        error={preview.error}
        what="打包預覽"
        retry={versionGone ? undefined : () => void preview.refetch()}
        retrying={preview.isFetching}
        focusTarget="packaging-preview-heading"
      >
        <PreviewErrorReason error={preview.error} />
      </ReadFailure>
      {!preview.error && !preview.isFetching && preview.data && (
        <PreviewReport preview={preview.data} />
      )}

      {!preview.error && !preview.isFetching && preview.data?.allowed && (
        <RetentionNotice preview={preview.data} />
      )}
    </>
  );
}
