# Identity Observability & Risk Intelligence Platform — Build Breakdown

This document gives you a concrete, step-by-step path to start and build the platform. Use it as your single reference for order of work and decisions.

---

## How to Start (First 2 Weeks)

### Step 0: Environment & Repo Setup
1. **Create the monorepo** (or multi-repo) structure:
   - `identity-observability/`
     - `backend/` (Go services)
     - `connectors/` (IdP sync services — Go or Python)
     - `frontend/` (React + TypeScript)
     - `docs/` (architecture, data flow, runbooks)
     - `infra/` (Terraform, K8s manifests)
     - `pkg/` or `shared/` (canonical schema, shared types)
2. **Choose and provision**:
   - PostgreSQL 15+ (canonical store + optional graph extension)
   - Redis (caching, rate-limit state)
   - (Later) Neo4j or Neptune for graph — or start with PostgreSQL + recursive CTEs.
3. **Document**:
   - One-page architecture diagram (see `DATA_FLOW.md`).
   - Canonical schema (next step) in `docs/canonical-schema.md`.

### Step 1: Define the Canonical Schema (Week 1)
**Goal:** One universal data model that every connector maps into. No UI yet.

- **Core entities** (minimal MVP):
  - `identity` — canonical person (with `employee_id`, `email`, `display_name`, `status`, `source_system`, `source_id`, `synced_at`).
  - `group` — group in a source system (`name`, `source_system`, `source_id`, `synced_at`).
  - `role` — role in a source system (`name`, `privilege_level` e.g. admin/read, `source_system`, `source_id`).
  - `permission` — permission or entitlement (`name`, `resource_type`, `source_system`, `source_id`).
  - `resource` (optional for MVP) — target resource (e.g. AWS account, app).
- **Relationship tables** (many-to-many, with source):
  - `identity_group` (identity_id, group_id, source_system, source_id).
  - `identity_role` (identity_id, role_id, source_system, source_id).
  - `group_role` (group_id, role_id, source_system, source_id).
  - `role_permission` (role_id, permission_id, source_system, source_id).
- **Resolution & lineage**:
  - `identity_sources` — links canonical identity to each source system’s user id (for identity resolution).
  - Keep every `source_id` and `source_system` so you can always trace back.

**Deliverable:** SQL migrations (e.g. `migrations/001_canonical_schema.sql`) and a short doc describing the model.

---

## Phase 1: Graph Engine Foundation (Weeks 2–6)

**Focus:** Data model + graph logic + effective access. No production IdP yet.

### 1.1 Mock Data & Test Dataset
- Create a **mock connector** that reads a static JSON/CSV test dataset and writes into the canonical tables.
- Dataset should include: users, groups, roles, permissions, and relationships across 2–3 “fake” systems (e.g. `okta_mock`, `entra_mock`, `aws_mock`).
- **Deliverable:** Script or job that loads mock data and idempotent upserts by `(source_system, source_id)`.

### 1.2 Graph Representation & Traversal
- **Option A:** Store relationships in PostgreSQL and use **recursive CTEs** to compute paths (User → Group → Role → Permission).
- **Option B:** Use Neo4j or Neptune and store nodes/edges there; sync from canonical DB or write from connectors.
- Implement:
  - **Path enumeration:** Given an identity_id, compute all (identity → group → role → permission) paths.
  - **Effective access:** Materialize “effective permissions” per identity (e.g. table or view: `identity_effective_permissions`).
- **Deliverable:** Service or module that, for any identity, returns the full lineage (path list) and a flattened effective-permission list.

### 1.3 Identity Resolution (MVP)
- **Matching rule:** Same `employee_id` → same canonical identity; else match by `email` (with confidence score).
- Table `identity_sources`: `canonical_identity_id`, `source_system`, `source_user_id`, `confidence`, `updated_at`.
- When mock connector “ingests” users, run resolution so one canonical identity can link to multiple source users.
- **Deliverable:** Resolution function + unit tests; all mock users linked to canonical identities.

### 1.4 APIs (Read-Only)
- **GET** `/api/v1/identities` — list identities (filter by source_system, status).
- **GET** `/api/v1/identities/:id` — single identity + linked sources + effective permissions.
- **GET** `/api/v1/identities/:id/access-lineage` — full explainability paths (User → Group → Role → Permission).
- Use Go (e.g. Gin) + PostgreSQL; optional Redis cache for hot identities.
- **Deliverable:** API spec (OpenAPI) + implementation; test with mock data.

**Phase 1 exit criteria:** You can query an identity by ID and get back effective access and full lineage from mock data.

---

## Phase 2: One Perfect Connector (Weeks 7–12)

**Focus:** One production IdP (e.g. Okta or Entra ID) with robust, incremental sync.

### 2.1 Connector Design
- **Read-only:** Only read users, groups, roles, app assignments from the IdP.
- **Idempotent:** Key by `(source_system, source_id)`; upsert on every run.
- **Incremental:** Use IdP’s “last modified” or “changed since” APIs to fetch only changes when possible.
- **Rate limits:** Respect 429 and backoff; store cursor/state in Redis or DB.

### 2.2 Okta or Entra ID Implementation
- **Okta:** Users API, Groups API, Group Assignments, Apps/App Assignments. Map to Identity, Group, Role, Permission.
- **Entra ID:** Microsoft Graph — users, group membership, directory roles, app role assignments. Map to canonical model.
- **Normalization layer:** One mapping module per IdP that converts API response → canonical entities + relationship rows.
- **Deliverable:** Connector service that runs on a schedule (e.g. cron or K8s CronJob), writes to canonical DB, and logs sync status and errors.

### 2.3 Identity Resolution with Real Data
- Run resolution after each sync (or in batch): link IdP users to existing canonical identities by `employee_id` then `email`.
- Handle conflicts (e.g. same email, different employee_id) with a confidence score and manual-review queue later.
- **Deliverable:** New IdP users correctly linked to canonical identities; `identity_sources` populated.

### 2.4 Connector Health
- **Dashboard or API:** Sync status, last success timestamp, error count, rate-limit usage.
- **Deliverable:** Simple status page or `/api/v1/connectors/okta/status` (or equivalent).

**Phase 2 exit criteria:** Real data from one IdP flows into the canonical model and appears in effective access and lineage APIs.

---

## Phase 3: Risk & Governance MVP (Weeks 13–18)

**Focus:** Risk rules, scores, and first UI for profile + heatmap.

### 3.1 Risk Scoring Engine
- **Rule 1 — Cross-System Admin:** Admin (or equivalent) in ≥2 systems → High risk.
- **Rule 2 — Disabled Drift:** Disabled in one IdP but active in another → Critical.
- **Scoring:** Simple model first: base score 0–100; each rule adds points (e.g. Critical +40, High +25). Cap at 100.
- **Explainability:** Store which rules fired per identity (e.g. `identity_risk_flags`: identity_id, rule_id, severity, detail).
- **Deliverable:** Risk score and flags per identity; API `GET /api/v1/identities/:id/risk`.

### 3.2 Governance Dashboard (UI)
- **Unified Identity Profile Page:** One page per identity: status per source, list of effective permissions, risk score, and “Explain” links.
- **Access Explainability View:** For a chosen permission, show the path(s): User → Group → Role → Permission (and source system).
- **Admin Heatmap:** Counts of privileged identities, cross-cloud admins, MFA coverage % (if you have MFA from IdP).
- **Tech:** React + TypeScript, Tailwind; consume backend APIs.
- **Deliverable:** Working UI for profile, explainability, and heatmap.

### 3.3 Audit Export
- **Export:** For a user or for “all high-risk,” generate a bundle (CSV/PDF) with: identity, effective permissions, lineage paths, risk score, timestamp.
- **Deliverable:** Export button or API that produces an audit bundle.

**Phase 3 exit criteria:** Security/audit can open one user, see risk and lineage, and export evidence.

---

## Phase 4: Expansion (Weeks 19–26)

**Focus:** More connectors, scale, policy-as-code, full feature set.

### 4.1 Additional Connectors
- Add AWS IAM (users, groups, roles, policies) and/or Google Workspace, SailPoint, Identity HQ.
- Reuse same pattern: connector → normalization → canonical DB → resolution.

### 4.2 Policy-as-Code & SoD
- Define rules (e.g. “Toxic Combination: Create Vendor + Approve Payment”) as config or code.
- SoD violation detection: for each identity, check if they have both roles in a forbidden pair; flag and add to risk.

### 4.3 Scale & Production Readiness
- Target 1M+ identities: index optimization, partitioning, caching (Redis), read replicas.
- Consider Kafka for event-driven sync and real-time risk updates (post-MVP).
- Kubernetes + Terraform for deployment and multi-tenant isolation if needed.

### 4.4 Full Feature Parity
- Historical risk score and access snapshots (for auditor “past year” view).
- Risk change alerts (e.g. score 50 → 85).
- Connector health dashboard, data lineage (source_id/sync_timestamp) visible in UI.
- D3.js (or similar) for richer graph visualization of access lineage.

---

## Technology Summary

| Layer / Need        | Technology (MVP)              | Optional / Later     |
|---------------------|-------------------------------|----------------------|
| Backend API         | Go 1.21+, Gin                 | —                    |
| Canonical store     | PostgreSQL 15+                | —                    |
| Graph               | PostgreSQL + recursive CTEs    | Neo4j, Neptune       |
| Cache / rate limit  | Redis                         | —                    |
| Connectors          | Go or Python                  | —                    |
| Frontend            | React, TypeScript, Tailwind   | D3.js for graph viz  |
| IdP APIs            | Okta API, Microsoft Graph      | AWS IAM, SailPoint   |
| Infra               | Docker, docker-compose        | Kubernetes, Terraform|
| Observability       | Structured logs, Prometheus   | —                    |

---

## Suggested First Steps (This Week)

1. **Create repo** and folders as in Step 0.
2. **Write** `docs/canonical-schema.md` and `migrations/001_canonical_schema.sql` (identities, groups, roles, permissions, relationship tables, identity_sources).
3. **Implement** mock connector + small test dataset; run resolution and one “effective access” query (even in SQL script).
4. **Implement** one API: `GET /api/v1/identities/:id` with effective permissions and lineage (from mock data).
5. **Sketch** data flow (see `DATA_FLOW.md`) and keep it in `docs/`.

After that, iterate: add risk rules, then one real connector, then the first UI screens.
