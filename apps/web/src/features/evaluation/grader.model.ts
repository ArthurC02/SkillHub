import type { GraderScope } from "./evaluation.service";

export const SCOPE_LABEL: Record<GraderScope, string> = {
  platform: "平台通用",
  workspace: "這個 Workspace 專用",
};
