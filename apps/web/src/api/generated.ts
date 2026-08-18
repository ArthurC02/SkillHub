import { Configuration, DefaultApi } from "@skillhub/api-client-ts";

import { API_BASE_URL } from "./client";

// Incremental adoption boundary: existing endpoint wrappers remain authoritative
// until each one has an explicit DTO-to-view-model adapter and behavior test.
export const generatedApi = new DefaultApi(
  new Configuration({
    basePath: API_BASE_URL,
    credentials: "include",
  }),
);
