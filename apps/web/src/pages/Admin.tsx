import { Fragment, useState, type ReactNode } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useMe } from "../api/me";
import {
  useAccountLookup,
  useCostStatistics,
  useCreditLedger,
  useDispatchHalt,
  useDispatchStatus,
  useGovernance,
  useGovernanceAction,
  useGrantCredits,
  useOperatorAuditLog,
  useRosters,
  usd,
  type AccountLookup,
  type SkillGovernance,
} from "../api/admin";
import { ApiError } from "../api/client";
import { AdminNav } from "../components/AdminNav";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { RouteNotFound } from "../components/RouteNotFound";
import { Timestamp } from "../components/Timestamp";

function AdminPage({
  heading,
  lede,
  children,
}: {
  heading: string;
  lede?: ReactNode;
  children: ReactNode;
}) {
  const me = useMe();
  if (me.isPending) return <Loading what="" />;
  if (me.data?.operator !== true) return <RouteNotFound />;
  return (
    <section>
      <AdminNav />
      <h1>{heading}</h1>
      {lede && <p className="note">{lede}</p>}
      {children}
    </section>
  );
}

const notFound = (error: unknown) => error instanceof ApiError && error.status === 404;
const messageOf = (error: unknown) => (error instanceof Error ? error.message : String(error));

function WriteFailure({ error }: { error: unknown }) {
  return (
    <ReadFailure error={error} what="這個動作的結果">
      <p role="alert">沒有完成，伺服器說：{messageOf(error)}</p>
    </ReadFailure>
  );
}

function ActionForm({
  id,
  submitLabel,
  pending,
  error,
  done,
  ready = true,
  onSubmit,
  children,
}: {
  id: string;
  submitLabel: string;
  pending: boolean;
  error: unknown;
  done?: ReactNode;
  ready?: boolean;
  onSubmit: (note: string) => void;
  children?: ReactNode;
}) {
  const [note, setNote] = useState("");
  const blocked = note.trim() === "" || !ready;
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!blocked) onSubmit(note.trim());
      }}
    >
      {children}
      <div className="field">
        <label htmlFor={`${id}-note`}>理由（必填，會寫進動作紀錄）</label>
        <textarea
          id={`${id}-note`}
          value={note}
          onChange={(event) => setNote(event.target.value)}
        />
      </div>
      <button
        type="submit"
        disabled={blocked || pending}
        aria-describedby={blocked ? `${id}-why` : undefined}
      >
        {pending ? "送出中…" : submitLabel}
      </button>
      {blocked && (
        <p id={`${id}-why`} className="note">
          「{submitLabel}」要等上面的欄位都填好。
        </p>
      )}
      {done && <p role="status">{done}</p>}
      <WriteFailure error={error} />
    </form>
  );
}

export function AdminHome() {
  return (
    <AdminPage heading="營運後台">
      <ul className="download-list">
        <li className="download-item">
          <p>
            <Link to="/admin/accounts">
              <strong>帳號與點數</strong>
            </Link>
          </p>
          <p className="note">用 email 找帳號，看餘額與分錄，授予點數。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/skills" search={{}}>
              <strong>Skill 治理</strong>
            </Link>
          </p>
          <p className="note">找任何工作區的 Skill，設定受限展示、再散布判定或下架。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/dispatch">
              <strong>派送煞車</strong>
            </Link>
          </p>
          <p className="note">看平台有沒有在派送新的 Run，宣告或解除煞車。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/rosters">
              <strong>名冊</strong>
            </Link>
          </p>
          <p className="note">目前生效的 operator 名冊與封測名單，唯讀。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/audit-log">
              <strong>動作紀錄</strong>
            </Link>
          </p>
          <p className="note">全平台 operator 做過的事，包括每一次查帳號與查點數。</p>
        </li>
        <li className="download-item">
          <p>
            <Link to="/admin/cost-statistics">
              <strong>成本統計</strong>
            </Link>
          </p>
          <p className="note">每一種模型與沙箱呼叫最新的成本分布，不含使用者維度。</p>
        </li>
      </ul>
    </AdminPage>
  );
}

export function AdminAccounts() {
  const [draft, setDraft] = useState("");
  const [email, setEmail] = useState("");
  const account = useAccountLookup(email);

  return (
    <AdminPage
      heading="帳號與點數"
      lede="每查到一次帳號、每讀一次點數，都會留下一筆紀錄：誰在何時查了誰。"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          setEmail(draft.trim());
        }}
      >
        <div className="field">
          <label htmlFor="admin-account-email">Email</label>
          <input
            id="admin-account-email"
            type="email"
            required
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
        <button type="submit" className="action">
          查詢
        </button>
      </form>
      {account.isFetching && <Loading what="帳號" />}
      {notFound(account.error) ? (
        <p role="status">沒有 email 是「{email}」的帳號。已刪除的帳號查不到。</p>
      ) : (
        <ReadFailure error={account.error} what="帳號" />
      )}
      {account.data && !account.isFetching && <AccountCard account={account.data} />}
    </AdminPage>
  );
}

function AccountCard({ account }: { account: AccountLookup }) {
  return (
    <>
      <h2>{account.display_name}</h2>
      <dl>
        <dt>Email</dt>
        <dd>{account.email}</dd>
        <dt>User id</dt>
        <dd>
          <code>{account.user_id}</code>
        </dd>
        <dt>Workspace id</dt>
        <dd>
          <code>{account.workspace_id}</code>
        </dd>
        <dt>建立時間</dt>
        <dd>
          <Timestamp at={account.created_at} />
        </dd>
        <dt>刪除申請</dt>
        <dd>
          {account.deletion_requested_at ? (
            <>
              已申請，時間 <Timestamp at={account.deletion_requested_at} />
            </>
          ) : (
            "沒有申請"
          )}
        </dd>
        <dt>封測名單</dt>
        <dd>{account.in_beta_allowlist ? "在名單上，或這個部署沒有設定名單" : "不在名單上"}</dd>
      </dl>
      <CreditPanel workspaceId={account.workspace_id} />
    </>
  );
}

const ENTRY_KIND: Record<string, string> = {
  debit: "扣點",
  grant: "授予",
  topup: "儲值",
  adjustment: "更正",
};

function CreditPanel({ workspaceId }: { workspaceId: string }) {
  const ledger = useCreditLedger(workspaceId);
  return (
    <>
      <h2>點數</h2>
      {ledger.isPending && <Loading what="點數" />}
      <ReadFailure error={ledger.error} what="點數" />
      {ledger.data && (
        <>
          <p>
            目前餘額 <strong>{ledger.data.balance_credits}</strong> 點
          </p>
          {ledger.data.entries.length === 0 ? (
            <p>這個帳戶的分錄：0 筆。</p>
          ) : (
            <div className="table-scroll">
              <table>
                <caption>最近 50 筆分錄，新的在上面</caption>
                <thead>
                  <tr>
                    <th scope="col">時間</th>
                    <th scope="col">種類</th>
                    <th scope="col">點數</th>
                    <th scope="col">來源</th>
                  </tr>
                </thead>
                <tbody>
                  {ledger.data.entries.map((entry, index) => (
                    <tr key={`${entry.kind}-${index}`}>
                      <td>
                        <Timestamp at={entry.created_at} />
                      </td>
                      <td>
                        {ENTRY_KIND[entry.kind] ?? entry.kind}
                        {entry.estimated && "（估計值）"}
                      </td>
                      <td>
                        {entry.delta_credits > 0 ? `+${entry.delta_credits}` : entry.delta_credits}
                      </td>
                      <td>{entry.ref_type ?? "不適用"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="note">
            分錄不記授予的理由；誰在何時、以什麼理由授予，看{" "}
            <Link to="/admin/audit-log">動作紀錄</Link>。
          </p>
        </>
      )}
      <GrantForm workspaceId={workspaceId} />
    </>
  );
}

function GrantForm({ workspaceId }: { workspaceId: string }) {
  const grant = useGrantCredits(workspaceId);
  const [amount, setAmount] = useState("");
  const credits = Number(amount);
  const valid = amount.trim() !== "" && Number.isInteger(credits) && credits !== 0;
  return (
    <>
      <h3>授予點數</h3>
      <ActionForm
        id="admin-grant"
        submitLabel="授予"
        pending={grant.isPending}
        error={grant.error}
        ready={valid}
        done={
          grant.data &&
          `已授予 ${grant.data.amount_credits} 點，餘額現在是 ${grant.data.balance_credits} 點。`
        }
        onSubmit={(reason) => grant.mutate({ amount_credits: credits, reason })}
      >
        <div className="field">
          <label htmlFor="admin-grant-amount">點數（整數，不能是 0；負數是更正）</label>
          <input
            id="admin-grant-amount"
            type="number"
            step={1}
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
          />
        </div>
      </ActionForm>
    </>
  );
}

const REDISTRIBUTION: Record<string, string> = {
  allowed: "可以再散布",
  blocked: "禁止再散布",
  unknown: "尚未判定",
  self_supplied: "使用者自己提供",
  generated: "平台生成",
};

export function AdminSkills() {
  const { q = "" } = useSearch({ from: "/admin/skills" });
  const navigate = useNavigate();
  const [draft, setDraft] = useState(q);
  const skills = useGovernance(q);
  const found = skills.data?.skills ?? [];

  return (
    <AdminPage
      heading="Skill 治理"
      lede="範圍是所有工作區，含私人的與已下架的；只顯示治理狀態，不顯示內容。"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void navigate({ to: "/admin/skills", search: { q: draft.trim() || undefined } });
        }}
      >
        <div className="field">
          <label htmlFor="admin-skill-q">Skill id 或名稱</label>
          <input
            id="admin-skill-q"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
        <button type="submit" className="action">
          查詢
        </button>
      </form>
      {q !== "" && skills.isPending && <Loading what=" Skill" />}
      <ReadFailure error={skills.error} what=" Skill" />
      {skills.data &&
        (found.length === 0 ? (
          <p>沒有符合「{q}」的 Skill：0 筆。已刪除的 Skill 不會出現。</p>
        ) : (
          <ul className="download-list">
            {found.map((skill) => (
              <GovernanceRow key={skill.skill_id} skill={skill} single={found.length === 1} />
            ))}
          </ul>
        ))}
      {found.length === 1 && found[0].takedown_at === null && (
        <GovernanceActions skill={found[0]} />
      )}
    </AdminPage>
  );
}

function GovernanceRow({ skill, single }: { skill: SkillGovernance; single: boolean }) {
  return (
    <li className="download-item">
      <p>
        <strong>{skill.name}</strong>
      </p>
      <p className="note">
        Skill <code>{skill.skill_id}</code>｜工作區 <code>{skill.workspace_id}</code>
      </p>
      <p className="badge-row">
        <span className={skill.access_restriction ? "badge badge-unverified" : "badge"}>
          {skill.access_restriction ? `受限展示：${skill.access_restriction}` : "沒有受限"}
        </span>{" "}
        <span className="badge">
          再散布：{REDISTRIBUTION[skill.redistribution] ?? skill.redistribution}
        </span>{" "}
        {skill.takedown_at && <span className="badge badge-danger">已下架</span>}
      </p>
      {skill.takedown_at && (
        <p>
          下架於 <Timestamp at={skill.takedown_at} />
          ，理由：{skill.takedown_reason ?? "未測量"}。下架沒有恢復的路。
        </p>
      )}
      {!single && (
        <p>
          <Link to="/admin/skills" search={{ q: skill.skill_id }}>
            處理這一個
          </Link>
        </p>
      )}
    </li>
  );
}

function GovernanceActions({ skill }: { skill: SkillGovernance }) {
  const restriction = useGovernanceAction(skill.skill_id, "restriction");
  const redistribution = useGovernanceAction(skill.skill_id, "redistribution");
  const takedown = useGovernanceAction(skill.skill_id, "takedown");
  const [verdict, setVerdict] = useState("blocked");
  const [licenseExpression, setLicenseExpression] = useState("");
  const [licenseSource, setLicenseSource] = useState("");
  const [takedownReason, setTakedownReason] = useState("");
  const releasing = verdict === "allowed";

  return (
    <>
      <h2>對「{skill.name}」的動作</h2>
      <h3>{skill.access_restriction ? "解除受限展示" : "設定受限展示"}</h3>
      <p className="note">受限展示關掉全文與試跑，Skill 仍在搜尋裡。可以用同一個地方改回來。</p>
      <ActionForm
        id="admin-restriction"
        submitLabel={skill.access_restriction ? "解除受限" : "設定受限"}
        pending={restriction.isPending}
        error={restriction.error}
        done={restriction.isSuccess && "已送出，上面的狀態已更新。"}
        onSubmit={(note) =>
          skill.access_restriction
            ? restriction.mutate({ method: "DELETE", body: { note } })
            : restriction.mutate({ method: "PUT", body: { reason: "license-review", note } })
        }
      />

      <h3>再散布判定</h3>
      <ActionForm
        id="admin-redistribution"
        submitLabel="送出判定"
        pending={redistribution.isPending}
        error={redistribution.error}
        ready={!releasing || (licenseExpression.trim() !== "" && licenseSource !== "")}
        done={redistribution.isSuccess && "已送出，上面的狀態已更新。"}
        onSubmit={(note) =>
          redistribution.mutate({
            method: "PUT",
            body: releasing
              ? {
                  value: verdict,
                  note,
                  license_expression: licenseExpression.trim(),
                  license_source: licenseSource,
                }
              : { value: verdict, note },
          })
        }
      >
        <div className="field">
          <label htmlFor="admin-redistribution-value">判定</label>
          <select
            id="admin-redistribution-value"
            value={verdict}
            onChange={(event) => setVerdict(event.target.value)}
          >
            <option value="blocked">禁止再散布</option>
            <option value="unknown">尚未判定</option>
            <option value="allowed">可以再散布（要附授權證據）</option>
          </select>
        </div>
        {releasing && (
          <>
            <div className="field">
              <label htmlFor="admin-license-expression">授權條款（例如 MIT）</label>
              <input
                id="admin-license-expression"
                value={licenseExpression}
                onChange={(event) => setLicenseExpression(event.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="admin-license-source">證據來源</label>
              <select
                id="admin-license-source"
                value={licenseSource}
                onChange={(event) => setLicenseSource(event.target.value)}
              >
                <option value="">選一個</option>
                <option value="manifest">SKILL.md 的宣告</option>
                <option value="manifest-referenced-file">SKILL.md 指向的授權檔</option>
                <option value="package-license-file">套件裡的授權檔</option>
                <option value="repo-license-file">儲存庫根目錄的授權檔</option>
              </select>
            </div>
          </>
        )}
      </ActionForm>

      <h3>下架</h3>
      <div className="field">
        <label htmlFor="admin-takedown-reason">下架理由（必填，會寫進動作紀錄）</label>
        <input
          id="admin-takedown-reason"
          value={takedownReason}
          onChange={(event) => setTakedownReason(event.target.value)}
        />
      </div>
      {takedownReason.trim() === "" ? (
        <p className="note">填了理由才能下架。</p>
      ) : (
        <ConfirmDelete
          scopeId="admin-takedown-scope"
          scope="下架後這個 Skill 從目錄與搜尋消失，不能再下載或試跑；既有的 Run 仍可追溯。下架沒有恢復的路。"
          pending={takedown.isPending}
          label="下架"
          confirmLabel="確認下架"
          onConfirm={() =>
            takedown.mutate({ method: "PUT", body: { reason: takedownReason.trim() } })
          }
        />
      )}
      <WriteFailure error={takedown.error} />
    </>
  );
}

const HALT_SOURCE: Record<string, string> = {
  p1_incident: "P1 事故：只有人能解除",
  orphan_threshold: "孤兒門檻：連續兩輪低於門檻會自動解除",
};

export function AdminDispatch() {
  const status = useDispatchStatus();
  const declare = useDispatchHalt("PUT");
  const lift = useDispatchHalt("DELETE");
  const [provider, setProvider] = useState("");
  const target = provider.trim() || undefined;

  return (
    <AdminPage heading="派送煞車">
      {status.isPending && <Loading what="派送狀態" />}
      <ReadFailure error={status.error} what="派送狀態" />
      {status.data && (
        <>
          <p>
            <span className={status.data.dispatching ? "badge" : "badge badge-danger"}>
              {status.data.dispatching ? "正在派送" : "停止派送"}
            </span>
          </p>
          {status.data.halts.length === 0 ? (
            <p>煞車：0 個。</p>
          ) : (
            <ul className="download-list">
              {status.data.halts.map((halt) => (
                <li className="download-item" key={`${halt.target}-${halt.source}`}>
                  <p>
                    <strong>{halt.target === "pool" ? "整個叢集" : `節點 ${halt.target}`}</strong>
                  </p>
                  <p className="badge-row">
                    <span className="badge">{HALT_SOURCE[halt.source] ?? halt.source}</span>
                  </p>
                  <p>理由：{halt.reason}</p>
                  <p>
                    宣告於 <Timestamp at={halt.declared_at} />
                  </p>
                </li>
              ))}
            </ul>
          )}
        </>
      )}

      <h2>宣告或解除</h2>
      <div className="field">
        <label htmlFor="admin-halt-provider">節點名稱（留空是整個叢集）</label>
        <input
          id="admin-halt-provider"
          value={provider}
          onChange={(event) => setProvider(event.target.value)}
        />
      </div>
      <h3>停止派送</h3>
      <ActionForm
        id="admin-halt-declare"
        submitLabel="停止派送"
        pending={declare.isPending}
        error={declare.error}
        done={declare.data?.note}
        onSubmit={(note) => declare.mutate({ note, provider: target })}
      />
      <h3>恢復派送</h3>
      <ActionForm
        id="admin-halt-lift"
        submitLabel="恢復派送"
        pending={lift.isPending}
        error={lift.error}
        done={lift.isSuccess && "已解除，上面的狀態已更新。"}
        onSubmit={(note) => lift.mutate({ note, provider: target })}
      />
    </AdminPage>
  );
}

export function AdminRosters() {
  const rosters = useRosters();
  return (
    <AdminPage heading="名冊" lede="要改名冊只能改部署設定再重啟，後台不提供編輯。">
      {rosters.isPending && <Loading what="名冊" />}
      <ReadFailure error={rosters.error} what="名冊" />
      {rosters.data && (
        <>
          <h2>operator</h2>
          <ul>
            {rosters.data.operator_user_ids.map((id) => (
              <li key={id}>
                <code>{id}</code>
              </li>
            ))}
          </ul>
          <h2>封測名單</h2>
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
        </>
      )}
    </AdminPage>
  );
}

const ACTION_LABEL: Record<string, string> = {
  "skill.access_restrict": "設定受限展示",
  "skill.access_unrestrict": "解除受限展示",
  "skill.redistribution_set": "再散布判定",
  "skill.takedown": "下架",
  "credit.grant": "授予點數",
  "dispatch.halted": "停止派送",
  "dispatch.resumed": "恢復派送",
  "account.lookup": "查詢帳號",
  "credit.lookup": "查詢點數",
};

function MetadataCell({ metadata }: { metadata: Record<string, unknown> }) {
  const entries = Object.entries(metadata);
  if (entries.length === 0) return <>不適用</>;
  return (
    <details>
      <summary>{entries.length} 項</summary>
      <dl>
        {entries.map(([key, value]) => (
          <Fragment key={key}>
            <dt>{key}</dt>
            <dd>{typeof value === "string" ? value : JSON.stringify(value)}</dd>
          </Fragment>
        ))}
      </dl>
    </details>
  );
}

export function AdminAuditLog() {
  const log = useOperatorAuditLog();
  const rows = log.data?.pages.flatMap((page) => page.events) ?? [];

  return (
    <AdminPage heading="動作紀錄">
      {log.isPending && <Loading what="動作紀錄" />}
      <ReadFailure error={log.error} what="動作紀錄" />
      {log.data &&
        (rows.length === 0 ? (
          <p>operator 動作：0 筆。</p>
        ) : (
          <div className="table-scroll">
            <table>
              <caption>operator 動作，新的在上面</caption>
              <thead>
                <tr>
                  <th scope="col">時間</th>
                  <th scope="col">動作</th>
                  <th scope="col">operator</th>
                  <th scope="col">對象</th>
                  <th scope="col">內容</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((event, index) => (
                  <tr key={`${event.action}-${index}`}>
                    <td>
                      <Timestamp at={event.occurred_at} />
                    </td>
                    <td>{ACTION_LABEL[event.action] ?? event.action}</td>
                    <td>
                      <code>{event.actor_user_id ?? "未測量"}</code>
                    </td>
                    <td>
                      {event.resource_type} <code>{event.resource_id ?? "不適用"}</code>
                    </td>
                    <td>
                      <MetadataCell metadata={event.metadata} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      {log.hasNextPage && (
        <button type="button" disabled={log.isFetchingNextPage} onClick={() => log.fetchNextPage()}>
          {log.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}
    </AdminPage>
  );
}

const COST_KIND: Record<string, string> = {
  creation_step: "創作步驟",
  creation_session: "創作會話",
  search_embedding: "搜尋向量",
  index_enrich: "索引增強",
  review: "評審",
  suggestion: "改善建議",
  generate: "單次生成",
  run: "試跑",
  match_reasons: "搜尋理由",
};

export function AdminCostStatistics() {
  const stats = useCostStatistics();
  const rows = stats.data?.statistics ?? [];

  return (
    <AdminPage heading="成本統計" lede="與開始前檢查、會話估價讀的是同一組數字；不含使用者維度。">
      {stats.isPending && <Loading what="成本統計" />}
      <ReadFailure error={stats.error} what="成本統計" />
      {stats.data &&
        (rows.length === 0 ? (
          <p>統計窗：0 個。每日統計跑過之後才會有。</p>
        ) : (
          <div className="table-scroll">
            <table>
              <caption>每一種呼叫最新的統計窗（美元）</caption>
              <thead>
                <tr>
                  <th scope="col">種類</th>
                  <th scope="col">統計窗結束</th>
                  <th scope="col">樣本數</th>
                  <th scope="col">p50</th>
                  <th scope="col">p90</th>
                  <th scope="col">p95</th>
                  <th scope="col">最大</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.kind}>
                    <th scope="row">{COST_KIND[row.kind] ?? row.kind}</th>
                    <td>
                      <Timestamp at={row.window_end} />
                    </td>
                    <td>{row.sample_count}</td>
                    <td>{usd(row.p50_usd_micros)}</td>
                    <td>{usd(row.p90_usd_micros)}</td>
                    <td>{usd(row.p95_usd_micros)}</td>
                    <td>{usd(row.max_usd_micros)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
    </AdminPage>
  );
}
