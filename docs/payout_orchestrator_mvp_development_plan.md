# Payout Orchestrator — MVP Development Plan

## 1. Purpose

This document defines how MVP development should be split into small, review-friendly pull requests and minimal commits.

The goal is:

- keep the full development context easy to follow
- make every PR easy to review in one sitting
- make every commit explain one minimal change
- avoid mixing infrastructure, refactoring, and feature logic in the same diff

---

## 2. Working Rules

### 2.1 Pull Request Rules

Each PR must follow these rules:

- one PR answers one review question
- one PR adds one vertical slice or one technical invariant
- one PR must not mix feature work, refactoring, and formatting
- each PR should stay roughly within `150-400` lines of useful diff, excluding generated files
- each PR must be readable from top to bottom: schema or queries -> service -> handler or wiring -> tests -> docs
- each PR must leave the branch buildable and manually verifiable
- generated files should be isolated in their own commit inside the PR when possible

### 2.2 Commit Rules

Each commit must follow these rules:

- one commit changes one minimal behavior or one technical invariant
- the commit message must describe the change in one sentence without `and`
- formatting-only changes must not be mixed with behavioral changes
- migrations, SQL, and generated code should be split when possible
- tests for new logic should be in the same commit or immediately after it
- avoid large "checkpoint" commits with unrelated edits

### 2.3 Naming Rules

Branch naming:

- `mvp/01-app-bootstrap`
- `mvp/02-postgres-wiring`
- `mvp/03-clients-auth`

Commit naming:

- `app: add http server bootstrap`
- `db: add clients schema`
- `auth: add api key middleware`
- `payouts: validate funding source ownership`

PR naming:

- `MVP-01 App bootstrap`
- `MVP-02 Postgres wiring`
- `MVP-03 Client auth`

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
- service or processor changes

How to verify
- exact manual verification steps
- test commands if available

Risks
- edge cases or follow-up work
```

---

## 4. Review Heuristics

Before opening a PR, ask:

- can the reviewer summarize this PR in one sentence
- can the reviewer explain why this PR exists without reading unrelated files
- can this PR be reverted without breaking unrelated work
- does this PR introduce exactly one new concept

If the answer is `no` to any of these, the PR is probably too large.

Before creating a commit, ask:

- does this commit change exactly one thing
- can I describe this commit without using `and`
- would `git show` be easy to understand without extra oral context

If the answer is `no`, the commit is probably too large.

---

## 5. MVP Roadmap

### PR-01 App Bootstrap

Goal:

- create the minimal HTTP service skeleton

Commits:

- `app: add server bootstrap`
- `api: add healthcheck route`
- `docs: add local run notes`

Outcomes:

- binary starts
- graceful shutdown works
- `GET /healthz` responds successfully

### PR-02 Postgres Wiring

Goal:

- connect the app to PostgreSQL and add migration workflow

Commits:

- `db: add postgres connection config`
- `app: wire postgres lifecycle`
- `build: add migration commands`

Outcomes:

- app can connect to Postgres
- migrations can be run locally

### PR-03 Clients And Auth

Goal:

- introduce clients table and API key authentication

Commits:

- `db: add clients schema`
- `db: add clients queries`
- `auth: add api key middleware`
- `auth: inject client context`
- `test: cover client authentication`

Outcomes:

- authenticated client identity is available in request context

### PR-04 Funding Sources Schema

Goal:

- introduce funding source persistence

Commits:

- `db: add funding sources schema`
- `db: add funding source queries`
- `sqlc: regenerate funding source code`

Outcomes:

- funding sources can be stored and queried

### PR-05 Funding Sources Create

Goal:

- implement `POST /funding-sources`

Commits:

- `service: add funding source creation use case`
- `api: add create funding source handler`
- `test: cover funding source creation`

Outcomes:

- authenticated client can create its own funding source

### PR-06 Funding Sources Read

Goal:

- implement funding source reads for the authenticated client

Commits:

- `service: add funding source read use cases`
- `api: add funding source read handlers`
- `test: cover client scoped funding source reads`

Outcomes:

- client can list and fetch its own funding sources

### PR-07 Payouts Schema

Goal:

- introduce payout persistence and status model

Commits:

- `db: add payouts schema`
- `db: add payout queries`
- `domain: add payout statuses`
- `test: cover payout status transitions`

Outcomes:

- payouts can be persisted with controlled statuses

### PR-08 Payouts Read

Goal:

- implement payout reads for the authenticated client

Commits:

- `service: add payout read use cases`
- `api: add payout read handlers`
- `test: cover client scoped payout reads`

Outcomes:

- client can list and fetch its own payouts

### PR-09 Payouts Create Pending

Goal:

- implement minimal payout creation without idempotency or outbox

Commits:

- `service: validate funding source ownership`
- `service: add pending payout creation`
- `api: add create payout handler`
- `test: cover pending payout creation`

Outcomes:

- client can create a payout in `pending`

### PR-10 Idempotency

Goal:

- make payout creation idempotent

Commits:

- `db: add idempotency keys schema`
- `db: add idempotency queries`
- `service: add payout idempotency logic`
- `test: cover duplicate payout requests`
- `test: cover idempotency conflicts`

Outcomes:

- same request returns the existing payout
- conflicting request under same key is rejected

### PR-11 Transactional Outbox Write

Goal:

- write payout, idempotency record, and outbox event atomically

Commits:

- `db: add outbox events schema`
- `db: add outbox queries`
- `service: wrap payout creation in transaction`
- `test: cover atomic payout write flow`

Outcomes:

- accepted payout always has a corresponding outbox record

### PR-12 Provider Simulator

Goal:

- introduce provider abstraction and simulator implementation

Commits:

- `provider: add payout provider interface`
- `provider: add simulator implementation`
- `test: cover simulator outcomes`

Outcomes:

- app has a stable provider boundary before background processing is added

### PR-13 Background Processor Loop

Goal:

- add the polling processor that claims pending outbox events

Commits:

- `db: add outbox claim query`
- `processor: add polling loop`
- `app: wire background processor lifecycle`
- `test: cover outbox event claiming`

Outcomes:

- processor can safely pick pending payout work from PostgreSQL

### PR-14 Async Payout Execution

Goal:

- execute claimed payouts asynchronously to completion

Commits:

- `processor: load payout execution context`
- `processor: mark payout as processing`
- `processor: execute provider call`
- `payouts: persist successful outcomes`
- `outbox: mark event processed`
- `test: cover successful async payout flow`

Outcomes:

- payout moves from `pending` to `processing` to `succeeded`

### PR-15 Failed Outcomes And Logging

Goal:

- persist failures and improve observability

Commits:

- `db: add payout failure reason`
- `processor: persist failed payout outcomes`
- `observability: add structured payout logs`
- `test: cover failed async payout flow`

Outcomes:

- payout can end in `failed`
- failure reason is visible through API and logs

### PR-16 Smoke Tests And Final Hardening

Goal:

- close MVP with an end-to-end verification path

Commits:

- `test: add end to end payout smoke test`
- `app: improve shutdown behavior`
- `docs: add mvp verification guide`

Outcomes:

- MVP flow is manually and automatically verifiable

---

## 6. Execution Order

Development should move in this order:

1. `PR-01` to `PR-06` for the intake layer and funding source flow.
2. `PR-07` to `PR-11` for reliable payout creation.
3. `PR-12` to `PR-15` for asynchronous execution.
4. `PR-16` to close the MVP with smoke coverage and final hardening.

---

## 7. Guardrails During Implementation

While implementing this roadmap:

- do not skip ahead to future PR concerns
- do not "sneak in" cleanup unrelated to the current PR
- do not add future infrastructure early unless current PR explicitly requires the seam
- prefer adding narrow domain types and validations before wider infrastructure
- prefer merging a small complete slice over starting multiple partially finished slices

This plan should be treated as the default development path for the MVP unless the design itself changes.
