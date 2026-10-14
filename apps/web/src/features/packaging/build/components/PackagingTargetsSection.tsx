import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { ListFreshness } from "../../../../shared/ui/ListFreshness";
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
      <h2 id="packaging-targets-heading" tabIndex={-1}>
        打包目標
      </h2>
      <p className="note" data-role="teaching">
        每個目標的安裝說明也隨套件內的 INSTALL.md 一起下載。
      </p>
      {targets.isPending && <Loading what="打包目標" />}
      {targets.isFetching && !targets.isPending && (
        <p role="status" id="packaging-targets-refresh-status">
          正在重新讀取打包目標，暫時不能更換目標。
        </p>
      )}
      {!targets.error && targets.data && (
        <ListFreshness
          inFlight={false}
          showWhenIdle
          subject="打包目標"
          updatedAt={targets.dataUpdatedAt}
          fetching={targets.isFetching}
          refetch={async () => {
            const result = await targets.refetch();
            if (result.error) document.getElementById("packaging-targets-heading")?.focus();
          }}
        />
      )}
      <ReadFailure
        error={targets.error}
        what="打包目標"
        retry={() => void targets.refetch()}
        retrying={targets.isFetching}
        focusTarget="packaging-targets-heading"
      />
      {!targets.error && targets.data && (
        <ul className="packaging-targets" data-role="evidence">
          {targets.data.targets.map((t) => (
            <TargetOption
              key={t.id}
              target={t}
              selected={t.id === selected}
              disabled={targets.isFetching}
              onSelect={() => onSelect(t.id)}
            />
          ))}
        </ul>
      )}
    </>
  );
}
