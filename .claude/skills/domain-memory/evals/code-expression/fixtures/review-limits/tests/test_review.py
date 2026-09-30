import unittest

from reviews.review import ReviewRejected, accept_review, excerpt


class ReviewTest(unittest.TestCase):
    def test_a_review_keeps_its_tags_once_each_in_lower_case(self) -> None:
        review = accept_review("Good.", 4, ["Crime", "crime ", "noir"])
        self.assertEqual(review.tags, ("crime", "noir"))

    def test_a_rating_above_the_scale_is_rejected(self) -> None:
        with self.assertRaises(ReviewRejected):
            accept_review("Good.", 6, [])

    def test_empty_text_is_rejected(self) -> None:
        with self.assertRaises(ReviewRejected):
            accept_review("  ", 3, [])

    def test_a_short_review_is_its_own_excerpt(self) -> None:
        review = accept_review("Short.", 3, [])
        self.assertEqual(excerpt(review), "Short.")


if __name__ == "__main__":
    unittest.main()
