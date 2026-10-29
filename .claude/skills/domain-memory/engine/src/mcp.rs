use crate::pyvalue::{message_text, str_of};
use std::io::{BufRead, Write};
use std::path::{Path, PathBuf};

use crate::json::{self, Object, Style, Value};
use crate::py;
use crate::revision;
use crate::store::ASSETS;

const MODERN_VERSIONS: [&str; 1] = ["2026-07-28"];
const LEGACY_VERSIONS: [&str; 4] = ["2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"];
const CACHEABLE_METHODS: [&str; 2] = ["server/discover", "tools/list"];
const VERSION_KEY: &str = "io.modelcontextprotocol/protocolVersion";
const CAPABILITIES_KEY: &str = "io.modelcontextprotocol/clientCapabilities";
const SERVER_INFO_KEY: &str = "io.modelcontextprotocol/serverInfo";
const SERVER_NAME: &str = "domain-memory";
const PLUGIN_MANIFEST: &str = include_str!("../../.claude-plugin/plugin.json");

const PARSE_ERROR: i64 = -32700;
const INVALID_REQUEST: i64 = -32600;
const METHOD_NOT_FOUND: i64 = -32601;
const INVALID_PARAMS: i64 = -32602;
const UNSUPPORTED_VERSION: i64 = -32022;

const REGISTRY_ROOT: &str = "--registry-root";
const REPO_ROOT: &str = "--repo-root";
const BOTH_ROOTS: &[&str] = &[REGISTRY_ROOT, REPO_ROOT];
const PREVIEW_STATUSES: &[&str] = &["submitted", "verified", "approved", "applied"];
const REPOSITORY_PATH: &str = "a path inside the repository, relative to its root";

pub type Runner = fn(&[String]) -> (i32, String, String);

#[derive(Clone, Copy, PartialEq)]
enum Kind {
    Text,
    Boolean,
    Array,
}

struct Parameter {
    name: &'static str,
    kind: Kind,
    required: bool,
    choices: Vec<String>,
    inside_repository: bool,
}

struct Tool {
    command: &'static str,
    description: &'static str,
    roots: &'static [&'static str],
    parameters: Vec<Parameter>,
    prints_object: bool,
}

struct ProtocolError {
    code: i64,
    message: String,
    data: Option<Value>,
}

struct Server {
    registry: PathBuf,
    repo: PathBuf,
    run: Runner,
    tools: Vec<Tool>,
}

fn parameter(name: &'static str, kind: Kind) -> Parameter {
    Parameter {
        name,
        kind,
        required: false,
        choices: Vec::new(),
        inside_repository: false,
    }
}

fn required(name: &'static str, kind: Kind) -> Parameter {
    Parameter {
        required: true,
        ..parameter(name, kind)
    }
}

fn asset_choices() -> Vec<String> {
    ASSETS.iter().map(|(name, _, _)| name.to_string()).collect()
}

fn asset(required_here: bool) -> Parameter {
    Parameter {
        required: required_here,
        choices: asset_choices(),
        ..parameter("asset", Kind::Text)
    }
}

fn tool(
    command: &'static str,
    description: &'static str,
    roots: &'static [&'static str],
    parameters: Vec<Parameter>,
    prints_object: bool,
) -> Tool {
    Tool {
        command,
        description,
        roots,
        parameters,
        prints_object,
    }
}

fn tools() -> Vec<Tool> {
    use Kind::{Array, Boolean, Text};
    let registry: &'static [&'static str] = &[REGISTRY_ROOT];
    vec![
        tool(
            "get-context",
            "Retrieve a Context model: its vocabulary, aggregates, rules, contracts and collaborations.",
            registry,
            vec![required("id", Text), parameter("summary", Boolean)],
            true,
        ),
        tool(
            "get-record",
            "Retrieve one Registry record exactly as stored, with the tests that prove a reviewed rule or contract.",
            registry,
            vec![asset(true), required("id", Text)],
            true,
        ),
        tool(
            "resolve-terms",
            "Resolve vocabulary: which reviewed term each query names, and which records mention it.",
            registry,
            vec![
                required("query", Array),
                parameter("context", Text),
                parameter("with_mentions", Boolean),
            ],
            false,
        ),
        tool(
            "lookup",
            "Search one asset of the Registry by text.",
            registry,
            vec![asset(true), required("query", Text)],
            false,
        ),
        tool(
            "dependents",
            "Follow a record's relations: every record a change to it may touch.",
            registry,
            vec![asset(true), required("id", Text)],
            true,
        ),
        tool(
            "coverage",
            "Report modelling gaps: Contexts without records, unproven rules, records without evidence.",
            registry,
            Vec::new(),
            true,
        ),
        tool(
            "brief",
            "Brief a change since a base revision, or one record given by asset and id, before review.",
            BOTH_ROOTS,
            vec![
                parameter("base", Text),
                asset(false),
                parameter("id", Text),
                parameter("markdown", Boolean),
            ],
            false,
        ),
        tool(
            "boundary-impact",
            "Find the Context boundaries a change since a base revision touches.",
            BOTH_ROOTS,
            vec![required("base", Text)],
            true,
        ),
        tool(
            "verify-evidence",
            "Verify that the Registry's citations still match the code, for every file or the given paths.",
            BOTH_ROOTS,
            vec![parameter("path", Array), parameter("strict", Boolean)],
            true,
        ),
        tool(
            "validate",
            "Validate the Registry's shape and references.",
            BOTH_ROOTS,
            vec![parameter("require_reviewed", Boolean)],
            false,
        ),
        tool(
            "readiness",
            "Assess what Domain Memory work this repository is ready for.",
            BOTH_ROOTS,
            Vec::new(),
            true,
        ),
        tool(
            "list-proposals",
            "List the Change Packages under a directory with their state.",
            &[],
            vec![Parameter {
                inside_repository: true,
                ..required("changes_root", Text)
            }],
            false,
        ),
        tool(
            "validate-change-package",
            "Validate a Change Package, or preview what the next stage still needs.",
            BOTH_ROOTS,
            vec![
                Parameter {
                    inside_repository: true,
                    ..required("package_root", Text)
                },
                Parameter {
                    choices: PREVIEW_STATUSES
                        .iter()
                        .map(|status| status.to_string())
                        .collect(),
                    ..parameter("as_status", Text)
                },
            ],
            false,
        ),
    ]
}

fn text(value: &str) -> Value {
    Value::Str(value.to_string())
}

fn texts(values: &[&str]) -> Value {
    Value::Array(values.iter().map(|value| text(value)).collect())
}

fn object(entries: Vec<(&str, Value)>) -> Value {
    let mut object = Object::new();
    for (key, value) in entries {
        object.insert(key, value);
    }
    Value::Object(object)
}

fn code(number: i64) -> Value {
    Value::Int(number.to_string())
}

fn supported_versions() -> Vec<&'static str> {
    MODERN_VERSIONS
        .iter()
        .chain(LEGACY_VERSIONS.iter())
        .copied()
        .collect()
}

fn capabilities() -> Value {
    object(vec![(
        "tools",
        object(vec![("listChanged", Value::Bool(false))]),
    )])
}

fn server_info() -> Value {
    let version = json::parse(PLUGIN_MANIFEST)
        .ok()
        .and_then(|manifest| {
            manifest
                .as_object()?
                .get("version")?
                .as_str()
                .map(str::to_string)
        })
        .unwrap_or_default();
    object(vec![
        ("name", text(SERVER_NAME)),
        ("version", Value::Str(version)),
    ])
}

impl Parameter {
    fn flag(&self) -> String {
        format!("--{}", self.name.replace('_', "-"))
    }

    fn schema(&self) -> Value {
        let mut schema = Object::new();
        schema.insert(
            "type",
            text(match self.kind {
                Kind::Text => "string",
                Kind::Boolean => "boolean",
                Kind::Array => "array",
            }),
        );
        if self.kind == Kind::Array {
            schema.insert("items", object(vec![("type", text("string"))]));
        }
        if !self.choices.is_empty() {
            schema.insert(
                "enum",
                Value::Array(self.choices.iter().map(|choice| text(choice)).collect()),
            );
        }
        if self.inside_repository {
            schema.insert("description", text(REPOSITORY_PATH));
        }
        Value::Object(schema)
    }

    fn command_words(&self, value: &Value, repo_root: &Path) -> Result<Vec<String>, String> {
        let fits = match (self.kind, value) {
            (Kind::Text, Value::Str(_)) | (Kind::Boolean, Value::Bool(_)) => true,
            (Kind::Array, Value::Array(items)) => {
                items.iter().all(|item| matches!(item, Value::Str(_)))
            }
            _ => false,
        };
        if !fits {
            let wanted = match self.kind {
                Kind::Text => "a string",
                Kind::Boolean => "a boolean",
                Kind::Array => "a list of strings",
            };
            return Err(format!("argument {} takes {wanted}", self.name));
        }
        Ok(match value {
            Value::Bool(set) => {
                if *set {
                    vec![self.flag()]
                } else {
                    Vec::new()
                }
            }
            Value::Array(items) => items
                .iter()
                .flat_map(|item| [self.flag(), str_of(item)])
                .collect(),
            Value::Str(given) if self.inside_repository => {
                let resolved = revision::inside(repo_root, Path::new(given)).ok_or_else(|| {
                    format!(
                        "argument {} must lie inside the repository {}: {given}",
                        self.name,
                        py::shown(repo_root)
                    )
                })?;
                vec![self.flag(), py::shown(&resolved)]
            }
            other => vec![self.flag(), str_of(other)],
        })
    }
}

impl Tool {
    fn name(&self) -> String {
        self.command.replace('-', "_")
    }

    fn listing(&self) -> Value {
        let mut properties = Object::new();
        for parameter in &self.parameters {
            properties.insert(parameter.name, parameter.schema());
        }
        let required: Vec<&str> = self
            .parameters
            .iter()
            .filter(|parameter| parameter.required)
            .map(|parameter| parameter.name)
            .collect();
        let mut listing = Object::new();
        listing.insert("name", Value::Str(self.name()));
        listing.insert("description", text(self.description));
        listing.insert(
            "inputSchema",
            object(vec![
                ("type", text("object")),
                ("properties", Value::Object(properties)),
                ("required", texts(&required)),
                ("additionalProperties", Value::Bool(false)),
            ]),
        );
        listing.insert(
            "annotations",
            object(vec![
                ("readOnlyHint", Value::Bool(true)),
                ("destructiveHint", Value::Bool(false)),
                ("openWorldHint", Value::Bool(false)),
            ]),
        );
        if self.prints_object {
            listing.insert(
                "outputSchema",
                object(vec![
                    ("type", text("object")),
                    (
                        "description",
                        Value::Str(format!(
                            "The JSON object {} prints, as references/script-api.md describes it.",
                            self.command
                        )),
                    ),
                ]),
            );
        }
        Value::Object(listing)
    }
}

fn protocol(code: i64, message: impl Into<String>) -> ProtocolError {
    ProtocolError {
        code,
        message: message.into(),
        data: None,
    }
}

fn error_reply(identifier: Value, number: i64, message: &str, data: Option<Value>) -> Value {
    let mut error = Object::new();
    error.insert("code", code(number));
    error.insert("message", text(message));
    if let Some(data) = data {
        error.insert("data", data);
    }
    object(vec![
        ("jsonrpc", text("2.0")),
        ("id", identifier),
        ("error", Value::Object(error)),
    ])
}

fn declares_its_version(method: &str, params: &Object) -> Result<bool, ProtocolError> {
    let empty = Object::new();
    let meta = match params.get("_meta") {
        Some(Value::Object(meta)) => meta,
        _ => &empty,
    };
    let requested = meta.get(VERSION_KEY).filter(|value| **value != Value::Null);
    if requested.is_none() && method != "server/discover" {
        return Ok(false);
    }
    let Some(requested) = requested.filter(|_| meta.get(CAPABILITIES_KEY).is_some()) else {
        return Err(protocol(
            INVALID_PARAMS,
            format!("a request must carry _meta {VERSION_KEY} and {CAPABILITIES_KEY}"),
        ));
    };
    let supported = supported_versions();
    if !requested
        .as_str()
        .is_some_and(|version| supported.contains(&version))
    {
        return Err(ProtocolError {
            code: UNSUPPORTED_VERSION,
            message: "Unsupported protocol version".to_string(),
            data: Some(object(vec![
                ("supported", texts(&supported)),
                ("requested", requested.clone()),
            ])),
        });
    }
    Ok(true)
}

fn is_request(message: &Value) -> bool {
    let Value::Object(message) = message else {
        return false;
    };
    matches!(message.get("method"), Some(Value::Str(_)))
        && matches!(message.get("params"), None | Some(Value::Object(_)))
}

impl Server {
    fn answer(&self, line: &str) -> Option<Value> {
        let message = match json::loads(line) {
            Ok(message) => message,
            Err(error) => {
                return Some(error_reply(
                    Value::Null,
                    PARSE_ERROR,
                    &format!("Parse error: {error}"),
                    None,
                ));
            }
        };
        if !is_request(&message) {
            let identifier = match &message {
                Value::Object(message) => message.get("id").cloned().unwrap_or(Value::Null),
                _ => Value::Null,
            };
            return Some(error_reply(
                identifier,
                INVALID_REQUEST,
                "Invalid Request: expected an object with a method and object params",
                None,
            ));
        }
        let Value::Object(message) = message else {
            return None;
        };
        let identifier = message.get("id")?.clone();
        let method = message
            .get("method")
            .and_then(Value::as_str)
            .unwrap_or_default();
        let empty = Object::new();
        let params = message
            .get("params")
            .and_then(Value::as_object)
            .unwrap_or(&empty);
        let answered = declares_its_version(method, params)
            .and_then(|per_request| Ok((per_request, self.result(method, params)?)));
        let (per_request, result) = match answered {
            Ok(answered) => answered,
            Err(error) => {
                return Some(error_reply(
                    identifier,
                    error.code,
                    &error.message,
                    error.data,
                ));
            }
        };
        let shaped = if per_request {
            let mut shaped = Object::new();
            shaped.insert("resultType", text("complete"));
            for (key, value) in result.iter() {
                shaped.insert(key.clone(), value.clone());
            }
            if CACHEABLE_METHODS.contains(&method) {
                shaped.insert("ttlMs", Value::int(300_000));
                shaped.insert("cacheScope", text("public"));
            }
            shaped
        } else {
            result
        };
        Some(object(vec![
            ("jsonrpc", text("2.0")),
            ("id", identifier),
            ("result", Value::Object(shaped)),
        ]))
    }

    fn result(&self, method: &str, params: &Object) -> Result<Object, ProtocolError> {
        let answer = match method {
            "initialize" => {
                let version = params
                    .get("protocolVersion")
                    .and_then(Value::as_str)
                    .filter(|requested| LEGACY_VERSIONS.contains(requested))
                    .unwrap_or(LEGACY_VERSIONS[0]);
                object(vec![
                    ("protocolVersion", text(version)),
                    ("capabilities", capabilities()),
                    ("serverInfo", server_info()),
                ])
            }
            "server/discover" => object(vec![
                ("supportedVersions", texts(&supported_versions())),
                ("capabilities", capabilities()),
                ("_meta", object(vec![(SERVER_INFO_KEY, server_info())])),
            ]),
            "ping" => object(Vec::new()),
            "tools/list" => object(vec![(
                "tools",
                Value::Array(self.tools.iter().map(Tool::listing).collect()),
            )]),
            "tools/call" => self.call_tool(params)?,
            _ => {
                return Err(protocol(
                    METHOD_NOT_FOUND,
                    format!("Method not found: {method}"),
                ));
            }
        };
        match answer {
            Value::Object(answer) => Ok(answer),
            _ => Ok(Object::new()),
        }
    }

    fn call_tool(&self, params: &Object) -> Result<Value, ProtocolError> {
        let named = params.get("name");
        let tool = named
            .and_then(Value::as_str)
            .and_then(|name| self.tools.iter().find(|tool| tool.name() == name));
        let Some(tool) = tool else {
            let shown = named.map_or_else(|| "null".to_string(), message_text);
            return Err(protocol(INVALID_PARAMS, format!("Unknown tool: {shown}")));
        };
        let empty = Value::Object(Object::new());
        let Some(arguments) = params.get("arguments").unwrap_or(&empty).as_object() else {
            return Err(protocol(
                INVALID_PARAMS,
                "tools/call arguments must be an object",
            ));
        };
        let (exit_code, printed, noted) = match self.command_line(tool, arguments) {
            Ok(words) => (self.run)(&words),
            Err(error) => (1, format!("ERROR: {error}\n"), String::new()),
        };
        let content: Vec<Value> = [&printed, &noted]
            .into_iter()
            .filter(|shown| !shown.is_empty())
            .map(|shown| object(vec![("type", text("text")), ("text", text(shown))]))
            .collect();
        let mut result = Object::new();
        result.insert("content", Value::Array(content));
        result.insert("isError", Value::Bool(exit_code != 0));
        if exit_code == 0
            && let Ok(structured @ Value::Object(_)) = json::loads(&printed)
        {
            result.insert("structuredContent", structured);
        }
        Ok(Value::Object(result))
    }

    fn command_line(&self, tool: &Tool, arguments: &Object) -> Result<Vec<String>, String> {
        let known: Vec<&str> = tool
            .parameters
            .iter()
            .map(|parameter| parameter.name)
            .collect();
        let mut unknown: Vec<&str> = arguments
            .iter()
            .map(|(name, _)| name.as_str())
            .filter(|name| !known.contains(name))
            .collect();
        unknown.sort_unstable();
        if !unknown.is_empty() {
            let taken = if known.is_empty() {
                "none".to_string()
            } else {
                known.join(", ")
            };
            return Err(format!(
                "{} takes no argument {}; it takes {taken}",
                tool.name(),
                unknown.join(", ")
            ));
        }
        let missing: Vec<&str> = tool
            .parameters
            .iter()
            .filter(|parameter| parameter.required && arguments.get(parameter.name).is_none())
            .map(|parameter| parameter.name)
            .collect();
        match missing.as_slice() {
            [] => {}
            [only] => return Err(format!("{}: argument {only} is required", tool.name())),
            several => {
                return Err(format!(
                    "{}: arguments {} are required",
                    tool.name(),
                    several.join(", ")
                ));
            }
        }
        let mut words = vec![tool.command.to_string()];
        for flag in tool.roots {
            let root = if *flag == REGISTRY_ROOT {
                &self.registry
            } else {
                &self.repo
            };
            words.extend([flag.to_string(), py::shown(root)]);
        }
        for (name, value) in arguments.iter() {
            let Some(parameter) = tool
                .parameters
                .iter()
                .find(|parameter| parameter.name == name)
            else {
                continue;
            };
            words.extend(parameter.command_words(value, &self.repo)?);
        }
        Ok(words)
    }
}

fn lines_of(chunk: &[u8]) -> Vec<String> {
    let text = String::from_utf8_lossy(chunk);
    let text = if cfg!(windows) {
        text.replace("\r\n", "\n").replace('\r', "\n")
    } else {
        text.into_owned()
    };
    text.split_inclusive('\n').map(str::to_string).collect()
}

pub fn serve(registry: PathBuf, repo: PathBuf, run: Runner) {
    let server = Server {
        registry,
        repo,
        run,
        tools: tools(),
    };
    let reply_style = Style {
        indent: None,
        sort_keys: false,
        ensure_ascii: false,
        compact: false,
    };
    let mut source = std::io::stdin().lock();
    let mut chunk = Vec::new();
    loop {
        chunk.clear();
        match source.read_until(b'\n', &mut chunk) {
            Ok(0) | Err(_) => return,
            Ok(_) => {}
        }
        for line in lines_of(&chunk) {
            if let Some(reply) = server.answer(&line) {
                let mut sink = std::io::stdout().lock();
                let _ =
                    sink.write_all(format!("{}\n", json::dumps(&reply, reply_style)).as_bytes());
                let _ = sink.flush();
            }
        }
    }
}
