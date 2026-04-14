# Payout Orchestrator — MVP System Design

## 1. Goal

Build the first working iteration of the payout orchestrator with the smallest scope that still demonstrates the core platform behavior:

- accept payout requests from client systems
- validate and store them reliably
- process them asynchronously
- expose final payout status back to the client

MVP must work end-to-end with minimal infrastructure while preserving architectural seams for future expansion.

---

## 2. MVP Boundaries

### Included in MVP

- one deployable Go service
- PostgreSQL as the only infrastructure dependency
- client authentication via API key
- funding source registration
- payout creation
- idempotent payout creation
- transactional outbox in PostgreSQL
- background payout processor that polls PostgreSQL
- provider simulator integration
- payout status tracking via API
- structured logs for investigation

### Explicitly excluded from MVP

- RabbitMQ
- Redis
- MongoDB
- webhook delivery
- separate outbox publisher service
- separate payout worker service
- retry backoff and DLQ handling
- dashboard UI
- billing
- multi-provider routing

---

## 3. Why This MVP Is Still Extensible

MVP keeps the same core contracts that later iterations will use:

- API writes payouts and outbox events in one transaction
- payout processing is asynchronous from the client perspective
- provider integration is hidden behind an internal adapter boundary
- payout status lifecycle already exists
- the polling processor can later be split into outbox publisher + RabbitMQ + payout worker without changing the public API

This means MVP is not a throwaway prototype. It is the first deployable slice of the target system.

---

## 4. Runtime Model

MVP runs as a single Go process with two internal responsibilities:

- HTTP API for request intake and status reads
- background payout processor loop for asynchronous execution

The process is single-binary, but the responsibilities stay logically separated.

---

## 5. Data Stores

### PostgreSQL

PostgreSQL is the only required infrastructure dependency in MVP.

Used for:

- clients
- funding_sources
- payouts
- idempotency_keys
- outbox_events

### Deferred Stores

RabbitMQ, Redis, and MongoDB are intentionally not part of MVP.

They remain future extensions once the core payout flow is stable.

---

## 6. Core Actors

### 6.1 Client System

External system integrating with the API.

Responsibilities:

- create funding sources
- create payouts
- query payout status

### 6.2 API Service

Responsibilities:

- authenticate requests
- validate input
- enforce idempotency
- persist funding sources and payouts
- write outbox events in the same transaction as payout creation

Does not:

- execute payouts synchronously
- wait for final provider outcome

### 6.3 Background Payout Processor

Responsibilities:

- poll pending outbox events from PostgreSQL
- claim work with row locking
- load payout and funding source
- transition payout to `processing`
- call provider simulator
- persist final payout status
- mark outbox event as processed

### 6.4 Provider Simulator

Responsibilities:

- emulate external payout execution
- return success or failure

---

## 7. Functional Requirements

### FR-1 Client authentication

The system shall authenticate client requests using an API key.

### FR-2 Funding source registration

The system shall allow a client to register a funding source.

### FR-3 Funding source ownership

Each funding source shall belong to exactly one client.

### FR-4 Payout creation

The system shall allow a client to create a payout request.

### FR-5 Idempotency

The system shall support idempotent payout creation using an idempotency key.

### FR-6 Async processing

The system shall process payouts asynchronously.

### FR-7 Status tracking

The system shall store and expose payout status.

---

## 8. Non-Functional Requirements

### Reliability

- no duplicate payout creation for the same idempotent request
- accepted payouts are not lost if the background processor is temporarily down
- replay-safe processing from PostgreSQL outbox

### Observability

- structured logs
- correlation by `payout_id` and `client_id`
- traceable payout lifecycle

### Maintainability

- thin HTTP handlers
- business logic in services/use cases
- explicit SQL and migrations

---

## 9. Entity Model for MVP

### 9.1 Client

Fields:

- id
- name
- api_key or api_key_hash
- status
- created_at
- updated_at

### 9.2 FundingSource

Fields:

- id
- client_id
- name
- type
- payment_account_id
- status
- created_at
- updated_at

Rule:

A funding source belongs to exactly one client.

### 9.3 Payout

Fields:

- id
- client_id
- funding_source_id
- external_id
- recipient
- amount
- currency
- status
- failure_reason
- created_at
- updated_at

### 9.4 IdempotencyKey

Fields:

- client_id
- key
- request_hash
- payout_id
- created_at

### 9.5 OutboxEvent

Fields:

- id
- event_type
- entity_id
- payload
- status
- created_at
- processed_at

---

## 10. Payout Lifecycle

Statuses:

- `pending`
- `processing`
- `succeeded`
- `failed`

Allowed transitions:

- `pending -> processing`
- `processing -> succeeded`
- `processing -> failed`

Invalid transitions must be rejected.

---

## 11. Authentication Model

Suggested transport:

- `X-API-Key: <key>`

Flow:

1. API extracts API key from request headers.
2. API loads the client.
3. API verifies the client is active.
4. API injects `client_id` into request context.
5. Handler and service logic execute under that client identity.

---

## 12. Idempotency Design

Idempotency is enforced at API intake.

Flow:

1. Client sends `POST /payouts` with `Idempotency-Key`.
2. API computes request hash.
3. API checks `(client_id, idempotency_key)`.
4. If a record exists:
   - same hash -> return existing payout
   - different hash -> reject with conflict
5. If no record exists:
   - create payout
   - create idempotency record
   - create outbox event
   - commit transaction

PostgreSQL is the source of truth for idempotency in MVP.

---

## 13. End-to-End Happy Path

### Step 1: Register funding source

Client calls `POST /funding-sources`.

API:

- authenticates client
- validates payload
- creates funding source in PostgreSQL

### Step 2: Create payout

Client calls `POST /payouts`.

API:

- authenticates client
- validates ownership of funding source
- checks idempotency
- writes payout
- writes idempotency record
- writes outbox event
- commits transaction
- returns payout with `pending` status

### Step 3: Claim outbox event

Background processor polls PostgreSQL and claims one pending payout event.

### Step 4: Execute payout

Background processor:

- loads payout and funding source
- marks payout as `processing`
- calls provider simulator
- stores final result
- marks outbox event as processed

### Step 5: Read final status

Client calls `GET /payouts/{id}` or `GET /payouts` to observe the result.

---

## 14. Failure Scenarios

### 14.1 Processor is temporarily down

Mitigation:

- API still accepts payout requests
- outbox events remain in PostgreSQL
- processing resumes when the processor comes back

### 14.2 Processor crashes during handling

Mitigation:

- claimed work remains replay-safe until marked processed
- payout status transitions must protect against invalid double execution

### 14.3 Duplicate client request

Mitigation:

- API-level idempotency table

### 14.4 Funding source belongs to another client

Mitigation:

- API checks `funding_source.client_id == authenticated_client_id`

### 14.5 Provider returns failure

Mitigation:

- payout moves to `failed`
- failure reason is stored in PostgreSQL
- client reads final status via API

---

## 15. Recommended Repository Shape for MVP

```text
/cmd
  /api

/internal
  /api
  /app
  /domain
  /processor
  /providersimulator
  /platform
    /postgres
    /config
    /logging
  /db
    /queries

/migrations
/docs
```

The shape should already make future extraction possible, but MVP should avoid adding empty packages for services that do not exist yet.

---

## 16. Testing Strategy

### Unit tests

Test:

- status transitions
- validation
- idempotency decision logic

### Integration tests

Test:

- SQL queries
- transaction boundaries
- payout + idempotency + outbox written atomically
- ownership checks
- polling processor updates payout correctly

### End-to-end smoke test

Verify:

- HTTP request accepted
- DB rows created
- outbox event claimed by processor
- payout reaches a final status
- API returns that final status

---

## 17. Development Plan

### Phase 1

- setup repo
- setup PostgreSQL
- setup migrations
- setup sqlc
- healthcheck endpoint

### Phase 2

- clients model
- API key authentication
- funding source create/get/list

### Phase 3

- payout create/get/list
- status model
- pending-only flow

### Phase 4

- idempotency keys table
- idempotent payout creation
- transactional payout + idempotency + outbox write

### Phase 5

- background PostgreSQL polling processor
- provider simulator
- final payout status handling

---

## 18. Post-MVP Evolution

Once MVP is stable, the next steps are:

- introduce RabbitMQ for work distribution
- split processor into outbox publisher and payout worker
- add webhook delivery
- add MongoDB for raw technical payloads if needed
- add Redis for rate limiting or caching if needed

---

## 19. Final Summary

MVP is a deliberately narrow first iteration:

- one service
- one infrastructure dependency
- one reliable async payout flow

It keeps the most important production-style ideas from the full design:

- API-level idempotency
- PostgreSQL as source of truth
- transactional outbox
- asynchronous processing
- clear domain boundaries

This gives you a system that already works, while keeping the path to the fuller architecture straightforward.
