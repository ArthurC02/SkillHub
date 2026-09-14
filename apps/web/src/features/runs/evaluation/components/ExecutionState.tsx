import { runStatusLabel } from "../../runs.model";

export function ExecutionState({ runStatus }: { runStatus: string }) {
  return (
    <p className="note">
      執行狀態：{runStatusLabel(runStatus)}（<code>{runStatus}</code>）。
      這說的是工作負載跑完了沒有，不是任務達成了沒有。
    </p>
  );
}
