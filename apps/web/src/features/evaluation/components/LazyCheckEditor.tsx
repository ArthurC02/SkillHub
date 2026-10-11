import { Suspense, lazy, type ComponentProps } from "react";
import { Loading } from "../../../shared/ui/Loading";

const CheckEditor = lazy(() => import("./CheckEditor").then((m) => ({ default: m.CheckEditor })));

export function LazyCheckEditor(props: ComponentProps<typeof CheckEditor>) {
  return (
    <Suspense fallback={<Loading what="檢查設計" />}>
      <CheckEditor {...props} />
    </Suspense>
  );
}
