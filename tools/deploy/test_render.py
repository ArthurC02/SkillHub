#!/usr/bin/env python3
import sys

import yaml

import render

RELEASE = "8145f05667bd23f93b69d5a1745bfe3a9f1bbcc9"
DIGEST = "sha256:" + "a" * 64
SETTINGS = {
    "SKILLHUB_DOMAIN": "skillhub.example",
    "SKILLHUB_ACME_EMAIL": "owner@skillhub.example",
    "SKILLHUB_PRIVATE_IP": "10.0.0.2",
    "SKILLHUB_ALERT_EMAIL": "owner@skillhub.example",
    "SKILLHUB_SMTP_SMARTHOST": "smtp.example:587",
    "SKILLHUB_SMTP_FROM": "alerts@skillhub.example",
    "SKILLHUB_SMTP_USERNAME": "alerts@skillhub.example",
    "SKILLHUB_GATEWAY_URL": "http://10.0.0.3:4000",
}


def published(repository, tag):
    return DIGEST


def expect_error(fragment, *args, **kwargs):
    try:
        render.release_env(*args, **kwargs)
    except render.RenderError as error:
        assert fragment in str(error), "error %r does not mention %r" % (str(error), fragment)
        return
    raise AssertionError("expected a RenderError mentioning %r" % fragment)


def test_a_control_plane_release_pins_every_image_by_tag_and_digest():
    env = render.read_settings(render.release_env("control-plane", RELEASE, SETTINGS, resolve=published))
    assert env["SKILLHUB_ROLE"] == "control-plane"
    assert env["SKILLHUB_RELEASE"] == RELEASE
    assert env["SKILLHUB_REPOSITORY"] == render.DEFAULT_REPOSITORY
    for key, repository in render.ROLES["control-plane"]["images"].items():
        assert env[key] == "ghcr.io/arthurc02/%s:%s@%s" % (repository, RELEASE, DIGEST), env[key]
    assert env["SKILLHUB_DOMAIN"] == "skillhub.example"


def test_an_image_the_release_never_published_refuses_the_render():
    def platform_missing(repository, tag):
        return "" if repository == "skillhub-platform" else DIGEST
    expect_error("skillhub-platform:%s is not published" % RELEASE, "control-plane", RELEASE, SETTINGS,
                 resolve=platform_missing)


def test_a_short_or_symbolic_release_is_refused():
    expect_error("not a full 40-character commit SHA", "control-plane", RELEASE[:7], SETTINGS, resolve=published)
    expect_error("not a full 40-character commit SHA", "control-plane", "main", SETTINGS, resolve=published)


def test_a_missing_and_a_misspelled_setting_are_both_named():
    without = dict(SETTINGS)
    del without["SKILLHUB_ALERT_EMAIL"]
    expect_error("needs SKILLHUB_ALERT_EMAIL", "control-plane", RELEASE, without, resolve=published)
    typo = dict(SETTINGS, SKILLHUB_ALERT_EMIAL="x@y")
    expect_error("does not use SKILLHUB_ALERT_EMIAL", "control-plane", RELEASE, typo, resolve=published)


def test_a_value_a_shell_would_reinterpret_is_refused():
    expect_error("SKILLHUB_DOMAIN", "control-plane", RELEASE, dict(SETTINGS, SKILLHUB_DOMAIN="a.example$(id)"),
                 resolve=published)
    expect_error("SKILLHUB_SMTP_FROM", "control-plane", RELEASE, dict(SETTINGS, SKILLHUB_SMTP_FROM="a b"),
                 resolve=published)


def test_a_gateway_release_carries_only_its_address_and_resolves_no_image():
    def nothing_to_resolve(repository, tag):
        raise AssertionError("the gateway pins its image in compose; nothing should be resolved")
    env = render.read_settings(render.release_env("gateway", RELEASE, {"SKILLHUB_PRIVATE_IP": "10.0.0.3"},
                                                  resolve=nothing_to_resolve))
    assert env == {"SKILLHUB_ROLE": "gateway", "SKILLHUB_RELEASE": RELEASE,
                   "SKILLHUB_REPOSITORY": render.DEFAULT_REPOSITORY, "SKILLHUB_PRIVATE_IP": "10.0.0.3"}, env
    expect_error("does not use SKILLHUB_DOMAIN", "gateway", RELEASE,
                 {"SKILLHUB_PRIVATE_IP": "10.0.0.3", "SKILLHUB_DOMAIN": "x"}, resolve=nothing_to_resolve)


def test_an_unknown_role_is_refused():
    expect_error("unknown role", "database", RELEASE, SETTINGS, resolve=published)


def test_the_user_data_is_cloud_config_carrying_the_release_env_verbatim():
    env = render.release_env("control-plane", RELEASE, SETTINGS, resolve=published)
    text = render.cloud_init(env)
    assert text.startswith("#cloud-config\n")
    document = yaml.safe_load(text)
    files = {entry["path"]: entry for entry in document["write_files"]}
    assert files["/etc/skillhub/release.env"]["content"] == env
    assert "git" in document["packages"]
    command = document["runcmd"][0][2]
    assert 'test "$(git rev-parse HEAD)" = "$SKILLHUB_RELEASE"' in command
    assert 'exec "infra/deploy/$SKILLHUB_ROLE/bin/skillhub-bootstrap"' in command


def test_settings_files_ignore_comments_and_refuse_a_line_without_a_value():
    assert render.read_settings("# note\n\nA=1\n B = two \n") == {"A": "1", "B": "two"}
    try:
        render.read_settings("A=1\nnot a setting\n")
    except render.RenderError as error:
        assert "line 2" in str(error)
    else:
        raise AssertionError("a line without = was accepted")


if __name__ == "__main__":
    failures = 0
    for name, test in sorted(globals().items()):
        if name.startswith("test_") and callable(test):
            try:
                test()
                print("ok   %s" % name)
            except AssertionError as error:
                failures += 1
                print("FAIL %s: %s" % (name, error))
    sys.exit(1 if failures else 0)
