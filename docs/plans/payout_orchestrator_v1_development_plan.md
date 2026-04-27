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

### 2.1 Pre-Implementation Discussion

Before implementing the next PR, Codex and the human maintainer must discuss the
implementation plan.

Codex must provide the discussion package in this structure:

- `What problem are we solving?`
- `What is the current code state relative to this problem?`
- `How will we solve this problem?`

The discussion must happen before code changes for the PR begin. The goal is to
make the implementation approach explicit before any branch accumulates diff.

After the discussion, Codex may proceed with implementation only when the human
maintainer asks Codex to implement the PR or a specific part of it.

### 2.2 Pull Request Rules

Each PR must follow these rules:

- one PR answers one review question
- one PR adds one vertical slice or one technical invariant
- one PR must not mix feature delivery, infrastructure extraction, and broad cleanup
- each PR should stay roughly within `150-400` lines of useful diff, excluding generated files
- each PR must remain buildable and manually verifiable
- generated files should be isolated in their own commit when possible

### 2.3 Commit Rules

Each commit must follow these rules:

- one commit changes one minimal behavior or one technical invariant
- the commit message must describe the change in one sentence without `and`
- formatting-only changes must not be mixed with behavioral changes
- schema, SQL, generated code, and runtime wiring should be split when possible
- tests for new logic should be in the same commit or immediately after it

### 2.4 Naming Rules

Branch naming:

- `fix/v1-01-mvp-stabilization`
- `feat/v1-02-payout-business-fields`
- `feat/v1-05-rabbitmq-payout-worker`

Commit naming:

- `test: fix smoke test payout execution wiring`
- `payouts: add recipient fields`
- `platform: add rabbitmq consumer adapter`

PR naming:

- `[fix] V1-01 MVP stabilization`
- `[feat] V1-02 Payout business fields`
- `[feat] V1-05 RabbitMQ payout worker`

### 2.5 PR Handoff Rules

At the end of each completed PR, Codex must provide a handoff package for the human maintainer.

The handoff package must include:

- the exact branch switch command to start the PR branch locally
- the exact commit commands for each intended commit in order
- the final PR title
- the final PR description using the repository PR template

The branch and commit commands are for the human maintainer to run manually.

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

- `test: fix smoke test payout execution wiring`
- `test: make full package test run green`
- `docs: align verification notes with current payout execution setup`

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

### PR-04 Outbox Relay Boundary

Goal:

- separate database polling from payout execution so the queue boundary can be introduced safely

Commits:

- `outbox: rename publisher boundary to relay`
- `outbox: add dispatcher event payload`
- `app: wire outbox relay lifecycle`
- `test: cover outbox dispatch handoff`

Outcomes:

- payout execution is no longer coupled to direct PostgreSQL polling
- the codebase has a stable dispatcher seam for RabbitMQ publishing

### PR-05 RabbitMQ Payout Worker

Goal:

- move payout execution to RabbitMQ-driven workers

Commits:

- `platform: add rabbitmq transport adapter`
- `broker: add payout queue publish and consume boundaries`
- `worker: execute payout jobs from queue`
- `app: add worker runtime wiring`
- `test: cover queued payout execution`

Outcomes:

- outbox relay dispatches payout jobs through RabbitMQ
- payout worker consumes jobs independently from the API process
- payout worker orchestration is kept outside transport adapter packages
- the system matches the intended distributed processing model

### PR-05b RabbitMQ Delivery Guarantees

Goal:

- make payout job delivery through RabbitMQ operationally safe and explicit

Commits:

- `platform: add payout exchange and queue topology`
- `platform: enable publisher confirms for payout publishing`
- `platform: declare durable payout and retry queues`
- `platform: publish payout jobs in persistent mode`
- `docs: add rabbitmq payout topology diagram`

Outcomes:

- payout publication has broker-level confirmation semantics
- payout queue topology is durable and reviewable
- the expected exchange/queue/binding behavior is documented before retry orchestration

### PR-05c Idempotency Schema Consistency

Goal:

- make the database schema express that one payout creation request resolves to one payout

Commits:

- `db: enforce one idempotency key per payout`
- `test: cover idempotency payout uniqueness`
- `docs: align idempotency schema cardinality`

Outcomes:

- `idempotency_keys.payout_id` is unique
- the ERD shows `payouts` to `idempotency_keys` as `1 -> 0..1`
- duplicate idempotency aliases for the same payout are rejected at the database boundary

### PR-06 Webhook Job Emission

Goal:

- generate webhook work when payout execution reaches a final state

Commits:

- `outbox: add payout result webhook event`
- `worker: emit payout result webhook outbox event`
- `test: cover webhook job creation`

Outcomes:

- successful and failed payouts can trigger client notifications
- final payout results create durable webhook outbox events
- webhook delivery has a clean handoff point from payout execution before RabbitMQ publication

### PR-07 Webhook Worker

Goal:

- deliver payout result notifications asynchronously from RabbitMQ webhook jobs

Commits:

- `broker: add webhook delivery topology`
- `broker: add webhook publisher`
- `webhooks: add delivery service`
- `broker: add webhook consumer`
- `worker: process webhook delivery jobs`
- `test: cover successful webhook delivery`

Outcomes:

- clients can receive payout result callbacks
- webhook delivery is decoupled from payout execution through outbox and RabbitMQ
- `webhook_deliveries` records delivery attempts and outcomes

### PR-08 Retry And Attempt Policy

Goal:

- add failure handling rules that make payout and webhook workers operationally useful

Commits:

- `db: add payout execution attempt metadata`
- `db: add payout awaiting retry status`
- `broker: add payout retry and dead letter queue topology`
- `worker: add payout consumer failure routing`
- `worker: add webhook retry scheduling`
- `worker: add payout retry policy`
- `test: cover retry exhaustion`
- `docs: add retry behavior notes`

Outcomes:

- transient failures can be retried with controlled backoff tiers
- exhausted payout jobs move to DLQ instead of looping indefinitely
- payout retry accounting is persisted and inspectable in PostgreSQL
- the system becomes retry-ready in practice, not only in design

### PR-09 Recovery And Provider Idempotency

Goal:

- prevent stuck processing and duplicate external payout side effects

Commits:

- `worker: gate payout execution by pending and awaiting retry statuses`
- `provider: add idempotent payout execution contract`
- `recovery: add stuck processing reconciler service`
- `test: cover payout recovery and idempotent provider replay`
- `docs: add payout recovery behavior notes`

Outcomes:

- payouts stuck in `processing` can be recovered automatically
- payout execution remains safe across redeliveries and worker restarts
- provider-side duplicate payout risk is explicitly controlled

### PR-10 Audit Trail

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

### PR-11 Observability And Runbook

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
3. `PR-04`, `PR-05`, and `PR-05b` to move from PostgreSQL-only polling to RabbitMQ-based payout execution with delivery guarantees.
4. `PR-05c` to tighten idempotency schema consistency before adding more async workflows.
5. `PR-06` and `PR-07` to add webhook delivery as an async workflow.
6. `PR-08` to make failures, retries, and DLQ behavior explicit.
7. `PR-09` to add payout recovery and provider idempotency protections.
8. `PR-10` and `PR-11` to improve investigation and operations.

---

## 6. Guardrails During Implementation

While implementing this roadmap:

- do not add Redis or MongoDB just because they appear in the long-term design
- do not mix queue extraction with webhook semantics in the same PR
- do not postpone verification until the end of the roadmap
- prefer stable seams over broad refactors
- prefer one working worker flow over multiple partially wired services
- do not place payout or webhook worker orchestration logic inside RabbitMQ transport or broker adapter packages
- keep runtime-service-specific application services under the owning service package
- keep top-level `internal/*` packages for genuinely shared domain, integration contract, database, and platform code

This plan should be treated as the default path from MVP to `v1` unless the target design itself changes.
