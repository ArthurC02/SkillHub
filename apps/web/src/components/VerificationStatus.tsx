import type { SkillVerification, VerificationState } from "../api/types";

// DISC-002/006 AC: skills without run evidence must be clearly marked
// "尚未試跑" (not yet tested) — this is the shared label for that state.
const LABELS: Record<VerificationState, string> = {
  not_run: "尚未試跑",
  success: "試跑成功",
  failure: "試跑失敗",
  partial: "部分成功",
};

export function VerificationStatus({ verification }: { verification: SkillVerification }) {
  return (
    <span className={`badge badge-verify-${verification.status}`}>
      {LABELS[verification.status]}
      {verification.status !== "not_run" && verification.last_verified_at
        ? `（${verification.last_verified_at}）`
        : ""}
    </span>
  );
}
