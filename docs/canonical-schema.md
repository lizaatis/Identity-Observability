# Canonical Schema — Approach & Model

Use this when defining and evolving the canonical schema and migrations.

---

## Approach (how to define it)

1. **List core entities**  
   One table per concept: who (Identity), groupings (Group), privilege level (Role), entitlement (Permission). Optional: Resource for MVP.

2. **Every entity is source-aware**  
   For Group, Role, Permission: always store `source_system` and `source_id` so you can trace back to the IdP and upsert idempotently by `(source_system, source_id)`.

3. **Identity is canonical; sources are linked**  
   One row per **person** in `identities` (merged view). Use `identity_sources` to link that person to each IdP’s user id and per-source status (for disabled drift).

4. **Relationship tables carry source**  
   `identity_group`, `identity_role`, `group_role`, `role_permission` store the many-to-many link **and** `source_system` / `source_id` so you know where the assignment came from and can sync incrementally.

5. **Migrations are additive and reversible**  
   One file per change (e.g. `001_canonical_schema.sql`). Prefer `CREATE TABLE` and later `ALTER`; avoid dropping columns until you have a proper migration strategy.

6. **Index what you query**  
   Foreign keys, `(source_system, source_id)` for upserts, and any columns used in effective-access or risk queries (e.g. `identity_id`, `privilege_level`).

---

## Model (summary)

| Table | Purpose |
|-------|--------|
| **identities** | Canonical person (one row per human). `employee_id`, `email`, `display_name`, effective `status`. |
| **identity_sources** | Links canonical identity to each IdP: `identity_id`, `source_system`, `source_user_id`, `source_status`, `confidence`, `synced_at`. |
| **groups** | Group in a source system. `name`, `source_system`, `source_id`, `synced_at`. |
| **roles** | Role in a source. `name`, `privilege_level` (e.g. admin, read), `source_system`, `source_id`, `synced_at`. |
| **permissions** | Permission/entitlement. `name`, `resource_type`, `source_system`, `source_id`, `synced_at`. |
| **identity_group** | Identity ↔ Group membership. `identity_id`, `group_id`, `source_system`, `source_id`, `synced_at`. |
| **identity_role** | Identity ↔ Role assignment. Same pattern. |
| **group_role** | Group ↔ Role. Same pattern. |
| **role_permission** | Role ↔ Permission. Same pattern. |

All relationship tables keep `source_system` and `source_id` for lineage and idempotent sync.

---

## Order of creation

1. **identities** (no FK to others).  
2. **identity_sources** (FK to identities).  
3. **groups**, **roles**, **permissions** (no FK between them).  
4. **identity_group**, **identity_role**, **group_role**, **role_permission** (FKs to the entities above).

Run `migrations/001_canonical_schema.sql` against your database to create these tables.
