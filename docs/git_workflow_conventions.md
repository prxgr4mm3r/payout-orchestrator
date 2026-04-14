# Git Workflow Conventions

## 1. Purpose

This document defines the naming rules for branches, pull requests, and commits in `payout-orchestrator`.

The goal is:

- make history easy to scan
- make PR type obvious before review starts
- keep roadmap order visible during MVP development
- keep commits small and easy to understand

---

## 2. Branch Naming

Branch format:

```text
<type>/mvp-<nn>-<topic>
```

Examples:

```text
feat/mvp-01-api-bootstrap
feat/mvp-02-postgres-wiring
feat/mvp-03-client-auth
fix/mvp-09-funding-source-ownership
docs/mvp-00-git-workflow
refactor/mvp-14-payout-service-split
chore/mvp-02-add-dev-tools
test/mvp-15-payout-smoke-tests
build/mvp-02-docker-improvements
```

### Branch Types

- `feat/` for new behavior or product functionality
- `fix/` for bug fixes
- `docs/` for documentation-only changes
- `refactor/` for structural code changes without intended behavior changes
- `chore/` for housekeeping, tooling, or repository maintenance
- `test/` for test-only changes
- `build/` for build, CI, docker, dependency, or environment changes
- `perf/` for performance improvements
- `spike/` for short-lived research branches

### Rules

- If the branch changes user-visible or API-visible behavior, prefer `feat/` or `fix/`.
- If the branch only changes documentation, use `docs/`.
- If the branch type is hard to choose, the scope is probably too broad.

---

## 3. Pull Request Titles

PR title format:

```text
[<type>] MVP-<nn> <title>
```

Examples:

```text
[feat] MVP-01 API bootstrap
[feat] MVP-02 Postgres wiring
[docs] MVP-00 Git workflow conventions
[fix] MVP-09 Funding source ownership validation
[refactor] MVP-14 Split payout creation service
```

### Rules

- Use sentence case for the title body.
- Keep the title short and specific.
- The PR title must describe one review question.

---

## 4. Commit Messages

Commit format:

```text
<scope>: <change>
```

Examples:

```text
app: add api bootstrap
api: add healthcheck route
docs: add local run notes
db: add clients schema
auth: add api key middleware
payouts: validate funding source ownership
processor: add polling loop
test: cover duplicate payout requests
```

### Preferred Scopes

- `app`
- `api`
- `auth`
- `docs`
- `db`
- `domain`
- `funding-sources`
- `payouts`
- `processor`
- `provider`
- `test`
- `build`
- `config`

### Rules

- One commit must represent one minimal behavior change or one technical invariant.
- The commit message should be understandable without extra spoken context.
- If the commit message needs the word `and`, the commit is probably too large.
- Formatting-only changes must not be mixed with behavioral changes.

---

## 5. Examples For MVP

### PR-01

Branch:

```text
feat/mvp-01-api-bootstrap
```

PR title:

```text
[feat] MVP-01 API bootstrap
```

Commits:

```text
app: add api bootstrap
docs: add local run notes
```

### PR-02

Branch:

```text
feat/mvp-02-postgres-wiring
```

PR title:

```text
[feat] MVP-02 Postgres wiring
```

Possible commits:

```text
db: add postgres connection config
app: wire postgres lifecycle
build: add migration commands
```

---

## 6. Collaboration Flow

For Codex-assisted implementation work, use this default collaboration flow:

1. Agree the PR scope before implementation starts.
2. Split the PR into minimal commits before or during implementation.
3. Codex implements one logical commit worth of changes at a time.
4. At each commit boundary, Codex stops and provides the exact `git add` and `git commit -S` commands.
5. The user reviews the staged scope and creates the signed commit locally.
6. Codex then continues with the next commit-sized change.
7. After all commits for the PR are complete, Codex provides:
   - the branch creation command
   - the suggested PR title
   - the suggested PR description

Rules:

- Codex does not create commits unless explicitly asked.
- The user creates signed commits locally.
- The user creates the branch and opens the pull request.
- Codex is responsible for keeping the implementation aligned with the agreed commit boundaries.

---

## 7. Default Rule

Unless there is a strong reason to do otherwise:

- use typed branches
- use typed PR titles
- use scoped commit messages
- preserve the MVP sequence number in branch and PR names

This is the default convention for all MVP work in this repository.
