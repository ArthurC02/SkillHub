export function focusFeedback(
  feedback: HTMLElement | null,
  ...additionalTargets: (Element | null)[]
) {
  if (!feedback) return;
  const active = document.activeElement;
  if (
    active === document.body ||
    active?.id === "main-content" ||
    additionalTargets.some((target) => target !== null && active === target) ||
    feedback.parentElement?.contains(active)
  )
    feedback.focus();
}
