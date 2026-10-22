from __future__ import annotations

import contextlib
import io
import json
import sys
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any, TextIO

from .changes import PREVIEW_STATUSES
from .cli_parser import ASSET_CHOICES, UsageError
from .version import PLUGIN_VERSION

MODERN_VERSIONS = ("2026-07-28",)
CACHEABLE_METHODS = ("server/discover", "tools/list")
CACHE_HINTS = {"ttlMs": 300_000, "cacheScope": "public"}
LEGACY_VERSIONS = ("2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05")
SUPPORTED_VERSIONS = (*MODERN_VERSIONS, *LEGACY_VERSIONS)
VERSION_KEY = "io.modelcontextprotocol/protocolVersion"
CAPABILITIES_KEY = "io.modelcontextprotocol/clientCapabilities"
SERVER_INFO_KEY = "io.modelcontextprotocol/serverInfo"
SERVER_NAME = "domain-memory"
CAPABILITIES = {"tools": {"listChanged": False}}

PARSE_ERROR = -32700
INVALID_REQUEST = -32600
METHOD_NOT_FOUND = -32601
INVALID_PARAMS = -32602
INTERNAL_ERROR = -32603
UNSUPPORTED_VERSION = -32022

REGISTRY_ROOT = "--registry-root"
REPO_ROOT = "--repo-root"
BOTH_ROOTS = (REGISTRY_ROOT, REPO_ROOT)

JSON_TYPES = {"string": str, "boolean": bool, "array": list}
REPOSITORY_PATH = "a path inside the repository, relative to its root"


@dataclass(frozen=True)
class Parameter:
    name: str
    kind: str = "string"
    required: bool = False
    choices: tuple[str, ...] = ()
    inside_repository: bool = False

    @property
    def flag(self) -> str:
        return "--" + self.name.replace("_", "-")

    def schema(self) -> dict[str, Any]:
        schema: dict[str, Any] = {"type": self.kind}
        if self.kind == "array":
            schema["items"] = {"type": "string"}
        if self.choices:
            schema["enum"] = list(self.choices)
        if self.inside_repository:
            schema["description"] = REPOSITORY_PATH
        return schema

    def command_words(self, value: object, repo_root: Path) -> list[str]:
        if not isinstance(value, JSON_TYPES[self.kind]) or (
            isinstance(value, list) and not all(isinstance(item, str) for item in value)
        ):
            raise ValueError(f"argument {self.name} takes {'a list of strings' if self.kind == 'array' else 'a ' + self.kind}")
        if self.kind == "boolean":
            return [self.flag] if value else []
        if self.kind == "array":
            return [word for item in value for word in (self.flag, item)]
        if self.inside_repository:
            return [self.flag, str(_inside_repository(repo_root, self.name, value))]
        return [self.flag, value]


@dataclass(frozen=True)
class Tool:
    command: str
    description: str
    roots: tuple[str, ...]
    parameters: tuple[Parameter, ...] = ()
    prints_object: bool = False

    @property
    def name(self) -> str:
        return self.command.replace("-", "_")

    def listing(self) -> dict[str, Any]:
        listing = {
            "name": self.name,
            "description": self.description,
            "inputSchema": {
                "type": "object",
                "properties": {parameter.name: parameter.schema() for parameter in self.parameters},
                "required": [parameter.name for parameter in self.parameters if parameter.required],
                "additionalProperties": False,
            },
            "annotations": {"readOnlyHint": True, "destructiveHint": False, "openWorldHint": False},
        }
        if self.prints_object:
            listing["outputSchema"] = {
                "type": "object",
                "description": f"The JSON object {self.command} prints, as references/script-api.md describes it.",
            }
        return listing


ASSET = Parameter("asset", required=True, choices=tuple(ASSET_CHOICES))
RECORD_ID = Parameter("id", required=True)

TOOLS = (
    Tool("get-context", "Retrieve a Context model: its vocabulary, aggregates, rules, contracts and collaborations.",
         (REGISTRY_ROOT,), (RECORD_ID, Parameter("summary", "boolean")), prints_object=True),
    Tool("get-record", "Retrieve one Registry record exactly as stored, with the tests that prove a reviewed rule or contract.",
         (REGISTRY_ROOT,), (ASSET, RECORD_ID), prints_object=True),
    Tool("resolve-terms", "Resolve vocabulary: which reviewed term each query names, and which records mention it.",
         (REGISTRY_ROOT,),
         (Parameter("query", "array", required=True), Parameter("context"), Parameter("with_mentions", "boolean"))),
    Tool("lookup", "Search one asset of the Registry by text.", (REGISTRY_ROOT,),
         (ASSET, Parameter("query", required=True))),
    Tool("dependents", "Follow a record's relations: every record a change to it may touch.", (REGISTRY_ROOT,),
         (ASSET, RECORD_ID), prints_object=True),
    Tool("coverage", "Report modelling gaps: Contexts without records, unproven rules, records without evidence.",
         (REGISTRY_ROOT,), prints_object=True),
    Tool("brief", "Brief a change since a base revision, or one record given by asset and id, before review.",
         BOTH_ROOTS,
         (Parameter("base"), Parameter("asset", choices=tuple(ASSET_CHOICES)), Parameter("id"),
          Parameter("markdown", "boolean"))),
    Tool("boundary-impact", "Find the Context boundaries a change since a base revision touches.", BOTH_ROOTS,
         (Parameter("base", required=True),), prints_object=True),
    Tool("verify-evidence", "Verify that the Registry's citations still match the code, for every file or the given paths.",
         BOTH_ROOTS, (Parameter("path", "array"), Parameter("strict", "boolean")), prints_object=True),
    Tool("validate", "Validate the Registry's shape and references.", BOTH_ROOTS,
         (Parameter("require_reviewed", "boolean"),)),
    Tool("readiness", "Assess what Domain Memory work this repository is ready for.", BOTH_ROOTS, prints_object=True),
    Tool("list-proposals", "List the Change Packages under a directory with their state.", (),
         (Parameter("changes_root", required=True, inside_repository=True),)),
    Tool("validate-change-package", "Validate a Change Package, or preview what the next stage still needs.",
         BOTH_ROOTS,
         (Parameter("package_root", required=True, inside_repository=True),
          Parameter("as_status", choices=tuple(PREVIEW_STATUSES)))),
)
TOOLS_BY_NAME = {tool.name: tool for tool in TOOLS}


@dataclass(frozen=True)
class ServerRoots:
    registry: Path
    repo: Path

    def given_for(self, flag: str) -> Path:
        return self.registry if flag == REGISTRY_ROOT else self.repo


class ProtocolError(Exception):
    def __init__(self, code: int, message: str, data: object = None) -> None:
        super().__init__(message)
        self.code = code
        self.data = data


def _inside_repository(repo_root: Path, name: str, given: str) -> Path:
    resolved = (repo_root / given).resolve()
    if not resolved.is_relative_to(repo_root.resolve()):
        raise ValueError(f"argument {name} must lie inside the repository {repo_root}: {given}")
    return resolved


def server_info() -> dict[str, str]:
    return {"name": SERVER_NAME, "version": PLUGIN_VERSION}


def serve(
    roots: ServerRoots, run_command: Callable[[list[str]], int], stdin: TextIO | None = None, stdout: TextIO | None = None
) -> None:
    source, sink = stdin or sys.stdin, stdout or sys.stdout
    server = McpServer(roots, run_command)
    for line in source:
        reply = server.answer(line)
        if reply is not None:
            sink.write(json.dumps(reply, ensure_ascii=False) + "\n")
            sink.flush()


def _error(identifier: object, code: int, message: str, data: object = None) -> dict[str, Any]:
    error: dict[str, Any] = {"code": code, "message": message}
    if data is not None:
        error["data"] = data
    return {"jsonrpc": "2.0", "id": identifier, "error": error}


def _json_object(printed: str) -> dict[str, Any] | None:
    try:
        parsed = json.loads(printed)
    except json.JSONDecodeError:
        return None
    return parsed if isinstance(parsed, dict) else None


def _is_request(message: object) -> bool:
    return (
        isinstance(message, dict)
        and isinstance(message.get("method"), str)
        and isinstance(message.get("params", {}), dict)
    )


def _declares_its_version(method: str, params: dict) -> bool:
    meta = params.get("_meta") if isinstance(params.get("_meta"), dict) else {}
    requested = meta.get(VERSION_KEY)
    if requested is None and method != "server/discover":
        return False
    if requested is None or CAPABILITIES_KEY not in meta:
        raise ProtocolError(INVALID_PARAMS, f"a request must carry _meta {VERSION_KEY} and {CAPABILITIES_KEY}")
    if requested not in SUPPORTED_VERSIONS:
        raise ProtocolError(
            UNSUPPORTED_VERSION, "Unsupported protocol version",
            {"supported": list(SUPPORTED_VERSIONS), "requested": requested},
        )
    return True


class McpServer:
    def __init__(self, roots: ServerRoots, run_command: Callable[[list[str]], int]) -> None:
        self.roots = roots
        self.run_command = run_command

    def answer(self, line: str) -> dict[str, Any] | None:
        try:
            message = json.loads(line)
        except json.JSONDecodeError as error:
            return _error(None, PARSE_ERROR, f"Parse error: {error}")
        if not _is_request(message):
            identifier = message.get("id") if isinstance(message, dict) else None
            return _error(identifier, INVALID_REQUEST, "Invalid Request: expected an object with a method and object params")
        if "id" not in message:
            return None
        params = message.get("params", {})
        try:
            per_request = _declares_its_version(message["method"], params)
            result = self.result(message["method"], params)
        except ProtocolError as error:
            return _error(message["id"], error.code, str(error), error.data)
        except Exception as error:
            print(f"mcp-serve: {message['method']} failed: {error!r}", file=sys.stderr)
            return _error(message["id"], INTERNAL_ERROR, f"Internal error: {error}")
        hints = CACHE_HINTS if message["method"] in CACHEABLE_METHODS else {}
        shaped = {"resultType": "complete", **result, **hints} if per_request else result
        return {"jsonrpc": "2.0", "id": message["id"], "result": shaped}

    def result(self, method: str, params: dict) -> dict[str, Any]:
        if method == "initialize":
            requested = params.get("protocolVersion")
            version = requested if requested in LEGACY_VERSIONS else LEGACY_VERSIONS[0]
            return {"protocolVersion": version, "capabilities": CAPABILITIES, "serverInfo": server_info()}
        if method == "server/discover":
            return {
                "supportedVersions": list(SUPPORTED_VERSIONS), "capabilities": CAPABILITIES,
                "_meta": {SERVER_INFO_KEY: server_info()},
            }
        if method == "ping":
            return {}
        if method == "tools/list":
            return {"tools": [tool.listing() for tool in TOOLS]}
        if method == "tools/call":
            return self.call_tool(params)
        raise ProtocolError(METHOD_NOT_FOUND, f"Method not found: {method}")

    def call_tool(self, params: dict) -> dict[str, Any]:
        name = params.get("name")
        tool = TOOLS_BY_NAME.get(name) if isinstance(name, str) else None
        if tool is None:
            raise ProtocolError(INVALID_PARAMS, f"Unknown tool: {params.get('name')}")
        arguments = params.get("arguments", {})
        if not isinstance(arguments, dict):
            raise ProtocolError(INVALID_PARAMS, "tools/call arguments must be an object")
        printed, noted = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(printed), contextlib.redirect_stderr(noted):
            try:
                exit_code = self.run_command(self.command_line(tool, arguments))
            except (UsageError, ValueError, OSError) as error:
                print(f"ERROR: {error}")
                exit_code = 1
        content = [{"type": "text", "text": text} for text in (printed.getvalue(), noted.getvalue()) if text]
        result: dict[str, Any] = {"content": content, "isError": exit_code != 0}
        structured = None if exit_code else _json_object(printed.getvalue())
        if structured is not None:
            result["structuredContent"] = structured
        return result

    def command_line(self, tool: Tool, arguments: dict[str, object]) -> list[str]:
        known = {parameter.name: parameter for parameter in tool.parameters}
        unknown = sorted(set(arguments) - set(known))
        if unknown:
            raise ValueError(
                f"{tool.name} takes no argument {', '.join(unknown)}; it takes {', '.join(known) or 'none'}"
            )
        missing = sorted(parameter.name for parameter in tool.parameters if parameter.required and parameter.name not in arguments)
        if missing:
            raise ValueError(
                f"{tool.name}: argument {missing[0]} is required" if len(missing) == 1
                else f"{tool.name}: arguments {', '.join(missing)} are required"
            )
        words = [tool.command]
        for flag in tool.roots:
            words += [flag, str(self.roots.given_for(flag))]
        for name, value in arguments.items():
            words += known[name].command_words(value, self.roots.repo)
        return words
