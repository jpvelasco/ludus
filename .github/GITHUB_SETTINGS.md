# GitHub Repository Settings

These settings are configured outside the repo (GitHub UI or API) and cannot
be enforced by files in the codebase. Re-apply these when setting up a fork
or new instance.

---

## Apply via `gh api` (replace `OWNER/REPO`)

```bash
# General settings
gh api repos/OWNER/REPO \
  --method PATCH \
  --field delete_branch_on_merge=true \
  --field default_branch=main

# Dependabot alerts + security updates
gh api repos/OWNER/REPO/vulnerability-alerts --method PUT
gh api repos/OWNER/REPO/automated-security-fixes --method PUT

# Secret scanning + push protection
gh api repos/OWNER/REPO \
  --method PATCH \
  --header "Content-Type: application/json" \
  --input - <<'EOF'
{
  "security_and_analysis": {
    "secret_scanning": { "status": "enabled" },
    "secret_scanning_push_protection": { "status": "enabled" }
  }
}
EOF

# CodeQL default setup
gh api repos/OWNER/REPO/code-scanning/default-setup \
  --method PATCH \
  --input - <<'EOF'
{ "state": "configured", "query_suite": "default" }
EOF

# Branch ruleset: Protect main
gh api repos/OWNER/REPO/rulesets \
  --method POST \
  --header "Content-Type: application/json" \
  --input - <<'EOF'
{
  "name": "Protect main",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": { "include": ["~DEFAULT_BRANCH"], "exclude": [] }
  },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false,
        "allowed_merge_methods": ["merge", "squash", "rebase"]
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "do_not_enforce_on_create": false,
        "required_status_checks": [
          { "context": "Build (ubuntu-latest)" },
          { "context": "Build (windows-latest)" },
          { "context": "Lint" },
          { "context": "Lint (Windows)" },
          { "context": "Test (ubuntu-latest)" },
          { "context": "Test (windows-latest)" }
        ]
      }
    }
  ]
}
EOF

# Tag ruleset: Protect release tags
gh api repos/OWNER/REPO/rulesets \
  --method POST \
  --header "Content-Type: application/json" \
  --input - <<'EOF'
{
  "name": "Protect release tags",
  "target": "tag",
  "enforcement": "active",
  "conditions": {
    "ref_name": { "include": ["refs/tags/v*"], "exclude": [] }
  },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    { "type": "update" }
  ]
}
EOF
```

---

## Current settings (jpvelasco/ludus)

### General
- Default branch: `main`
- Auto-delete head branches: enabled

### Branch ruleset: `Protect main`
- Blocks deletion and force pushes
- Requires PR before merging: yes
- Required approvals: 0
- Dismiss stale reviews on push: no
- Require CODEOWNERS review: no
- Require conversation resolution: no
- Required status checks (all must pass):
  - `Build (ubuntu-latest)`
  - `Build (windows-latest)`
  - `Lint`
  - `Lint (Windows)`
  - `Test (ubuntu-latest)`
  - `Test (windows-latest)`
- Require branch up to date: yes
- Enforce on admins: yes

### Tag ruleset: `Protect release tags`
- Pattern: `v*`
- Restrict deletions: yes
- Restrict force pushes: yes
- Restrict updates: yes

### Security & Analysis
- Secret scanning: enabled
- Push protection: enabled
- CodeQL (default setup): enabled — Go, JavaScript/TypeScript, Actions
- Dependabot alerts: enabled
- Dependabot security updates: enabled
