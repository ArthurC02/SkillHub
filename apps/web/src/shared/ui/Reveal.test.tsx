import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { Reveal } from "./Reveal";

test("text without hidden characters is rendered as it is", () => {
  expect(renderToStaticMarkup(<Reveal text="plain 文字" />)).toBe("plain 文字");
});

test("a right-to-left override is named where it sits instead of vanishing", () => {
  expect(renderToStaticMarkup(<Reveal text={"abc‮def"} />)).toBe(
    'abc<mark class="hidden-char">隱藏字元 U+202E</mark>def',
  );
});

test("adjacent hidden characters share one mark", () => {
  expect(renderToStaticMarkup(<Reveal text={"a​⁦b"} />)).toBe(
    'a<mark class="hidden-char">隱藏字元 U+200B U+2066</mark>b',
  );
});

test("joiners that belong to ordinary text are not flagged", () => {
  expect(renderToStaticMarkup(<Reveal text={"👩‍💻 a‌b"} />)).toBe("👩‍💻 a‌b");
});
