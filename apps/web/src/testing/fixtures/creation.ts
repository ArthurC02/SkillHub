import type { CreationSession } from "../../features/creation/creation.service";

export const CREATION_LIMITS = {
  min_budget_credits: 130,
  max_budget_credits: 6500,
  max_steps: 20,
  max_tool_calls: 10,
  call_timeout_seconds: 120,
  session_timeout_seconds: 3600,
  retention_seconds: 604800,
};

export function creationSnapshot(brief: string): CreationSession["snapshot"] {
  return {
    messages: [],
    brief,
    brief_confirmed: false,
    acceptance_criteria: [],
    diagram_understanding: "",
    diagram_confirmed: false,
    references: [],
    pending_action: "",
    budget_credits: 500,
    reserved_credits: 0,
    usage_unknown: false,
    steps: 1,
    tool_calls: 0,
  };
}
