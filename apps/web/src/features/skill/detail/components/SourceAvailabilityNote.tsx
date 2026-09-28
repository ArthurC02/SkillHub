import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { SkillSource } from "../../../../core/api/types";
import { StateIcon } from "../../../../shared/ui/StateIcon";

const AVAILABILITY_CLASS: Record<string, string> = {
  lost: "badge badge-risk",
  unreachable: "badge badge-unverified",
};

export function SourceAvailabilityNote({
  availability,
  unavailableSince,
}: {
  availability: NonNullable<SkillSource["availability"]>;
  unavailableSince: string | undefined;
}) {
  return (
    <p className={AVAILABILITY_CLASS[availability.value] ?? "note"}>
      {availability.value === "lost" && <StateIcon state="fail" />}
      {availability.label}
      {unavailableSince && (
        <>
          （自 <Timestamp at={unavailableSince} /> 起抓不到）
        </>
      )}
      ：{availability.note}
    </p>
  );
}
