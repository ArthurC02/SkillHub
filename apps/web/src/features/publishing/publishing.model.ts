import { ApiError } from "../../core/api/client";

export type PublishingRefusalReason =
  | "no_publisher"
  | "already_registered"
  | "name_taken"
  | "name_is_permanent"
  | "name_shape"
  | "name_reserved"
  | "license_hold"
  | "not_redistributable"
  | "license_unknown"
  | "validation_blocked"
  | "rights_not_attested";

export const PUBLISHING_REFUSAL_LABEL: Record<PublishingRefusalReason, string> = {
  no_publisher: "這個帳號還沒有註冊發佈者名稱，要先到帳號頁註冊一個才能發佈。",
  already_registered: "這個帳號已經有一個發佈者名稱了，一個帳號只能有一個，不能再註冊第二個。",
  name_taken: "這個名稱已經被別人取走了，換一個名稱再試一次。",
  name_is_permanent: "這個 Skill 已經用另一個名稱發佈過，名稱一經建立就不能更改。",
  name_shape:
    "名稱格式不對：1～64 字元，只能是小寫英文字母、數字與連字號，開頭與結尾要是英數字，不能有連續兩個連字號。",
  name_reserved: "這個名稱是保留字，用來避免冒充平台或上游供應商，換一個名稱再試一次。",
  license_hold: "這個 Skill 的內容因授權問題尚未釐清而被保留，所以不能發佈。",
  not_redistributable: "這個 Skill 的授權不允許再散布，所以不能發佈。",
  license_unknown: "沒有人確認過這個 Skill 可不可以再散布，未確認的授權視同不允許，所以不能發佈。",
  validation_blocked: "這一版的套件沒有通過規格驗證，所以不能發佈。",
  rights_not_attested:
    "這份內容是你自己帶進來的，或是平台依你的描述寫出來的；發佈之前要先聲明你有權散布它。",
};

export const PUBLISHER_NAME_RULE =
  "名稱規則：1～64 字元，只能是小寫英文字母、數字與連字號（-），開頭與結尾必須是英數字，不能有連續兩個連字號。";

export const PUBLISHER_NAME_PERMANENT = "註冊之後這個名稱就是永久的：沒有改名的功能。";

export function refusalReason(error: unknown): PublishingRefusalReason | undefined {
  if (!(error instanceof ApiError) || typeof error.body !== "object" || error.body === null) {
    return undefined;
  }
  const reason = (error.body as { reason?: unknown }).reason;
  return typeof reason === "string" && reason in PUBLISHING_REFUSAL_LABEL
    ? (reason as PublishingRefusalReason)
    : undefined;
}

export function refusalSentence(error: unknown): string | undefined {
  const reason = refusalReason(error);
  return reason ? PUBLISHING_REFUSAL_LABEL[reason] : undefined;
}

const SERVER_WORDED_STATUSES = new Set([403, 409, 422]);

// Bundle and acquisition refusals carry reasons this table does not list
// (they are already Chinese from the server); a 500 here is not, so only the
// statuses that are always a deliberate refusal fall back to error.message.
export function actionFailureSentence(error: unknown, fallback: string): string {
  const sentence = refusalSentence(error);
  if (sentence) return sentence;
  if (error instanceof ApiError && SERVER_WORDED_STATUSES.has(error.status) && error.message) {
    return error.message;
  }
  return fallback;
}
