# Security Policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's **[Report a vulnerability](https://github.com/ArthurC02/SkillHub/security/advisories/new)** form rather than in a public issue or pull request. Include what you found, how to reproduce it, and what an attacker gains — a proof of concept helps more than a severity score.

You will get an acknowledgement of the report. This is a small project without a paid security team, so please allow time for a fix before disclosing publicly.

## Supported versions

The project is pre-release. Fixes land on `main`; there is no maintained release branch and no backporting.

## What is in scope

The system is built so that untrusted material — a skill, a script, a dataset someone uploaded — never runs inside the web or API process, and so that the execution plane never reaches the core database directly. Reports that break either of those properties are the ones we most want to see, along with anything that leaks another workspace's data, escapes the sandbox, or exposes a provider key.

## What is deliberately not a boundary

**Clean test mode is not a security boundary, by design.** It is the same program with its database, object storage and sandbox replaced by in-process stand-ins so the system can run on a machine that cannot install software. In that mode the sandbox provides no isolation, presigned object URLs are not verified, and the database serves a single connection. The application says so on screen. Findings that amount to "clean test mode does not isolate" are already documented rather than unknown; a finding that the **default** configuration behaves that way is very much in scope.

Development defaults that exist only behind an explicit opt-in flag — the development login, the relaxed cookie settings used with it, the local CORS allowance — are likewise not vulnerabilities in themselves. A path that reaches them without the flag is.
