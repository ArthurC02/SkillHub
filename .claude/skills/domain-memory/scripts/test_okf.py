from __future__ import annotations

import unittest

from domain_registry.okf import decode, dump_yaml, encode, load_yaml, split_concept

CITED = {"path": "src/orders.py", "lines": {"start": 3, "end": 9}, "excerpt_sha256": "sha256:" + "a" * 64}


class ConceptRoundTripTest(unittest.TestCase):
    def assert_round_trip(self, asset: str, record: dict) -> str:
        text = encode(asset, record)
        self.assertEqual(record, decode(asset, record["id"], text))
        return text

    def test_a_context_is_a_titled_concept_with_its_responsibility_as_prose(self) -> None:
        text = self.assert_round_trip("contexts", {
            "id": "orders", "name": "Orders", "responsibility": "Own the order lifecycle.",
            "subdomain": "core", "status": "reviewed", "evidence": [CITED],
        })
        self.assertEqual(
            "---\ntype: Bounded Context\ntitle: Orders\nstatus: stable\nsubdomain: core\nsources:\n"
            '  - resource: "src/orders.py#L3-9"\n    excerpt_sha256: "sha256:' + "a" * 64 + '"\n---\n'
            "Own the order lifecycle.\n",
            text,
        )

    def test_relations_are_typed_markdown_links_and_string_lists_are_sections(self) -> None:
        text = self.assert_round_trip("rules", {
            "id": "total", "contexts": ["orders", "billing"], "statement": "A total is never negative.",
            "examples": ["0 is accepted", "-1 is refused"], "status": "candidate",
        })
        self.assertIn(
            "A total is never negative.\n\n# Relations\n- Contexts: [orders](/contexts/orders)\n"
            "- Contexts: [billing](/contexts/billing)\n\n# Examples\n- 0 is accepted\n- -1 is refused\n",
            text,
        )
        self.assertIn("status: draft\n", text)

    def test_each_single_relation_names_its_target_asset(self) -> None:
        text = self.assert_round_trip("interactions", {
            "id": "orders-billing", "producer_context": "orders", "consumer_context": "billing",
            "contract_id": "orders-v1", "consistency": "synchronous",
        })
        self.assertIn("- Producer context: [orders](/contexts/orders)\n", text)
        self.assertIn("- Contract id: [orders-v1](/contracts/orders-v1)\n", text)

    def test_a_rule_and_an_event_link_the_aggregate_and_the_contract_they_name(self) -> None:
        rule = self.assert_round_trip("rules", {
            "id": "total", "contexts": ["orders"], "aggregate": "order", "statement": "A total is never negative.",
        })
        event = self.assert_round_trip("events", {
            "id": "order-placed", "owner_context": "orders", "aggregate": "order", "contract_id": "orders-v1",
            "meaning": "An order was placed.",
        })
        self.assertIn("- Aggregate: [order](/aggregates/order)\n", rule)
        self.assertIn("- Aggregate: [order](/aggregates/order)\n- Contract id: [orders-v1](/contracts/orders-v1)\n", event)

    def test_what_markdown_cannot_carry_stays_in_frontmatter(self) -> None:
        cases = [
            ("relation id with a space", "interactions", {"id": "i", "producer_context": "two words"}),
            ("relation that is not a string", "interactions", {"id": "i", "consumer_context": 7}),
            ("empty list relation", "rules", {"id": "r", "contexts": []}),
            ("list relation given as text", "rules", {"id": "r", "contexts": "orders"}),
            ("prose with a heading line", "rules", {"id": "r", "statement": "First.\n# Not a heading"}),
            ("prose with surrounding space", "rules", {"id": "r", "statement": " padded "}),
            ("blank prose", "rules", {"id": "r", "statement": ""}),
            ("list item with a line break", "rules", {"id": "r", "examples": ["one\ntwo"]}),
            ("empty list", "contexts", {"id": "c", "requirement_prefixes": []}),
            ("evidence without lines", "rules", {"id": "r", "evidence": [{"path": "a.py"}]}),
            ("evidence lines that are not integers", "rules",
             {"id": "r", "evidence": [{**CITED, "lines": {"start": "1", "end": 2}}]}),
            ("name with a line break", "contexts", {"id": "c", "name": "Two\nlines"}),
            ("nested object", "contracts", {"id": "k", "idempotency": {"key": "origin key", "scope": "per Workspace"}}),
            ("decimal number", "rules", {"id": "r", "weight": 1.5}),
            ("number in exponent form", "rules", {"id": "r", "weight": 2.5e-07}),
            ("whole number", "rules", {"id": "r", "weight": 2}),
            ("text that looks like a number", "rules", {"id": "r", "weight": "1.5"}),
        ]
        for label, asset, record in cases:
            with self.subTest(label):
                self.assert_round_trip(asset, record)

    def test_each_registry_status_maps_to_an_okf_status(self) -> None:
        cases = [
            ("candidate", "status: draft\n", None),
            ("reviewed", "status: stable\n", None),
            ("deprecated", "status: deprecated\n", None),
            ("superseded", "status: deprecated\n", "registry_status: superseded\n"),
            ("bogus", None, "registry_status: bogus\n"),
        ]
        for status, okf_line, kept in cases:
            with self.subTest(status):
                text = self.assert_round_trip("rules", {"id": "r", "status": status})
                if okf_line:
                    self.assertIn(okf_line, text)
                else:
                    self.assertNotIn("\nstatus:", text)
                if kept:
                    self.assertIn(kept, text)

    def test_a_decision_keeps_its_own_status_apart_from_its_review_status(self) -> None:
        text = self.assert_round_trip("decisions", {
            "id": "d", "status": "accepted", "review_status": "reviewed", "statement": "Use one store.",
            "supersedes": ["older"],
        })
        self.assertIn("status: stable\ndecision_status: accepted\n", text)
        self.assertIn("- Supersedes: [older](/decisions/older)\n", text)

    def test_a_record_without_a_status_writes_none(self) -> None:
        text = self.assert_round_trip("rules", {"id": "r", "statement": "S."})
        self.assertNotIn("status", text)

    def test_a_field_that_collides_with_a_concept_key_is_refused(self) -> None:
        for field in ("type", "title", "sources", "registry_status", "decision_status"):
            with self.subTest(field), self.assertRaisesRegex(ValueError, f"has a field named {field}, which the concept format reserves"):
                encode("rules", {"id": "r", field: "x"})

    def test_a_cited_path_holding_a_hash_reads_back_whole(self) -> None:
        self.assert_round_trip("rules", {"id": "r", "evidence": [{**CITED, "path": "docs/a#L1-2.md"}]})

    def test_a_context_title_needs_a_name_that_fits_on_one_line(self) -> None:
        text = self.assert_round_trip("contexts", {"id": "c", "name": "Two\nlines"})
        self.assertNotIn("title:", text)


class YamlSubsetTest(unittest.TestCase):
    def test_each_scalar_reads_back_as_written(self) -> None:
        values = {
            "plain": "composition-root fact reader",
            "reserved_word": "yes",
            "reserved_upper": "True",
            "number_like": "123",
            "colon": "human:ArthurC",
            "leading_digit": "2026-10-09T16:39:59Z",
            "trailing_space": "ends ",
            "empty": "",
            "non_ascii": "訂單",
            "quote": 'say "hi"',
            "line_break": "one\ntwo",
            "hash": "a # b",
            "integer": 7,
            "negative": -3,
            "flag": False,
            "nothing": None,
            "empty_list": [],
            "empty_map": {},
        }
        lines = dump_yaml(values)
        self.assertEqual(values, load_yaml(lines))
        self.assertIn("plain: composition-root fact reader", lines)
        for key in ("reserved_word", "reserved_upper", "number_like", "colon", "leading_digit", "trailing_space", "empty"):
            with self.subTest(key):
                self.assertTrue(next(line for line in lines if line.startswith(key + ":")).endswith('"'))

    def test_nested_sequences_and_mappings_read_back_as_written(self) -> None:
        value = {
            "approvals": [{"role": "developer", "at": "2026-01-01T00:00:00Z", "scope": {"kind": "all"}}],
            "matrix": [["a", "b"], [], ["c"]],
            "map_of_lists": {"one": ["x"], "two": []},
        }
        self.assertEqual(value, load_yaml(dump_yaml(value)))

    def test_a_sequence_may_sit_at_its_key_indentation(self) -> None:
        self.assertEqual({"tags": ["a", "b"], "type": "T"}, load_yaml(["tags:", "- a", "- b", "type: T"]))

    def test_comments_and_blank_lines_are_skipped(self) -> None:
        self.assertEqual({"type": "T"}, load_yaml(["# a note", "", "type: T"]))

    def test_each_unreadable_frontmatter_names_its_line(self) -> None:
        cases = [
            ("single quotes", ["type: 'T'"], "line 2: write this value plainly or in double quotes: 'T'"),
            ("flow list", ["tags: [a, b]"], "line 2: write this value plainly or in double quotes: [a, b]"),
            ("no colon", ["type T"], "line 2: expected 'key: value', got type T"),
            ("no space after colon", ["type:T"], "line 2: expected 'key: value', got type:T"),
            ("repeated key", ["type: T", "type: U"], "line 3: type appears twice"),
            ("deeper line after a scalar", ["type: T", "  extra: x"], "line 3: unexpected indentation"),
            ("broken quoted text", ['type: "T'], "line 2: unreadable quoted text"),
        ]
        for label, lines, message in cases:
            with self.subTest(label), self.assertRaisesRegex(ValueError, "frontmatter " + message.replace("[", r"\[").replace("]", r"\]")):
                load_yaml(lines)

    def test_a_key_the_subset_cannot_write_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "cannot write the key 'two words'"):
            dump_yaml({"two words": 1})


class ConceptDocumentTest(unittest.TestCase):
    def test_each_malformed_concept_document_is_named(self) -> None:
        cases = [
            ("no frontmatter", "type: T\n", "starts with a --- frontmatter line"),
            ("unclosed frontmatter", "---\ntype: T\n", "has no closing --- line"),
            ("no type", "---\ntitle: T\n---\n", "needs a non-empty type"),
            ("blank type", '---\ntype: ""\n---\n', "needs a non-empty type"),
        ]
        for label, text, message in cases:
            with self.subTest(label), self.assertRaisesRegex(ValueError, message):
                split_concept(text)

    def test_windows_line_endings_read_like_unix_ones(self) -> None:
        self.assertEqual(({"type": "T"}, "Body."), split_concept("---\r\ntype: T\r\n---\r\nBody."))

    def test_a_relations_line_that_is_not_a_link_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "a Relations line must read"):
            decode("rules", "r", "---\ntype: Business Rule\n---\n# Relations\n- Contexts: orders\n")

    def test_a_number_that_is_not_finite_is_refused(self) -> None:
        for value in (float("inf"), float("nan")):
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, "cannot write a number that is not finite"):
                encode("rules", {"id": "r", "weight": value})

    def test_free_text_under_a_section_is_refused_instead_of_dropped(self) -> None:
        with self.assertRaisesRegex(ValueError, "a line under Examples must start with '- ': free text a human added"):
            decode("rules", "r", "---\ntype: Business Rule\n---\n# Examples\n- one\nfree text a human added\n")

    def test_prose_on_an_asset_without_a_prose_field_is_refused(self) -> None:
        with self.assertRaisesRegex(ValueError, "an? interactions concept has no prose field"):
            decode("interactions", "i", "---\ntype: Interaction\n---\nSome text.\n")


if __name__ == "__main__":
    unittest.main()
