import { Suspense, lazy, type ComponentProps } from "react";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { emptyNode, type CheckNode } from "../check.model";
import { useCheckKinds } from "../evaluation.service";
import { KindPicker } from "./KindPicker";

const CheckEditor = lazy(() => import("./CheckEditor").then((m) => ({ default: m.CheckEditor })));

function CheckStart({ onChange }: { onChange: (next: CheckNode) => void }) {
  const kinds = useCheckKinds();
  if (kinds.isPending) return <Loading what="檢查方式" />;
  if (!kinds.data) return <ReadFailure error={kinds.error} what="檢查方式" />;
  return (
    <KindPicker
      id="check-editor-root"
      label="從哪一種節點開始"
      kinds={kinds.data.kinds}
      action="開始設計"
      onPick={(kind) => onChange(emptyNode(kind))}
    />
  );
}

export function LazyCheckEditor({
  value,
  onChange,
}: Omit<ComponentProps<typeof CheckEditor>, "value"> & { value: CheckNode | null }) {
  if (!value) return <CheckStart onChange={onChange} />;
  return (
    <Suspense fallback={<Loading what="檢查設計" />}>
      <CheckEditor value={value} onChange={onChange} />
    </Suspense>
  );
}
