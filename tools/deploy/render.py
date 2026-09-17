#!/usr/bin/env python3
import argparse
import ipaddress
import json
import pathlib
import re
import subprocess
import sys
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
TEMPLATE = ROOT / "infra" / "deploy" / "cloud-init.yaml.tmpl"
REGISTRY = "ghcr.io"
OWNER = "arthurc02"
DEFAULT_REPOSITORY = "https://github.com/ArthurC02/SkillHub"
MANIFEST_TYPES = ", ".join([
    "application/vnd.oci.image.index.v1+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
    "application/vnd.oci.image.manifest.v1+json",
    "application/vnd.docker.distribution.manifest.v2+json",
])

ROLES = {
    "control-plane": {
        "settings": [
            "SKILLHUB_DOMAIN", "SKILLHUB_ACME_EMAIL", "SKILLHUB_PRIVATE_IP", "SKILLHUB_ALERT_EMAIL",
            "SKILLHUB_SMTP_SMARTHOST", "SKILLHUB_SMTP_FROM", "SKILLHUB_SMTP_USERNAME", "SKILLHUB_GATEWAY_URL",
        ],
        "images": {
            "SKILLHUB_WEB_IMAGE": "skillhub-web",
            "SKILLHUB_PLATFORM_IMAGE": "skillhub-platform",
            "SKILLHUB_LLM_IMAGE": "skillhub-llm",
            "SKILLHUB_POSTGRES_IMAGE": "skillhub-postgres",
        },
    },
    "gateway": {
        "settings": ["SKILLHUB_PRIVATE_IP"],
        "images": {},
    },
    "sandbox": {
        "settings": ["SKILLHUB_PRIVATE_IP", "SKILLHUB_CONTROL_PLANE_IP", "SKILLHUB_SANDBOX_SLOTS"],
        "images": {
            "SKILLHUB_SANDBOXD_IMAGE": "skillhub-sandboxd",
        },
        "runtime_image": "SKILLHUB_SANDBOX_IMAGE",
    },
}
RUNTIME_REPOSITORY = "skillhub-runtime-agent-sdk"
RUNTIME_DOCKERFILE = "infra/images/runtime-agent-sdk/Dockerfile"

RELEASE = re.compile(r"^[0-9a-f]{40}$")
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
SAFE_VALUE = re.compile(r"^[A-Za-z0-9@._:/+,=-]+$")


class RenderError(Exception):
    pass


def is_ipv4(value):
    try:
        return ipaddress.ip_address(value).version == 4
    except ValueError:
        return False


SHAPES = {
    "SKILLHUB_PRIVATE_IP": (is_ipv4, "an IPv4 address"),
    "SKILLHUB_CONTROL_PLANE_IP": (is_ipv4, "an IPv4 address"),
    "SKILLHUB_SANDBOX_SLOTS": (lambda value: re.fullmatch(r"[1-9][0-9]*", value) is not None, "a positive integer"),
}


def read_settings(text):
    settings = {}
    for number, line in enumerate(text.splitlines(), 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, sep, value = line.partition("=")
        if not sep:
            raise RenderError("line %d is not KEY=VALUE" % number)
        settings[key.strip()] = value.strip()
    return settings


def ghcr_digest(repository, tag):
    token_url = "https://%s/token?scope=repository:%s/%s:pull&service=%s" % (REGISTRY, OWNER, repository, REGISTRY)
    with urllib.request.urlopen(token_url, timeout=30) as response:
        token = json.load(response)["token"]
    request = urllib.request.Request(
        "https://%s/v2/%s/%s/manifests/%s" % (REGISTRY, OWNER, repository, tag),
        method="HEAD",
        headers={"Authorization": "Bearer " + token, "Accept": MANIFEST_TYPES},
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return response.headers.get("Docker-Content-Digest", "")
    except urllib.error.HTTPError as error:
        if error.code == 404:
            return ""
        raise


def git_show(release, path):
    return subprocess.run(["git", "-C", str(ROOT), "show", "%s:%s" % (release, path)],
                          check=True, capture_output=True, text=True).stdout


def dockerfile_arg(dockerfile, name):
    match = re.search(r"^ARG %s=(\S+)$" % name, dockerfile, re.MULTILINE)
    if not match:
        raise RenderError("%s at this release declares no ARG %s" % (RUNTIME_DOCKERFILE, name))
    return match.group(1)


def pinned(repository, tag, resolve, why):
    digest = resolve(repository, tag)
    if not DIGEST.match(digest or ""):
        raise RenderError("%s/%s/%s:%s is not published; %s" % (REGISTRY, OWNER, repository, tag, why))
    return "%s/%s/%s:%s@%s" % (REGISTRY, OWNER, repository, tag, digest)


def release_env(role, release, settings, resolve=ghcr_digest, read_file=git_show):
    if role not in ROLES:
        raise RenderError("unknown role %r; known roles: %s" % (role, ", ".join(sorted(ROLES))))
    if not RELEASE.match(release):
        raise RenderError("release %r is not a full 40-character commit SHA" % release)
    spec = ROLES[role]

    missing = [key for key in spec["settings"] if not settings.get(key)]
    if missing:
        raise RenderError("%s needs %s" % (role, ", ".join(missing)))
    unknown = sorted(set(settings) - set(spec["settings"]) - {"SKILLHUB_REPOSITORY"})
    if unknown:
        raise RenderError("%s does not use %s; remove it so a typo cannot pass for a setting" % (role, ", ".join(unknown)))

    for key, (accepts, shape) in SHAPES.items():
        if key in spec["settings"] and not accepts(settings[key]):
            raise RenderError("%s=%r is not %s" % (key, settings[key], shape))

    values = {
        "SKILLHUB_ROLE": role,
        "SKILLHUB_RELEASE": release,
        "SKILLHUB_REPOSITORY": settings.get("SKILLHUB_REPOSITORY", DEFAULT_REPOSITORY),
    }
    values.update({key: settings[key] for key in spec["settings"]})
    for key, repository in spec["images"].items():
        values[key] = pinned(repository, release, resolve,
                             "only a commit whose main CI pushed its images can be deployed")
    if "runtime_image" in spec:
        dockerfile = read_file(release, RUNTIME_DOCKERFILE)
        version = dockerfile_arg(dockerfile, "IMAGE_VERSION")
        values[spec["runtime_image"]] = pinned(RUNTIME_REPOSITORY, version, resolve,
                                               "the runtime image workflow publishes each IMAGE_VERSION once")
        values["SKILLHUB_SANDBOX_RUNTIME_VERSION"] = dockerfile_arg(dockerfile, "CLAUDE_AGENT_SDK_VERSION")

    for key, value in values.items():
        if not SAFE_VALUE.match(value):
            raise RenderError("%s=%r has characters a shell, compose or envsubst would reinterpret" % (key, value))
    return "".join("%s=%s\n" % (key, values[key]) for key in values)


def cloud_init(env):
    indented = "".join("      %s\n" % line for line in env.splitlines())
    template = TEMPLATE.read_text(encoding="utf-8")
    if template.count("@RELEASE_ENV@\n") != 1:
        raise RenderError("%s must hold exactly one @RELEASE_ENV@ line" % TEMPLATE.relative_to(ROOT))
    return template.replace("@RELEASE_ENV@\n", indented)


def main(argv=None):
    parser = argparse.ArgumentParser(description="Render a node's cloud-init user-data for one release.")
    parser.add_argument("role", choices=sorted(ROLES))
    parser.add_argument("--release", required=True, help="the 40-character commit SHA to deploy")
    parser.add_argument("--settings", required=True, type=pathlib.Path, help="KEY=VALUE file of non-secret settings")
    parser.add_argument("--release-env", action="store_true",
                        help="print only /etc/skillhub/release.env, for upgrading a running node")
    args = parser.parse_args(argv)
    try:
        env = release_env(args.role, args.release, read_settings(args.settings.read_text(encoding="utf-8")))
    except RenderError as error:
        print("render: %s" % error, file=sys.stderr)
        return 1
    sys.stdout.write(env if args.release_env else cloud_init(env))
    return 0


if __name__ == "__main__":
    sys.exit(main())
