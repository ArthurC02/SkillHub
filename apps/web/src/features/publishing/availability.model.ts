import { isRecord } from "../../core/api/wire";

export function isLabelled(value: unknown) {
  return (
    isRecord(value) &&
    typeof value.value === "string" &&
    typeof value.label === "string" &&
    typeof value.note === "string"
  );
}

export function isAcquisition(value: unknown) {
  return isRecord(value) && typeof value.available === "boolean" && typeof value.note === "string";
}
