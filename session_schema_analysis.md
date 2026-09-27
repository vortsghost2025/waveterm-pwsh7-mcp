# Kilo / OpenCode SQLite Session Schema Analysis

Codebase root: `C:\Users\seand\kilocode\packages\opencode`
ORM: Drizzle (`drizzle-orm/sqlite-core`), backed by SQLite. Schema is defined in TypeScript and migrations are generated as `.sql` files.

---

## 1. Session table definition (columns, types, constraints)

### Drizzle source of truth
**File:** `packages/opencode/src/session/session.sql.ts` (lines 14-48)

```ts
export const SessionTable = sqliteTable(
  "session",
  {
    id: text().$type<SessionID>().primaryKey(),
    project_id: text()
      .$type<ProjectID>()
      .notNull()
      .references(() => ProjectTable.id, { onDelete: "cascade" }),
    workspace_id: text().$type<WorkspaceID>(),
    parent_id: text().$type<SessionID>(),
    slug: text().notNull(),
    directory: text().notNull(),
    title: text().notNull(),
    version: text().notNull(),
    share_url: text(),
    summary_additions: integer(),
    summary_deletions: integer(),
    summary_files: integer(),
    // kilocode_change start - lightweight diff type (no file contents)
    summary_diffs: text({ mode: "json" }).$type<
      { file: string; additions: number; deletions: number; status?: "added" | "deleted" | "modified" }[]
    >(),
    // kilocode_change end
    revert: text({ mode: "json" }).$type<{ messageID: MessageID; partID?: PartID; snapshot?: string; diff?: string }>(),
    permission: text({ mode: "json" }).$type<Permission.Ruleset>(),
    ...Timestamps,
    time_compacting: integer(),
    time_archived: integer(),
  },
  (table) => [
    index("session_project_idx").on(table.project_id),
    index("session_workspace_idx").on(table.workspace_id),
    index("session_parent_idx").on(table.parent_id),
  ],
)
```

`Timestamps` (imported from `../storage/schema.sql`) adds `time_created` and `time_updated` (both `integer().notNull()`).

### Column summary

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | TEXT | PRIMARY KEY, `$type<SessionID>` (branded) |
| `project_id` | TEXT | NOT NULL, FK → `project(id)` ON DELETE CASCADE, `$type<ProjectID>` |
| `workspace_id` | TEXT | nullable, `$type<WorkspaceID>` |
| `parent_id` | TEXT | nullable, `$type<SessionID>` (self-referencing parent) |
| `slug` | TEXT | NOT NULL |
| `directory` | TEXT | NOT NULL |
| `title` | TEXT | NOT NULL |
| `version` | TEXT | NOT NULL |
| `share_url` | TEXT | nullable |
| `summary_additions` | INTEGER | nullable |
| `summary_deletions` | INTEGER | nullable |
| `summary_files` | INTEGER | nullable |
| `summary_diffs` | TEXT (JSON) | nullable, serialized diff array |
| `revert` | TEXT (JSON) | nullable, `{messageID, partID?, snapshot?, diff?}` |
| `permission` | TEXT (JSON) | nullable, `Permission.Ruleset` |
| `time_created` | INTEGER | NOT NULL (from `Timestamps`) |
| `time_updated` | INTEGER | NOT NULL (from `Timestamps`) |
| `time_compacting` | INTEGER | nullable |
| `time_archived` | INTEGER | nullable |

Indexes: `session_project_idx` (project_id), `session_workspace_idx` (workspace_id), `session_parent_idx` (parent_id).

### Raw SQL migration (authoritative DDL)
**File:** `packages/opencode/migration/20260127222353_familiar_lady_ursula/migration.sql` (lines 41-61)

```sql
CREATE TABLE `session` (
	`id` text PRIMARY KEY,
	`project_id` text NOT NULL,
	`parent_id` text,
	`slug` text NOT NULL,
	`directory` text NOT NULL,
	`title` text NOT NULL,
	`version` text NOT NULL,
	`share_url` text,
	`summary_additions` integer,
	`summary_deletions` integer,
	`summary_files` integer,
	`summary_diffs` text,
	`revert` text,
	`permission` text,
	`time_created` integer NOT NULL,
	`time_updated` integer NOT NULL,
	`time_compacting` integer,
	`time_archived` integer,
	CONSTRAINT `fk_session_project_id_project_id_fk` FOREIGN KEY (`project_id`) REFERENCES `project`(`id`) ON DELETE CASCADE
);
```

Note: `workspace_id` is added in a later migration (see §2).

---

## 2. SQL migration files / schema definitions

All migrations live under `packages/opencode/migration/`. Those touching the session table:

| Migration file | Lines | Change |
|----------------|-------|--------|
| `20260127222353_familiar_lady_ursula/migration.sql` | 41-61 | Initial `CREATE TABLE session` (+ project, message, part, permission, todo, session_share). Lines 85-90 create indexes `message_session_idx`, `part_message_idx`, `part_session_idx`, `session_project_idx`, `session_parent_idx`, `todo_session_idx`. |
| `20260227213759_add_session_workspace_id/migration.sql` | 1-2 | `ALTER TABLE session ADD workspace_id text;` + `CREATE INDEX session_workspace_idx ON session(workspace_id);` |
| `20260312043431_session_message_cursor/migration.sql` | 1-4 | Drops `message_session_idx` / `part_message_idx`; creates composite `message_session_time_created_id_idx (session_id, time_created, id)` and `part_message_id_id_idx (message_id, id)` for cursor-based pagination. |

Other related tables defined in the initial migration:
- `message` (lines 14-21): `id`, `session_id` FK→session ON DELETE CASCADE, `time_created`, `time_updated`, `data` (JSON).
- `part` (lines 23-31): `id`, `message_id` FK→message CASCADE, `session_id`, timestamps, `data` (JSON).
- `todo` (lines 63-73): composite PK `(session_id, position)`, FK→session CASCADE.
- `session_share` (lines 75-83): `session_id` PK, `id`, `secret`, `url`, timestamps, FK→session CASCADE.

The Drizzle counterparts for these live in `src/session/session.sql.ts` (SessionTable, MessageTable, PartTable, TodoTable, PermissionTable).

---

## 3. How sessions are created, persisted, and queried

**File:** `packages/opencode/src/session/index.ts`

### Create
- `createNext` (lines 446-491): builds an `Info` object with `id: SessionID.descending(input.id)` (sortable ID), `slug: Slug.create()`, `version: Installation.VERSION`, `projectID: Instance.project.id`, `directory`, optional `workspaceID`/`parentID`/`title`/`permission`, and `time: { created, updated }`. Emits `Event.Created` sync event; auto-shares if `KILO_AUTO_SHARE` flag or `cfg.share === "auto"`.
- `create` (lines 599-619): public wrapper that calls `createNext` with `Instance.directory`; optionally stores a platform override via `KiloSession.setPlatformOverride`.
- `fork` (lines 621-655): creates a new session via `createNext`, copies messages/parts up to `messageID` with freshly ascending IDs.

### Persist
- `toRow` (lines 93-115) maps `Info` → DB row (snake_case columns).
- `fromRow` (lines 59-91) maps a DB row → `Info`.
- Writes go through `SyncEvent.run(Event.Created | Event.Updated | Event.Deleted, ...)` which the storage layer persists to SQLite (Drizzle). Patches are applied via `patch` (lines 657-658) → `SyncEvent.run(Event.Updated, ...)`.
- `touch` updates only `time.updated`; `setTitle`, `setArchived`, `setPermission`, `setRevert`, `setSummary`, `clearRevert` all call `patch`.

### Query
- `get` (lines 493-497):
  ```ts
  const row = yield* db((d) => d.select().from(SessionTable).where(eq(SessionTable.id, id)).get())
  if (!row) throw new NotFoundError({ message: `Session not found: ${id}` })
  return fromRow(row)
  ```
- `children` (lines 522-532): selects where `project_id = Instance.project.id AND parent_id = parentID`.
- `list` (lines 854-897): filters by `project_id`, optional `workspace_id`, `directory`, `roots` (parent_id IS NULL), `start` (time_updated >=), `search` (LIKE title); ordered `desc(time_updated)`, limited.
- `listGlobal` (lines 900-994): kilocode change — supports multi-project "family" via `KiloSession.family`, cursor pagination on `time_updated`, optional `directories` (filesystem contains check), joins `project` for display.
- `remove` (lines 534-556): recursively deletes children, calls `KiloSessions.remove`, runs `Event.Deleted` + `SyncEvent.remove`.

Public (promise-based) API wrappers at the bottom of the file (lines 802-1025) wrap the Effect service with zod-validated input (`SessionID.zod`, etc.).

---

## 4. Session ID typing (`SessionID.zod`)

**File:** `packages/opencode/src/session/schema.ts` (lines 7-16)

```ts
import { Schema } from "effect"
import z from "zod"
import { Identifier } from "@/id/id"
import { withStatics } from "@/util/schema"

export const SessionID = Schema.String.pipe(
  Schema.brand("SessionID"),
  withStatics((s) => ({
    make: (id: string) => s.makeUnsafe(id),
    descending: (id?: string) => s.makeUnsafe(Identifier.descending("session", id)),
    zod: Identifier.schema("session").pipe(z.custom<Schema.Schema.Type<typeof s>>()),
  })),
)

export type SessionID = Schema.Schema.Type<typeof SessionID>
```

Key points:
- `SessionID` is an **Effect Schema** branded string (`Schema.String` + `Schema.brand("SessionID")`).
- `.make(id)` / `.descending(id?)` construct IDs. `descending` uses `Identifier.descending("session", id)` to produce a sortable (time-descending) ID — used as the default `id` in `createNext`.
- `.zod` exposes a **zod** validator (`Identifier.schema("session")` piped through `z.custom<...>()`) used for runtime validation of API input (e.g. `src/session/index.ts`, `src/server/routes/session.ts`, `src/session/prompt.ts` all import `SessionID.zod`).
- Related branded IDs in the same file: `MessageID` (lines 18-27, with `.ascending`), `PartID` (lines 29-37, with `.ascending`).

`Identifier.schema` / `Identifier.descending` / `Identifier.ascending` are defined in `packages/opencode/src/id/id.ts` (not separately listed here but referenced via `@/id/id`).

---

## Key file references (absolute paths)

- `C:\Users\seand\kilocode\packages\opencode\src\session\schema.ts` — `SessionID`/`MessageID`/`PartID` branded types + `.zod` (lines 7-37)
- `C:\Users\seand\kilocode\packages\opencode\src\session\session.sql.ts` — Drizzle `SessionTable` + related tables (lines 14-107)
- `C:\Users\seand\kilocode\packages\opencode\src\session\index.ts` — `Session` namespace: create/persist/query logic (lines 1-1026)
- `C:\Users\seand\kilocode\packages\opencode\migration\20260127222353_familiar_lady_ursula\migration.sql` — initial `session` DDL + indexes (lines 41-90)
- `C:\Users\seand\kilocode\packages\opencode\migration\20260227213759_add_session_workspace_id\migration.sql` — `workspace_id` column add (lines 1-2)
- `C:\Users\seand\kilocode\packages\opencode\migration\20260312043431_session_message_cursor\migration.sql` — cursor pagination indexes (lines 1-4)
- `C:\Users\seand\kilocode\packages\opencode\src\id\id.ts` — `Identifier` (descending/ascending/schema) used by `SessionID.zod`
- `C:\Users\seand\kilocode\packages\opencode\src\server\routes\session.ts` — HTTP routes validating `sessionID: SessionID.zod` (many occurrences, e.g. lines 182, 267, 318...)
