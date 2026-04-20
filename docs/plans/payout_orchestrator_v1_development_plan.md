# Payout Orchestrator — V1 Development Plan

## 1. Purpose

This document defines how development should move from the completed MVP to a coherent `v1` of the product.

The goal is:

- keep the roadmap review-friendly
- preserve the MVP's working vertical slice while extending it
- reach a version that matches the core platform shape from the system design
- avoid mixing product semantics, reliability work, and infrastructure extraction in the same PR

`v1` in this plan means:

- payouts are business-complete enough for realistic client use
- processing is distributed through RabbitMQ instead of PostgreSQL-only polling
- webhook delivery exists
- retries and operational investigation are first-class concerns
- the system remains understandable and easy to verify locally

---

## 2. Working Rules

### 2.1 Pull Request Rules

Each PR must follow these rules:

- one PR answers one review question
- one PR adds one vertical slice or one technical invariant
- one PR must not mix feature delivery, infrastructure extraction, and broad cleanup
- each PR should stay roughly within `150-400` lines of useful diff, excluding generated files
- each PR must remain buildable and manually verifiable
- generated files should be isolated in their own commit when possible

### 2.2 Commit Rules

Each commit must follow these rules:

- one commit changes one minimal behavior or one technical invariant
- the commit message must describe the change in one sentence without `and`
- formatting-only changes must not be mixed with behavioral changes
- schema, SQL, generated code, and runtime wiring should be split when possible
- tests for new logic should be in the same commit or immediately after it

### 2.3 Naming Rules

Branch naming:

- `fix/v1-01-mvp-stabilization`
- `feat/v1-02-payout-business-fields`
- `feat/v1-05-rabbitmq-payout-worker`

Commit naming:

- `test: fix smoke test processor wiring`
- `payouts: add recipient fields`
- `broker: add rabbitmq payout consumer`

PR naming:

- `[fix] V1-01 MVP stabilization`
- `[feat] V1-02 Payout business fields`
- `[feat] V1-05 RabbitMQ payout worker`

---

## 3. PR Template

Every PR description should use the same structure:

```text
Goal
- what this PR introduces

Not in scope
- what is intentionally deferred

Changes
- schema changes
- API changes
- worker or processing changes

How to verify
- exact manual verification steps
- test commands if available

Risks
- edge cases or follow-up work
```

---

## 4. V1 Roadmap

### PR-01 MVP Stabilization

Goal:

- restore a clean verification baseline before extending the architecture

Commits:

- `test: fix smoke test processor wiring`
- `test: make full package test run green`
- `docs: align verification notes with current processor setup`

Outcomes:

- `go test ./...` passes
- smoke verification reflects the current runtime wiring
- future PRs build on a stable baseline

### PR-02 Payout Business Fields

Goal:

- make payout creation closer to the target business model

Commits:

- `db: add payout external id`
- `db: add payout recipient fields`
- `payouts: validate business payout payload`
- `api: expose payout business fields`
- `test: cover duplicate external references`

Outcomes:

- payout requests can carry a client business reference
- payout requests can identify the intended recipient
- the public API better matches the target design

### PR-03 Webhook Configuration And Delivery Schema

Goal:

- introduce the persistence model required for outbound client notifications

Commits:

- `db: add client webhook configuration`
- `db: add webhook deliveries schema`
- `db: add webhook delivery queries`
- `sqlc: regenerate webhook delivery code`
- `test: cover webhook delivery persistence`

Outcomes:

- clients can have delivery settings
- webhook delivery attempts have a persistent home before workers are added

### PR-04 Outbox Publisher Boundary

Goal:

- separate database polling from payout execution so the queue boundary can be introduced safely

Commits:

- `outbox: add publisher service boundary`
- `outbox: add publishable event payload`
- `app: wire outbox publisher lifecycle`
- `test: cover outbox publish handoff`

Outcomes:

- payout execution is no longer coupled to direct PostgreSQL polling
- the codebase has a stable seam for RabbitMQ publishing

### PR-05 RabbitMQ Payout Worker

Goal:

- move payout execution to RabbitMQ-driven workers

Commits:

- `broker: add rabbitmq payout publisher`
- `broker: add rabbitmq payout consumer`
- `worker: execute payout jobs from queue`
- `app: add worker runtime wiring`
- `test: cover queued payout execution`

Outcomes:

- outbox publisher emits payout jobs to RabbitMQ
- payout worker consumes jobs independently from the API process
- the system matches the intended distributed processing model

### PR-06 Webhook Job Emission

Goal:

- generate webhook work when payout execution reaches a final state

Commits:

- `outbox: add payout result webhook event`
- `worker: persist webhook delivery job`
- `test: cover webhook job creation`

Outcomes:

- successful and failed payouts can trigger client notifications
- webhook delivery has a clean handoff point from payout execution

### PR-07 Webhook Worker

Goal:

- deliver payout result notifications asynchronously

Commits:

- `webhooks: add delivery service`
- `broker: add webhook consumer`
- `worker: process webhook delivery jobs`
- `test: cover successful webhook delivery`

Outcomes:

- clients can receive payout result callbacks
- webhook delivery is decoupled from payout execution

### PR-08 Retry And Attempt Policy

Goal:

- add failure handling rules that make workers operationally useful

Commits:

- `db: add delivery attempt metadata`
- `worker: add webhook retry scheduling`
- `worker: add payout retry policy`
- `test: cover retry exhaustion`
- `docs: add retry behavior notes`

Outcomes:

- transient failures can be retried with controlled policy
- terminal failures become explicit instead of accidental
- the system becomes retry-ready in practice, not only in design

### PR-09 Audit Trail

Goal:

- persist the technical facts needed for support and investigation

Commits:

- `db: add payout audit log`
- `worker: persist provider execution facts`
- `webhooks: persist delivery results`
- `test: cover audit trail writes`

Outcomes:

- payout execution and webhook outcomes are traceable after the fact
- support/debugging no longer depends only on transient logs

### PR-10 Observability And Runbook

Goal:

- make the distributed runtime easier to operate and verify

Commits:

- `observability: add payout correlation logs`
- `api: add readiness checks`
- `metrics: add worker counters`
- `docs: add v1 runbook`

Outcomes:

- operators can distinguish intake health from worker health
- payout lifecycle is easier to trace across services
- local and staging verification become more repeatable

---

## 5. Execution Order

Development should move in this order:

1. `PR-01` to restore a clean baseline.
2. `PR-02` and `PR-03` to complete the core product data model.
3. `PR-04` and `PR-05` to move from PostgreSQL-only polling to RabbitMQ-based payout execution.
4. `PR-06` and `PR-07` to add webhook delivery as an async workflow.
5. `PR-08` to make failures and retries explicit.
6. `PR-09` and `PR-10` to improve investigation and operations.

---

## 6. Guardrails During Implementation

While implementing this roadmap:

- do not add Redis or MongoDB just because they appear in the long-term design
- do not mix queue extraction with webhook semantics in the same PR
- do not postpone verification until the end of the roadmap
- prefer stable seams over broad refactors
- prefer one working worker flow over multiple partially wired services

This plan should be treated as the default path from MVP to `v1` unless the target design itself changes.
