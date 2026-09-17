#!/usr/bin/env python3
import sys

import checkout

TEMPLATE = 'exec "/opt/skillhub/infra/deploy/$SKILLHUB_ROLE/bin/skillhub-bootstrap"\n'


def tree(**extra):
    files = {
        checkout.TEMPLATE: TEMPLATE,
        "infra/deploy/node/checkout-paths": "infra/deploy/node/\ninfra/compose/node.yml\n",
        "infra/deploy/node/bin/skillhub-bootstrap": 'here=/opt/skillhub/infra/deploy\n"$here"/node/systemd/*\n',
        "infra/deploy/node/systemd/skillhub.service": "ExecStart=/opt/skillhub/infra/compose/node.yml\n",
        "infra/compose/node.yml": "",
    }
    files.update(extra)
    return files


def problems_for(files):
    problems, _ = checkout.gaps("node", {path for path, text in files.items() if text is not None},
                                lambda path: files[path])
    return problems


def test_a_checkout_carrying_everything_its_files_name_has_no_gap():
    assert problems_for(tree()) == []


def test_a_script_naming_a_file_outside_the_checkout_is_a_gap():
    files = tree(**{"infra/deploy/node/bin/skillhub-bootstrap": '"$here/common/install-docker"\n',
                    "infra/deploy/common/install-docker": ""})
    assert problems_for(files) == [
        "infra/deploy/node/bin/skillhub-bootstrap names infra/deploy/common/install-docker, "
        "which the node checkout does not carry"], problems_for(files)


def test_a_compose_mount_through_the_deploy_dir_default_is_a_reference():
    files = tree(**{"infra/compose/node.yml": "- ${SKILLHUB_DEPLOY_DIR:-/opt/skillhub}/infra/observability/alerts.yml:/x\n",
                    "infra/observability/alerts.yml": ""})
    assert problems_for(files) == [
        "infra/compose/node.yml names infra/observability/alerts.yml, which the node checkout does not carry"]


def test_a_glob_reference_needs_only_its_directory_to_hold_a_file():
    files = tree(**{"infra/deploy/node/systemd/skillhub.service": None})
    assert problems_for(files) == [
        "infra/deploy/node/bin/skillhub-bootstrap names infra/deploy/node/systemd, which the node checkout does not carry"]


def test_the_template_is_read_with_the_role_substituted():
    files = tree(**{"infra/deploy/node/bin/skillhub-bootstrap": None})
    assert problems_for(files) == [
        "%s names infra/deploy/node/bin/skillhub-bootstrap, which the node checkout does not carry" % checkout.TEMPLATE]


def test_an_entry_selecting_no_file_is_stale():
    files = tree(**{"infra/deploy/node/checkout-paths": "infra/deploy/node/\ninfra/compose/node.yml\ntools/gone.py\n"})
    assert problems_for(files) == ["checkout-paths entry 'tools/gone.py' selects no file"]


def test_a_file_entry_selects_only_that_path_and_a_directory_entry_everything_under_it():
    assert checkout.selects("infra/compose/node.yml", "infra/compose/node.yml")
    assert not checkout.selects("infra/compose/node.yml", "infra/compose/node.yml.bak")
    assert checkout.selects("infra/deploy/node/", "infra/deploy/node/bin/x")
    assert not checkout.selects("infra/deploy/node/", "infra/deploy/nodes/bin/x")


def test_files_outside_the_checkout_are_not_scanned():
    files = tree(**{"infra/deploy/other/bin/skillhub-bootstrap": '"$here/other/missing"\n'})
    assert problems_for(files) == []


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
