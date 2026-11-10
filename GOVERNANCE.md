# Governance

## Model

SecPlexus is maintainer-led. Until more maintainers are added, **EayonLee
(@EayonLee)** is the sole maintainer and makes the final decision on scope,
releases and merges.

## Decisions

- Small fixes and documentation changes: pull request with the relevant checks.
- Features, contract changes, migrations and significant designs: an approved
  plan under `docs/plans/<request-slug>/` before implementation. See
  [CONTRIBUTING.md](CONTRIBUTING.md).
- Security reports: private channel in [SECURITY.md](SECURITY.md), never a public
  issue.
- Day-to-day technical questions: GitHub Issues. The maintainer aims for lazy
  consensus; if consensus is unclear, the maintainer decides and records the
  reason in the issue or pull request.

## Roles

| Role | Expectations |
| --- | --- |
| Contributor | Opens issues or pull requests, follows the Code of Conduct and contribution rules. |
| Reviewer | Invited by the maintainer; reviews changes in an area and gives actionable feedback. Approval does not grant merge permission. |
| Maintainer | Owns direction, releases, repository settings and merges; protects security and compatibility. |

## Becoming a maintainer

There is no fixed quota. A contributor may be invited after sustained,
high-quality contributions, careful reviews, good judgment on security and
compatibility, and consistent adherence to the Code of Conduct.

## Releases

- Before 1.0, `main` is the development line and APIs may change.
- After 1.0, releases follow Semantic Versioning and are recorded in
  [CHANGELOG.md](CHANGELOG.md).
- A release is cut from `main` after the required checks pass and the changelog
  is updated. There is no fixed release cadence.

## Changes to this document

Open a pull request. Governance changes need maintainer approval and are recorded
in the pull request discussion.
