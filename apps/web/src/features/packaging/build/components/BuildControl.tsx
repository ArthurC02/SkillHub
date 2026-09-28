import { ApiError } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { useCreateDownload, usePackagingPreview } from "../../packaging.service";

const DEAD_REASON_ID = "packaging-build-disabled-reason";

function BuildErrorReason({ error }: { error: unknown }) {
  if (error instanceof ApiError && error.status === 403) {
    return (
      <p role="alert">
        這個帳號還沒有封測邀請，所以套件沒有建立。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。
      </p>
    );
  }
  return <p role="alert">套件沒有建立成功，可以再按一次。</p>;
}

export function BuildControl({
  build,
  preview,
  deadReason,
  onBuild,
}: {
  build: ReturnType<typeof useCreateDownload>;
  preview: ReturnType<typeof usePackagingPreview>;
  deadReason: string;
  onBuild: () => void;
}) {
  return (
    <>
      <p className="note">平台目前只讓有封測邀請的帳號建立下載套件。</p>
      <ReadFailure error={build.error} what="套件建立">
        <BuildErrorReason error={build.error} />
      </ReadFailure>

      <p>
        <button
          type="button"
          className="action"
          disabled={!preview.data?.allowed || build.isPending}
          onClick={onBuild}
          aria-describedby={deadReason ? DEAD_REASON_ID : undefined}
        >
          {build.isPending ? "打包中…" : "建立下載套件"}
        </button>
      </p>
      {deadReason && (
        <p className="note" id={DEAD_REASON_ID}>
          {deadReason}
        </p>
      )}
    </>
  );
}
