from dataclasses import dataclass


class ReviewRejected(Exception):
    pass


@dataclass(frozen=True)
class Review:
    text: str
    rating: int
    tags: tuple[str, ...]


def accept_review(text: str, rating: int, tags: list[str]) -> Review:
    if not text.strip():
        raise ReviewRejected("a review needs text")
    if len(text) > 2000:
        raise ReviewRejected("a review is limited to 2000 characters")
    if rating < 1 or rating > 5:
        raise ReviewRejected("the rating must be from 1 to 5")
    cleaned = sorted({tag.strip().lower() for tag in tags if tag.strip()})
    if len(cleaned) > 12:
        raise ReviewRejected("a review takes at most 12 tags")
    return Review(text=text, rating=rating, tags=tuple(cleaned))


def excerpt(review: Review) -> str:
    if len(review.text) <= 200:
        return review.text
    return review.text[:200].rstrip() + "..."
