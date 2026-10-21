import type { CreationSession } from "../../features/creation/creation.service";

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
