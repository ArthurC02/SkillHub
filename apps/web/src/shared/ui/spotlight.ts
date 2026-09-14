import type { PointerEvent } from "react";

function cardsOf(e: PointerEvent<HTMLElement>) {
  return Array.from(e.currentTarget.children) as HTMLElement[];
}

export function followPointer(e: PointerEvent<HTMLElement>) {
  if (e.pointerType !== "mouse") return;
  for (const card of cardsOf(e)) {
    const box = card.getBoundingClientRect();
    card.style.setProperty("--x", `${e.clientX - box.left}px`);
    card.style.setProperty("--y", `${e.clientY - box.top}px`);
  }
}

export function releasePointer(e: PointerEvent<HTMLElement>) {
  for (const card of cardsOf(e)) {
    card.style.removeProperty("--x");
    card.style.removeProperty("--y");
  }
}
