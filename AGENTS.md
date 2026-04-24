# Repository Instructions

## Git Ownership

The human maintainer signs and creates all commits manually.

Codex must not run these operations unless the user explicitly asks for that exact
operation in the current chat:

- `git add`
- `git commit`
- `git commit --amend`
- `git tag`
- `git push`
- creating or updating pull requests

Codex may:

- edit files needed for the requested task
- run formatters, generators, tests, and local verification commands
- inspect git status, diffs, logs, branches, and remotes
- create or switch local branches only when explicitly asked
- suggest commit messages and commit splits for the human maintainer

If files are already staged, Codex must leave the index unchanged unless the user
explicitly asks to stage, unstage, or commit files.

After implementation, Codex should report:

- changed files
- verification commands that were run
- suggested commit split, when useful

## Architecture Documentation

When an implementation changes system architecture, service boundaries,
runtime topology, persistence ownership, messaging flow, or other design-level
behavior, Codex must update the system design documentation in the same change.

The update must keep both parts synchronized:

- textual design description
- architecture diagrams, including Mermaid diagrams when the affected concept is
  represented visually

An architecture change is incomplete if the prose and diagrams disagree.
