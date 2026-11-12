export const OFFSET_PAGE_SIZE = 50;

export const OFFSET_PAGE_REQUEST = String(OFFSET_PAGE_SIZE + 1);

export function offsetPage<K extends string, T>(
  key: K,
  fetched: T[],
  offset: number,
): Record<K, T[]> & { nextOffset: number | undefined } {
  const items = fetched.slice(0, OFFSET_PAGE_SIZE);
  const nextOffset = fetched.length > OFFSET_PAGE_SIZE ? offset + OFFSET_PAGE_SIZE : undefined;
  return { [key]: items, nextOffset } as Record<K, T[]> & { nextOffset: number | undefined };
}
