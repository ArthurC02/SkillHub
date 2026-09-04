import { Loading } from "../components/Loading";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { apiFetch } from "../api/client";
import type { DataRetentionPolicy } from "../api/types";

/**
 * 02:O11Y-004 / 04 丙-25② — the data policy page, and specifically the analytics
 * half of it, because that is the one data class a user produces without
 * submitting anything. Everything else on the platform starts with somebody
 * typing or uploading; this starts with them arriving.
 *
 * **Every number here is read from the server, none is written into this file.**
 * `retention_days` comes from the deployment's ANALYTICS_RETENTION through
 * GET /policy/data-retention, so a page that says "180 天" only ever says it
 * because the running platform applies 180 days. When nothing is configured the
 * page says 目前不收集 — which is not a placeholder but the honest state, and the
 * visible form of NFR-002「未定值前不得開始收集」. Hard-coding ADR-029's proposed
 * 180 would be exactly the 04 乙-2 mistake: a policy displayed and enforced
 * nowhere.
 *
 * Public, like the endpoint behind it: a policy you have to log in to read is not
 * one you can decide by.
 *
 * Scope note: this page covers the analytics class and points at where the other
 * classes are deleted. The full per-class retention table lives in
 * gate-test/consent-and-data-policy.md §3 and is **not** copied here, because
 * those numbers are PDM-006 proposals that no code enforces yet — restating them
 * as product copy would turn an unratified proposal into a promise.
 */
export function DataPolicy() {
  // A `useQuery` in a page rather than a hook in `api/`, and deliberately, for
  // all three of the app's page-local reads (this one, `DatasetUpload`'s limits,
  // `RunPreflight`'s summary): each has exactly one call site and its own
  // options — `RunPreflight` needs `staleTime: 0`/`gcTime: 0` because a cached
  // permission summary would be confirmed against a hash the server has already
  // stopped accepting. A hook per single caller is an abstraction with one
  // implementation. What `api/` actually owns is the convention, and all three
  // follow it: `retry: false` (資訊架構 §5 IA-6), and no request built anywhere
  // but `apiFetch`.
  const policy = useQuery({
    queryKey: ["policy", "data-retention"],
    queryFn: () => apiFetch<DataRetentionPolicy>("/policy/data-retention"),
    retry: false,
  });

  return (
    <section>
      <h1>資料保存政策</h1>
      <p className="note" data-role="teaching">
        這一頁講兩件事：平台在你沒有主動送出任何東西的情況下記了什麼，以及你要刪掉自己的東西時該去哪裡。
      </p>

      <h2>使用行為分析事件</h2>
      {policy.isPending && <Loading what="分析事件政策" />}
      {policy.error && (
        <p role="alert">
          無法讀取分析事件政策：{policy.error.message}
          。讀不到不等於沒有收集，這一頁不會替伺服器回答這個問題。
        </p>
      )}

      {policy.data && (
        <>
          {policy.data.collecting ? (
            /* §2.2: the count is the list's length, not a numeral typed here.
               「四個」 was written three times against a list the server
               supplies — the shipped fixture has two — which is the same class
               of defect as a hardcoded retention window, just cheaper to hit. */
            <p>
              這個部署<strong>有</strong>在收集下面 {policy.data.events.length} 個事件，保存
              <strong>{policy.data.retention_days} 天</strong>
              ，到期後刪除。分析事件用的 cookie 也是同一個期限。
            </p>
          ) : (
            /* NFR-002 made visible: no retention value, no collection, and the
               page says so rather than showing a blank or a proposed number. */
            <p>
              這個部署<strong>目前不收集</strong>
              使用行為分析事件：沒有設定保存期限，所以一列都不寫，cookie 也不發。
              保存期限定案之前不會開始收集——這是規則，不是還沒做完。下面仍然列出
              {policy.data.events.length} 個事件，因為「現在不收」本身就是需要說清楚的一件事。
            </p>
          )}

          <p className="note">{policy.data.note}</p>

          <div className="table-scroll" tabIndex={0}>
            <table className="compare-table">
              <caption>
                全部只有這 {policy.data.events.length} 個事件。要再加一個，得先說明既有的資料表
                為什麼答不出那個問題。
              </caption>
              <thead>
                <tr>
                  <th scope="col">事件</th>
                  <th scope="col">什麼時候產生</th>
                  <th scope="col">記了哪些欄位</th>
                  <th scope="col">沒有記什麼</th>
                </tr>
              </thead>
              <tbody>
                {policy.data.events.map((event) => (
                  <tr key={event.name}>
                    <th scope="row">
                      <code>{event.name}</code>
                    </th>
                    <td>{event.when}</td>
                    <td>
                      {/* The whitelist verbatim: these are column names, and
                        translating them would break the link between what the
                        page claims and what the table holds. */}
                      <ul className="risk-list">
                        {event.attributes.map((attribute) => (
                          <li key={attribute}>
                            <code>{attribute}</code>
                          </li>
                        ))}
                      </ul>
                    </td>
                    <td>{event.not_recorded}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/*
            04 丙-154 ②. `feedback` reached this endpoint's response body since
            POST /feedback existed; this section is the first time a screen reads
            it. What it discloses is a fact about the report pipeline
            (BETA-003/004/005), not about the events above, so it gets its own
            heading rather than a row in that table.
          */}
          <h2>回報問題的資料</h2>
          <p>{policy.data.feedback.what}</p>
          <ul className="risk-list">
            {policy.data.feedback.collected.map((column) => (
              <li key={column}>
                <code>{column}</code>
              </li>
            ))}
          </ul>
          <p>{policy.data.feedback.free_text}</p>
          <p>{policy.data.feedback.page_path}</p>
          <p>{policy.data.feedback.run_id}</p>
          <p>{policy.data.feedback.on_account_deletion}</p>
          {policy.data.feedback.retention_days !== null ? (
            <p>
              保存 <strong>{policy.data.feedback.retention_days} 天</strong>，到期後刪除。
            </p>
          ) : (
            <p className="note">保存期限：尚未定值。{policy.data.feedback.note}</p>
          )}
        </>
      )}

      <h2>你的東西怎麼刪</h2>
      {/*
        §2.2 / this file's own docstring: the per-class retention numbers are
        PDM-006 proposals no code enforces, which is why the table above is not
        copied here — and two of those numbers were then written into this list
        anyway (版本快照 30 天、帳號 30 天寬限期). They are removed rather than
        re-sourced: neither is the analytics `retention_days` this page reads,
        so there is no server value here to route them through. Each destination
        page states its own scope at the moment of the deletion, and one of them
        (WorkspaceAccount) gets the real end date from the server.
      */}
      <p className="note" data-role="teaching">
        刪除都是分兩步的：按下去之後會先說明這一次刪掉的是什麼、什麼會留下、期限多長，確認才真的執行。
      </p>
      <ul className="risk-list">
        <li>
          <Link to="/workspace/skills">我的 Skill</Link>
          ：刪掉一個 Skill。版本快照會凍結保留，不隨這次刪除消失，誤刪還有救；別人 Fork
          過的版本不受影響。
        </li>
        <li>
          <Link to="/workspace/downloads">下載紀錄</Link>
          ：刪掉打包好的檔案。「你下載過幾次」的紀錄會留著，因為那件事發生過。
        </li>
        <li>
          <Link to="/workspace/runs">Run 歷史</Link>：進到某一次 Run
          可以刪掉它的產出檔案。執行紀錄與評估判定保留，引用過該檔案的評估會顯示證據已不存在。
        </li>
        <li>
          <Link to="/workspace/account">帳號</Link>
          ：刪掉整個帳號。申請之後有一段寬限期，期間隨時可以取消，實際結束日期由伺服器算出來顯示在
          那一頁；哪些會實體刪除、哪些會保留但去掉你的身分，也由伺服器在申請後逐條列出。
        </li>
      </ul>

      <h2>這一頁沒有回答的事</h2>
      <p className="note">
        每一類資料各自保存多久（上傳的資料集、Run
        產出、Trace、稽核事件……）仍在核定中，核定前不會寫在這裡當成承諾。 試跑會把你的 Prompt
        與相關內容送往模型供應商，那一段在試跑前的權限確認畫面上逐項列出。
      </p>
    </section>
  );
}
