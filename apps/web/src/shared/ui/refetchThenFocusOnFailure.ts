export function refetchThenFocusOnFailure(
  refetch: () => Promise<{ error: unknown }>,
  headingId: string,
) {
  return async () => {
    const result = await refetch();
    if (result.error) document.getElementById(headingId)?.focus();
  };
}
