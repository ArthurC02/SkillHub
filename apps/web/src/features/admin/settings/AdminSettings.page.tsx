import { useJudgePanelSwitch, usePlatformSettings, type PlatformSettings } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ActionForm } from "../components/ActionForm";

function JudgePanel({ panel }: { panel: PlatformSettings["judge_panel"] }) {
  const toggle = useJudgePanelSwitch();
  const next = !panel.enabled;
  return (
    <section aria-labelledby="admin-settings-judge-panel-heading">
      <h2 id="admin-settings-judge-panel-heading">評估判定的評審團</h2>
      <p className="badge-row">
        <span className={panel.enabled ? "badge badge-danger" : "badge"}>
          {panel.enabled ? "開啟：三個判定逐條多數決" : "關閉：一個判定（預設）"}
        </span>
      </p>
      <p className="note">
        開啟後，每次評估由三個獨立的判定各判一次，每條驗收條件取過半數的結果；沒有過半就記為無法判定。
        判定的費用約為三倍，任一個判定失敗整次評估就失敗。下一次評估就會照新的設定。
      </p>
      {panel.reason && (
        <>
          <p>理由：{panel.reason}</p>
          {panel.set_at && (
            <p>
              設定於 <Timestamp at={panel.set_at} />
            </p>
          )}
        </>
      )}
      <ActionForm
        id="admin-settings-judge-panel"
        submitLabel={next ? "開啟評審團" : "關閉評審團"}
        tone={next ? "caution" : undefined}
        pending={toggle.isPending}
        error={toggle.error}
        done={
          toggle.isSuccess &&
          (toggle.variables.enabled
            ? "已開啟，下一次評估由三個判定投票。"
            : "已關閉，下一次評估回到一個判定。")
        }
        onSubmit={(note) => toggle.mutate({ enabled: next, note })}
      />
    </section>
  );
}

export function AdminSettings() {
  const settings = usePlatformSettings();
  return (
    <AdminPage heading="平台設定">
      <p className="note">
        影響整個平台的開關。每一次改動都要寫理由，並記進動作紀錄。平台 Agent 的每日花費上限在平台
        Agent 頁調整。
      </p>
      {settings.isPending && <Loading what="平台設定" />}
      <ReadFailure error={settings.error} what="平台設定" />
      {settings.data && <JudgePanel panel={settings.data.judge_panel} />}
    </AdminPage>
  );
}
