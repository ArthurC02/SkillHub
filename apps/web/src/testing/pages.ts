import type { AsyncRouteComponent } from "@tanstack/react-router";
import { router } from "../app/router";

type LazyPage = Partial<Pick<AsyncRouteComponent<unknown>, "preload">>;

export async function preloadEveryPage() {
  const loads = Object.values(router.routesById).flatMap((route) => {
    const preload = (route.options.component as LazyPage | undefined)?.preload;
    return preload ? [preload()] : [];
  });
  if (loads.length === 0) throw new Error("no route component offers preload()");
  await Promise.all(loads);
}
