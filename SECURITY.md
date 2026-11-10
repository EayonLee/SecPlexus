# Security Policy

## Supported versions

SecPlexus is pre-1.0. Security fixes land on the `main` branch; there are no
maintained release branches yet.

| Version | Supported |
| --- | --- |
| `main` | Yes |
| Tagged pre-1.0 releases | No |

## Report a vulnerability privately

Do **not** open a public issue, discussion or pull request containing exploit
details, credentials or affected private data.

Use GitHub private vulnerability reporting:

<https://github.com/EayonLee/SecPlexus/security/advisories/new>

If that form is unavailable, contact the maintainer privately through GitHub
(@EayonLee) and ask for a private channel before sending details.

Please include:

- the affected revision or release;
- the affected component (IAM, Console, Web, configuration, migration);
- impact and the conditions required to exploit it;
- reproduction steps or a minimal proof of concept;
- sanitized logs or configuration, with secrets removed.

Never send passwords, session tokens, CSRF values, API keys, private keys or
production data.

## Scope

Reports are especially useful for:

- authentication, session lifecycle, CSRF and origin checks;
- authorization decisions and permission delegation;
- the Web same-origin API proxy and cookie handling;
- service credentials between Console and IAM;
- migration or configuration handling that can expose data;
- audit integrity or bypasses.

Dependency findings without a demonstrated path to impact may be handled as
ordinary maintenance rather than a security advisory.

## Response

We aim to acknowledge a report within five business days and will agree on a
disclosure timeline with the reporter. Timelines depend on severity, affected
versions and the complexity of the fix. Please give us reasonable time to
release a fix before public disclosure.

## Safe harbor

We welcome good-faith research that follows this policy. Do not access other
users' data, degrade the service, use social engineering, or test against a
deployment you do not own without written permission.
