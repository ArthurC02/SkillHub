const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

export function isText(value: unknown): value is string {
  return typeof value === "string";
}

export function isOptionalText(value: unknown): boolean {
  return value === undefined || isText(value);
}

export function isDate(value: unknown): boolean {
  return isText(value) && Number.isFinite(Date.parse(value));
}

export function isUuid(value: unknown): value is string {
  return isText(value) && UUID.test(value);
}

export function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
