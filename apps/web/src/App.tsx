import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { queryClient } from "./api/queryClient";
import { router as defaultRouter } from "./router";

export default function App({ router = defaultRouter }: { router?: typeof defaultRouter } = {}) {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
