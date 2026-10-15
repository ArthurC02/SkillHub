import type { CreationSessionRead } from "../../creation.service";
import { Findings } from "../../../../shared/ui/Findings";
import type { DraftInspection } from "../create.model";

export function DraftFindings({
  inspection,
  read,
}: {
  inspection: DraftInspection | undefined;
  read: CreationSessionRead;
}) {
  if (inspection)
    return "refusal" in inspection ? (
      <p>{inspection.refusal}</p>
    ) : (
      <Findings findings={inspection.findings} level={4} />
    );
  return (
    <div>
      <p id="draft-inspection-unavailable" role={read.fetchStatus === "idle" ? "alert" : "status"}>
        {read.fetchStatus === "paused"
          ? "等待連線以重新讀取草稿檢查結果…"
          : read.fetchStatus === "fetching"
            ? "正在重新讀取草稿檢查結果…"
            : "草稿檢查細節尚未確認。請先重新讀取，再決定是否保存。"}
      </p>
      <button
        type="button"
        disabled={read.fetchStatus !== "idle"}
        onClick={() => void read.refetch()}
      >
        重新讀取檢查結果
      </button>
    </div>
  );
}
