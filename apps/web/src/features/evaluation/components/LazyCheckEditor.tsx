import { Suspense, lazy, useId, type ComponentProps } from "react";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { emptyNode, type CheckNode } from "../check.model";
import { useCheckKinds } from "../evaluation.service";
import { KindPicker } from "./KindPicker";

const CheckEditor = lazy(() => import("./CheckEditor").then((m) => ({ default: m.CheckEditor })));

export function LazyCheckEditor({
  value,
  onChange,
}: Omit<ComponentProps<typeof CheckEditor>, "value" | "kinds"> & { value: CheckNode | null }) {
  const kinds = useCheckKinds();
  const id = useId();
  return (
    <div id={id} tabIndex={-1} role="group" aria-label="檢查方式與設計">
      {kinds.isPending && <Loading what="檢查方式" paused={kinds.fetchStatus === "paused"} />}
      <ReadFailure
        error={kinds.error}
        what="檢查方式"
        retry={() => void kinds.refetch()}
        retrying={kinds.isFetching}
        focusTarget={id}
      />
      {!kinds.error &&
        kinds.data &&
        (value ? (
          <Suspense fallback={<Loading what="檢查設計" />}>
            <CheckEditor value={value} onChange={onChange} kinds={kinds.data} />
          </Suspense>
        ) : (
          <KindPicker
            id="check-editor-root"
            label="從哪一種節點開始"
            kinds={kinds.data.kinds}
            action="開始設計"
            onPick={(kind) => onChange(emptyNode(kind))}
          />
        ))}
    </div>
  );
}
