import { useRosters } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { AdminPage } from "../components/AdminPage";

export function AdminRosters() {
  const rosters = useRosters();
  return (
    <AdminPage heading="名冊" lede="要改名冊只能改部署設定再重啟，後台不提供編輯。">
      {rosters.isPending && <Loading what="名冊" />}
      <ReadFailure error={rosters.error} what="名冊">
        <p role="alert">
          暫時無法讀取名冊。{rosters.data ? "先前載入的名冊已隱藏。" : "請稍後再試。"}
        </p>
        <button type="button" disabled={rosters.isFetching} onClick={() => void rosters.refetch()}>
          {rosters.isFetching ? "重新讀取中…" : "再試一次"}
        </button>
      </ReadFailure>
      {rosters.data && !rosters.error && (
        <>
          <ListFreshness
            inFlight={false}
            showWhenIdle
            updatedAt={rosters.dataUpdatedAt}
            fetching={rosters.isFetching}
            refetch={rosters.refetch}
            subject="名冊"
          />
          <h2>封測准入</h2>
          {rosters.data.beta_allowlist.length === 0 ? (
            <p>這個部署沒有設定封測名單：每一個登入的帳號都算受邀。</p>
          ) : (
            <>
              <p>名單上記的是登入服務的使用者 id，不是 email。</p>
              <ul>
                {rosters.data.beta_allowlist.map((id) => (
                  <li key={id}>
                    <code>{id}</code>
                  </li>
                ))}
              </ul>
            </>
          )}
          <h2>operator 名冊</h2>
          <p>目前 {rosters.data.operator_user_ids.length} 位。</p>
          {rosters.data.operator_user_ids.length > 0 && (
            <ul>
              {rosters.data.operator_user_ids.map((id) => (
                <li key={id}>
                  <code>{id}</code>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </AdminPage>
  );
}
