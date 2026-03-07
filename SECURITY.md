# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | No |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please report vulnerabilities via [GitHub Security Advisories](https://github.com/jpvelasco/ludus/security/advisories/new).

Include:
- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Suggested fix (if any)

You should receive an acknowledgment within 48 hours. We will work with you to understand the issue and coordinate a fix before any public disclosure.

## Scope

Ludus generates Dockerfiles, executes shell commands, and interacts with AWS APIs. Security-relevant areas include:

- **Generated Dockerfiles** — Ludus includes built-in Dockerfile linting (`ludus doctor`) with rules for root user, unpinned base images, package cleanup, and sensitive environment variables
- **Shell command execution** — All commands go through `runner.Runner`; no user input is interpolated into shell strings
- **AWS credentials** — Ludus never stores or logs AWS credentials; it relies on the AWS SDK credential chain
- **Container images** — Optional Trivy scanning via `ludus doctor` for vulnerability detection

## Dependencies

Ludus uses Dependabot to monitor Go module and GitHub Actions dependencies weekly. Security updates are prioritized.
