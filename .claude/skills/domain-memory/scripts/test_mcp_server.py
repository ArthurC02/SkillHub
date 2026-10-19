from __future__ import annotations

import argparse
import contextlib
import io
import json
import os
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

from domain_registry.cli_commands import run_command
from domain_registry.commands.read import handle_mcp_serve
from domain_registry.mcp_server import READ_ONLY, McpServer, Parameter, ServerRoots, Tool, serve
from test_support import RegistryTestCase

PLUGIN_VERSION = json.loads((Path(__file__).parents[1] / ".claude-plugin" / "plugin.json").read_text(encoding="utf-8"))["version"]
EVERY_VERSION = ["2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"]
MODERN_META = {"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": {}}
EXPOSED_TOOLS = {
    "get_context", "get_record", "resolve_terms", "lookup", "dependents", "coverage", "brief", "boundary_impact",
    "verify_evidence", "validate", "readiness", "list_proposals", "validate_change_package",
}
MUTATING_COMMANDS = (
    "apply-approved-updates", "upsert-candidate", "record-approval", "ship-package", "harvest", "recite",
    "relocate-evidence", "retract-candidate", "promote-candidate", "submit-proposal", "record-test-result",
    "init-change-package", "init-domain-memory", "amend-policy", "counterfactual", "harvest-candidates",
)


def request(identifier: object, method: str, **params: object) -> str:
    return json.dumps({"jsonrpc": "2.0", "id": identifier, "method": method, "params": params}, ensure_ascii=False)


def raising_command(_words: list[str]) -> int:
    raise RuntimeError("broken handler")


class McpServerTest(RegistryTestCase):
    def roots(self) -> tuple[str, str]:
        return str((self.repo / "memory").resolve()), str(self.repo.resolve())

    def exchange(self, *lines: str, run=run_command) -> list[dict]:
        registry, repo = self.roots()
        replies = io.StringIO()
        serve(ServerRoots(Path(registry), Path(repo)), run, io.StringIO("".join(f"{line}\n" for line in lines)), replies)
        return [json.loads(reply) for reply in replies.getvalue().splitlines()]

    def call(self, name: str, arguments: object) -> dict:
        [reply] = self.exchange(request(1, "tools/call", name=name, arguments=arguments))
        return reply["result"]

    def cli(self, command: str, *arguments: str, repo: bool = False) -> str:
        registry, repo_root = self.roots()
        roots = ("--registry-root", registry, *(("--repo-root", repo_root) if repo else ()))
        result = self.run_cli(command, *roots, *arguments)
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        return result.stdout


class HandshakeTest(McpServerTest):
    def test_initialize_answers_the_legacy_version_asked_for_with_the_tools_capability(self) -> None:
        [reply] = self.exchange(request(1, "initialize", protocolVersion="2025-06-18", capabilities={}))
        self.assertEqual(
            {"jsonrpc": "2.0", "id": 1, "result": {
                "protocolVersion": "2025-06-18", "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "domain-memory", "version": PLUGIN_VERSION},
            }},
            reply,
        )

    def test_initialize_with_a_version_it_does_not_speak_there_answers_the_newest_legacy_version(self) -> None:
        for asked in ("1999-01-01", "2026-07-28"):
            with self.subTest(asked=asked):
                [reply] = self.exchange(request(1, "initialize", protocolVersion=asked, capabilities={}))
                self.assertEqual("2025-11-25", reply["result"]["protocolVersion"])

    def test_server_discover_lists_every_version_it_serves_and_names_itself_in_meta(self) -> None:
        [reply] = self.exchange(request("d", "server/discover", _meta=MODERN_META))
        self.assertEqual(
            {"resultType": "complete", "supportedVersions": EVERY_VERSION, "capabilities": {"tools": {"listChanged": False}},
             "_meta": {"io.modelcontextprotocol/serverInfo": {"name": "domain-memory", "version": PLUGIN_VERSION}},
             "ttlMs": 300000, "cacheScope": "public"},
            reply["result"],
        )

    def test_a_host_that_discovers_then_lists_without_initialize_gets_the_cache_hints_it_requires(self) -> None:
        replies = self.exchange(
            request(0, "server/discover", _meta=MODERN_META), request(1, "tools/list", _meta=MODERN_META),
            request(2, "tools/list"), request(3, "ping", _meta=MODERN_META),
        )
        self.assertEqual(
            [(0, 300000, "public", "complete"), (1, 300000, "public", "complete"), (2, None, None, None), (3, None, None, "complete")],
            [(reply["id"], reply["result"].get("ttlMs"), reply["result"].get("cacheScope"), reply["result"].get("resultType"))
             for reply in replies],
        )
        self.assertEqual(13, len(replies[1]["result"]["tools"]))

    def test_a_modern_request_with_an_unsupported_or_incomplete_meta_is_refused(self) -> None:
        cases = [
            ("unsupported version", request(1, "tools/list", _meta={**MODERN_META, "io.modelcontextprotocol/protocolVersion": "1900-01-01"}),
             {"code": -32022, "message": "Unsupported protocol version",
              "data": {"supported": EVERY_VERSION, "requested": "1900-01-01"}}),
            ("no client capabilities", request(1, "tools/list", _meta={"io.modelcontextprotocol/protocolVersion": "2026-07-28"}),
             {"code": -32602, "message": "a request must carry _meta io.modelcontextprotocol/protocolVersion and "
                                         "io.modelcontextprotocol/clientCapabilities"}),
            ("discover without meta", request(1, "server/discover"),
             {"code": -32602, "message": "a request must carry _meta io.modelcontextprotocol/protocolVersion and "
                                         "io.modelcontextprotocol/clientCapabilities"}),
        ]
        for name, line, error in cases:
            with self.subTest(name):
                self.assertEqual([{"jsonrpc": "2.0", "id": 1, "error": error}], self.exchange(line))

    def test_notifications_get_no_reply_and_ping_gets_an_empty_result(self) -> None:
        self.assertEqual(
            [{"jsonrpc": "2.0", "id": 7, "result": {}}],
            self.exchange(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}), request(7, "ping")),
        )

    def test_a_result_carries_result_type_only_when_the_request_declared_its_version(self) -> None:
        replies = self.exchange(
            request(1, "initialize", protocolVersion="2025-06-18", capabilities={}), request(2, "tools/list"),
            request(3, "ping"), request(4, "tools/list", _meta=MODERN_META), request(5, "ping", _meta=MODERN_META),
        )
        self.assertEqual(
            [(1, False), (2, False), (3, False), (4, True), (5, True)],
            [(reply["id"], "resultType" in reply["result"]) for reply in replies],
        )

    def test_requests_before_initialize_and_a_repeated_initialize_are_answered(self) -> None:
        replies = self.exchange(
            request(1, "tools/list"), request(2, "initialize", protocolVersion="2025-06-18", capabilities={}),
            request(3, "initialize", protocolVersion="2025-03-26", capabilities={}),
        )
        self.assertEqual(
            [(1, 13), (2, "2025-06-18"), (3, "2025-03-26")],
            [(reply["id"], len(reply["result"]["tools"]) if reply["id"] == 1 else reply["result"]["protocolVersion"])
             for reply in replies],
        )


class ToolListTest(McpServerTest):
    def test_tools_list_offers_exactly_the_read_only_commands_and_no_mutating_one(self) -> None:
        for name, meta in (("legacy", {}), ("modern", {"_meta": MODERN_META})):
            with self.subTest(name):
                [reply] = self.exchange(request(1, "tools/list", **meta))
                names = {tool["name"] for tool in reply["result"]["tools"]}
                self.assertEqual(EXPOSED_TOOLS, names)
                self.assertEqual(set(), {name.replace("_", "-") for name in names} - set(READ_ONLY))
                self.assertEqual(set(), names & {command.replace("-", "_") for command in MUTATING_COMMANDS})

    def test_each_tool_declares_its_parameters_as_a_closed_json_schema(self) -> None:
        [reply] = self.exchange(request(1, "tools/list"))
        schemas = {tool["name"]: tool["inputSchema"] for tool in reply["result"]["tools"]}
        self.assertEqual(
            ({"type": "object", "properties": {"query": {"type": "array", "items": {"type": "string"}},
                                               "context": {"type": "string"}, "with_mentions": {"type": "boolean"}},
              "required": ["query"], "additionalProperties": False},
             {"type": "string", "description": "a path inside the repository, relative to its root"},
             "rules", ["base"], {"type": "object", "properties": {}, "required": [], "additionalProperties": False}),
            (schemas["resolve_terms"], schemas["list_proposals"]["properties"]["changes_root"],
             next(asset for asset in schemas["get_record"]["properties"]["asset"]["enum"] if asset == "rules"),
             schemas["boundary_impact"]["required"], schemas["coverage"]),
        )

    def test_tools_list_names_only_the_tools_key_and_an_object_schema_only_where_output_is_always_an_object(self) -> None:
        [reply] = self.exchange(request(1, "tools/list"))
        tools = {tool["name"]: tool for tool in reply["result"]["tools"]}
        self.assertEqual({"tools"}, set(reply["result"]))
        self.assertEqual(
            {"name": "coverage",
             "description": "Report modelling gaps: Contexts without records, unproven rules, records without evidence.",
             "inputSchema": {"type": "object", "properties": {}, "required": [], "additionalProperties": False},
             "annotations": {"readOnlyHint": True, "destructiveHint": False, "openWorldHint": False},
             "outputSchema": {"type": "object",
                              "description": "The JSON object coverage prints, as references/script-api.md describes it."}},
            tools["coverage"],
        )
        self.assertEqual(
            {"get_context", "get_record", "dependents", "coverage", "boundary_impact", "verify_evidence", "readiness"},
            {name for name, tool in tools.items() if "outputSchema" in tool},
        )


class ToolCallTest(McpServerTest):
    def test_get_context_returns_exactly_what_the_command_prints_and_the_same_object_structured(self) -> None:
        self.two_mapped_contexts()
        printed = self.cli("get-context", "--id", "orders")
        self.assertEqual("orders", json.loads(printed)["context"]["id"])
        self.assertEqual(
            {"content": [{"type": "text", "text": printed}], "isError": False,
             "structuredContent": json.loads(printed)},
            self.call("get_context", {"id": "orders"}),
        )

    def test_brief_with_markdown_returns_the_markdown_the_command_prints_and_without_it_json(self) -> None:
        self.two_mapped_contexts()
        self.change("src/orders/port.py")
        markdown = self.cli("brief", "--base", "HEAD", "--markdown", repo=True)
        result = self.call("brief", {"base": "HEAD", "markdown": True})
        self.assertEqual((markdown, False, None),
                         (result["content"][0]["text"], result["isError"], result.get("structuredContent")))
        self.assertFalse(markdown.startswith("{"))
        plain = self.call("brief", {"base": "HEAD", "markdown": False})
        self.assertEqual("no-cross-context-change", json.loads(plain["content"][0]["text"])["verdict"])
        self.assertEqual(json.loads(plain["content"][0]["text"]), plain["structuredContent"])

    def test_resolve_terms_passes_every_query_in_order(self) -> None:
        self.two_contexts()
        self.seed("vocabulary.json", [
            {"id": "order", "name": "Order", "contexts": ["orders"]},
            {"id": "invoice", "name": "Invoice", "contexts": ["billing"]},
        ])
        printed = self.cli("resolve-terms", "--query", "Order", "--query", "Invoice")
        result = self.call("resolve_terms", {"query": ["Order", "Invoice"]})
        self.assertEqual((printed, ["Order", "Invoice"]),
                         (result["content"][0]["text"], [match["query"] for match in json.loads(printed)]))

    def test_a_note_the_command_writes_on_stderr_comes_back_as_a_second_text(self) -> None:
        result = self.call("lookup", {"asset": "rules", "query": "zzz"})
        self.assertEqual(("[]\n", False, None),
                         (result["content"][0]["text"], result["isError"], result.get("structuredContent")))
        self.assertTrue(result["content"][1]["text"].startswith('NOTE: no rules record mentions "zzz"'), result["content"])

    def test_list_proposals_reads_a_directory_given_relative_to_the_repository(self) -> None:
        self.draft_package()
        result = self.call("list_proposals", {"changes_root": "."})
        self.assertEqual((["PRO-1"], False), ([row["proposal_id"] for row in json.loads(result["content"][0]["text"])], result["isError"]))


class ToolErrorTest(McpServerTest):
    def test_a_refusal_inside_a_tool_is_a_tool_error_carrying_its_message(self) -> None:
        self.assertEqual(
            {"content": [{"type": "text", "text": "ERROR: context not found: nope\n"}], "isError": True},
            self.call("get_context", {"id": "nope"}),
        )

    def test_a_command_that_exits_non_zero_without_raising_is_a_tool_error(self) -> None:
        self.add_context()
        result = self.call("validate", {"require_reviewed": True})
        self.assertEqual((True, "ERROR: "), (result["isError"], result["content"][0]["text"][:7]))

    def test_arguments_the_command_cannot_take_are_tool_errors(self) -> None:
        cases = [
            ("a root from the client", "get_context", {"id": "orders", "registry_root": "/elsewhere"},
             "ERROR: get_context takes no argument registry_root; it takes id, summary\n"),
            ("a number for a string", "get_context", {"id": 5}, "ERROR: argument id takes a string\n"),
            ("a number among strings", "resolve_terms", {"query": ["Order", 1]}, "ERROR: argument query takes a list of strings\n"),
            ("a path outside the repository", "list_proposals", {"changes_root": "../outside"},
             f"ERROR: argument changes_root must lie inside the repository {self.repo.resolve()}: ../outside\n"),
        ]
        for name, tool, arguments, message in cases:
            with self.subTest(name):
                self.assertEqual({"content": [{"type": "text", "text": message}], "isError": True},
                                 self.call(tool, arguments))

    def test_a_read_the_file_system_refuses_is_a_tool_error(self) -> None:
        result = self.call("list_proposals", {"changes_root": "absent"})
        self.assertEqual((True, "ERROR: "), (result["isError"], result["content"][0]["text"][:7]))

    def test_missing_required_arguments_are_tool_errors_naming_them_as_json_arguments(self) -> None:
        cases = [
            ("one", "get_context", {}, "ERROR: get_context: argument id is required\n"),
            ("two, sorted", "get_record", {}, "ERROR: get_record: arguments asset, id are required\n"),
            ("one of two", "lookup", {"query": "order"}, "ERROR: lookup: argument asset is required\n"),
        ]
        for name, tool, arguments, message in cases:
            with self.subTest(name):
                self.assertEqual({"content": [{"type": "text", "text": message}], "isError": True},
                                 self.call(tool, arguments))

    def test_missing_required_arguments_are_named_in_alphabetical_order_not_declaration_order(self) -> None:
        declared_backwards = Tool("probe", "Probe.", (), (Parameter("zone", required=True), Parameter("area", required=True)))
        server = McpServer(ServerRoots(self.repo, self.repo), run_command)
        with self.assertRaisesRegex(ValueError, r"^probe: arguments area, zone are required$"):
            server.command_line(declared_backwards, {})

    def test_a_failing_command_that_prints_a_json_object_returns_no_structured_content(self) -> None:
        self.seed_two_cited_rules()
        result = self.call("verify_evidence", {"strict": True})
        self.assertEqual((True, 1, None),
                         (result["isError"], json.loads(result["content"][0]["text"])["summary"]["shifted"],
                          result.get("structuredContent")))

    def test_unknown_tools_methods_and_non_object_arguments_are_json_rpc_errors(self) -> None:
        cases = [
            ("unknown tool", request(2, "tools/call", name="upsert_candidate", arguments={}),
             {"code": -32602, "message": "Unknown tool: upsert_candidate"}),
            ("unknown method", request(2, "resources/list"), {"code": -32601, "message": "Method not found: resources/list"}),
            ("arguments not an object", request(2, "tools/call", name="get_context", arguments=["orders"]),
             {"code": -32602, "message": "tools/call arguments must be an object"}),
        ]
        for name, line, error in cases:
            with self.subTest(name):
                self.assertEqual([{"jsonrpc": "2.0", "id": 2, "error": error}], self.exchange(line))

    def test_malformed_and_invalid_messages_get_json_rpc_errors_and_the_loop_goes_on(self) -> None:
        replies = self.exchange(
            "{not json", "[1]", json.dumps({"jsonrpc": "2.0", "id": 3}),
            json.dumps({"jsonrpc": "2.0", "id": 4, "method": "ping", "params": [1]}), request(5, "ping"),
        )
        self.assertEqual(
            [(None, -32700), (None, -32600), (3, -32600), (4, -32600), (5, None)],
            [(reply["id"], reply.get("error", {}).get("code")) for reply in replies],
        )
        self.assertTrue(replies[0]["error"]["message"].startswith("Parse error: "))

    def test_an_unexpected_failure_is_an_internal_error_logged_on_stderr_and_the_server_keeps_serving(self) -> None:
        logged = io.StringIO()
        with contextlib.redirect_stderr(logged):
            replies = self.exchange(request(1, "tools/call", name="coverage", arguments={}), request(2, "ping"), run=raising_command)
        self.assertEqual(
            [{"jsonrpc": "2.0", "id": 1, "error": {"code": -32603, "message": "Internal error: broken handler"}},
             {"jsonrpc": "2.0", "id": 2, "result": {}}],
            replies,
        )
        self.assertEqual("mcp-serve: tools/call failed: RuntimeError('broken handler')\n", logged.getvalue())


class CommandLineTest(McpServerTest):
    def test_mcp_serve_prints_only_the_replies_and_exits_zero_at_end_of_input(self) -> None:
        registry, repo = self.roots()
        result = self.run_cli(
            "mcp-serve", "--registry-root", registry, "--repo-root", repo,
            stdin=request(1, "tools/call", name="lookup", arguments={"asset": "rules", "query": "zzz"}) + "\n" + request(2, "ping") + "\n",
        )
        replies = [json.loads(line) for line in result.stdout.splitlines()]
        self.assertEqual(
            (0, "", [1, 2], {}),
            (result.returncode, result.stderr, [reply["id"] for reply in replies], replies[1]["result"]),
        )
        self.assertEqual("[]\n", replies[0]["result"]["content"][0]["text"])

    def test_mcp_serve_on_an_input_stream_that_cannot_be_reconfigured_serves_it_as_given(self) -> None:
        registry, repo = self.roots()
        arguments = argparse.Namespace(registry_root=Path(registry), repo_root=Path(repo))
        with patch.object(sys, "stdin", io.StringIO("")), \
                patch("domain_registry.commands.read.serve") as served:
            exit_code = handle_mcp_serve(arguments)
        self.assertEqual(
            (0, ServerRoots(Path(registry).resolve(), Path(repo).resolve())), (exit_code, served.call_args.args[0])
        )

    def test_mcp_serve_reads_its_input_as_utf8_whatever_the_locale(self) -> None:
        registry, repo = self.roots()
        environment = {key: value for key, value in os.environ.items() if key not in ("PYTHONIOENCODING", "PYTHONUTF8")}
        result = subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", "mcp-serve", "--registry-root", registry, "--repo-root", repo],
            cwd=Path(__file__).parent, input=(request(1, "tools/call", name="get_context", arguments={"id": "訂單"}) + "\n").encode("utf-8"),
            capture_output=True, env=environment, check=False,
        )
        [reply] = [json.loads(line) for line in result.stdout.decode("utf-8").splitlines()]
        self.assertEqual("ERROR: context not found: 訂單\n", reply["result"]["content"][0]["text"])


if __name__ == "__main__":
    unittest.main()
