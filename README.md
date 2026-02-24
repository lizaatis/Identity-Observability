# Identity Observability & Risk Intelligence Platform

A read-only **Identity Observability & Risk Intelligence Layer** that observes, correlates, detects, scores, and explains identity risk across connected IAM systems (SailPoint, Okta, Entra ID, AWS, Google Workspace, etc.).

**Positioning:** *“Datadog for IAM”* — measure and expose identity risk across systems, without replacing your IdPs or IGA.

---

## Quick links

| Document | Purpose |
|----------|---------|
| **[BUILD_BREAKDOWN.md](./BUILD_BREAKDOWN.md)** | Step-by-step build plan: how to start, Phase 1–4, tech stack, first steps |
| **[DATA_FLOW.md](./DATA_FLOW.md)** | End-to-end data flow (Mermaid diagrams + Excalidraw redraw guide) |
| **[docs/identity-observability-data-flow.excalidraw](./docs/identity-observability-data-flow.excalidraw)** | Excalidraw diagram of the data flow (open in [excalidraw.com](https://excalidraw.com)) |

---

## Core pillars

1. **Unified Identity Graph** — Canonical User → Group → Role → Permission → Resource across all sources.
2. **Identity Risk & Posture Scoring** — Explainable risk score (0–100) per user/role.
3. **Drift & Exposure Detection** — Disabled drift, cross-system admin stacking, orphaned access.

---

## MVP features

- **Unified Identity Profile** — One view per user: status and access across all systems + risk score.
- **Access Explainability View** — Path from user to permission (e.g. User → Group → Role → Permission).
- **Admin Heatmap** — Privileged identity counts, cross-cloud admins, MFA coverage.
- **MVP risk rules** — Cross-System Admin (High), Disabled Drift (Critical).

---

## Suggested first steps

1. Read [BUILD_BREAKDOWN.md](./BUILD_BREAKDOWN.md) and follow **Step 0** (repo structure, PostgreSQL, Redis).
2. Define the **canonical schema** and create `migrations/001_canonical_schema.sql`.
3. Build the **mock connector** and **effective access** computation (Phase 1).
4. Open [DATA_FLOW.md](./DATA_FLOW.md) and the Excalidraw file to align the team on data flow.

Repository is currently in planning/docs phase. Backend, connectors, and frontend scaffolding can follow the structure in BUILD_BREAKDOWN.
