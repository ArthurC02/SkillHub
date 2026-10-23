export function contractEnum<T extends string>(values: Record<T, true>): readonly string[] {
  return Object.keys(values);
}
