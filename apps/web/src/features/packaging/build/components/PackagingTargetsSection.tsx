import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { usePackagingTargets, PackagingTargetId } from "../../packaging.service";
import { TargetOption } from "./TargetOption";

export function PackagingTargetsSection({
  targets,
  selected,
  onSelect,
}: {
  targets: ReturnType<typeof usePackagingTargets>;
  selected: PackagingTargetId | "";
  onSelect: (id: PackagingTargetId) => void;
}) {
  return (
    <>
      <h2>打包目標</h2>
      <p className="note" data-role="teaching">
        每個目標的安裝說明也隨套件內的 INSTALL.md 一起下載。
      </p>
      {targets.isPending && <Loading what="打包目標" />}
      <ReadFailure error={targets.error} what="打包目標" />
      {targets.data && (
        <ul className="packaging-targets" data-role="evidence">
          {targets.data.targets.map((t) => (
            <TargetOption
              key={t.id}
              target={t}
              selected={t.id === selected}
              onSelect={() => onSelect(t.id)}
            />
          ))}
        </ul>
      )}
    </>
  );
}
