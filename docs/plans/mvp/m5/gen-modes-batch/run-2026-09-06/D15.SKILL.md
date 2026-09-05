---
name: release-flow-execution
description: Use this skill when you need to follow a software release flow shown in a diagram, especially for a main-branch freeze, testing, version bumping, tagging, Docker build, staging deploy, smoke tests, and release-note announcement.
---

# Release flow execution

Follow the release flow in the order shown. This skill describes the process; it does not perform repository changes, tagging, builds, or deployments by itself.

## 1) Freeze the main branch
- Stop merging or landing unrelated changes on `main`.
- Confirm the release window is open and that the branch is intended to be frozen.
- If you cannot enforce the freeze yourself, notify the team responsible for branch control.

## 2) Run the full test suite
- Run the complete automated test suite for the project.
- Do not continue until the suite passes.
- If tests fail, collect the failing test names, error output, and likely cause, then fix or escalate before proceeding.

## 3) Bump the version in `package.json`
- Update the package version according to the project’s versioning rules.
- Keep the version change consistent with the release being prepared.
- Verify that the version field is updated and that no unrelated package metadata changed.

## 4) Write the `CHANGELOG` entry
- Add a release entry describing the changes included in this release.
- Summarize user-visible changes, fixes, and notable internal updates.
- Keep the entry aligned with the version you just set.

## 5) Tag the release
- Create the release tag that matches the new version.
- Ensure the tag points to the release commit containing the version bump and changelog entry.
- If the repository uses signed tags or a specific tag format, follow that convention.

## 6) Build the Docker image
- Build the release Docker image from the tagged release or release commit.
- Confirm the image builds successfully and is labeled with the correct release version.
- Record the image identifier or tag for deployment.

## 7) Deploy to staging
- Deploy the built image to the staging environment.
- Verify the deployment completed successfully and that the staging instance is running the new release.
- If deployment requires approval or a separate pipeline, hand off the exact image/tag and release version.

## 8) Smoke test 5 endpoints
- Run smoke tests against five representative endpoints in staging.
- Confirm each endpoint responds correctly and that the release is functioning as expected.
- If any endpoint fails, stop the release flow, capture the failure details, and investigate before announcing the release.

## 9) Announce in release notes
- Publish or update the release notes with the release announcement.
- Include the version, key changes, and any important deployment or verification notes.
- Make sure the announcement reflects the successful staging deploy and smoke-test results.

## Completion criteria
- The release flow is complete only when all steps above have been finished in order and the smoke tests have passed.
- If any step cannot be completed, report exactly where the flow stopped and what information is needed to continue.
