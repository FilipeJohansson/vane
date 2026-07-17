# fullstack-app

A bigger Vane example: a wasm frontend (login, register, dashboard, users) talking
to a real backend API over HTTP. Shows Vane in a shape closer to a real app -
auth, protected routes, a `Layout` shell, forms, and data fetched from a server.

## Architecture

Two separate Go processes:

- **`api/`** - a plain `net/http` backend, no wasm build tag, its own `main` package.
  In-memory user store (`api/store.go`) standing in for a real database, bcrypt
  password hashing, opaque bearer tokens, and a small REST API:

  | Route | Auth | Purpose |
  |---|---|---|
  | `POST /api/register` | - | create account, returns `{token, user}` |
  | `POST /api/login` | - | returns `{token, user}` |
  | `POST /api/logout` | bearer | revokes the token |
  | `GET /api/me` | bearer | current user |
  | `GET /api/users` | bearer | list all users |
  | `GET /api/users/{id}` | bearer | one user |
  | `PATCH /api/users/{id}` | bearer, self only | update your own name |
  | `DELETE /api/users/{id}` | bearer, self only | delete your own account (cascades to their notes) |
  | `GET /api/stats` | bearer | total users + member-since |
  | `GET /api/users/{id}/notes` | bearer | list a user's notes |
  | `POST /api/users/{id}/notes` | bearer, self only | create a note for yourself |
  | `PATCH /api/notes/{id}` | bearer, note owner only | edit a note |
  | `DELETE /api/notes/{id}` | bearer, note owner only | delete a note |

  Notes are a one-to-many relation (`User` → `[]Note`) standing in for whatever
  "user has many X" a real app would model - a small dose of relational shape
  on top of the flat `users` resource.

- **root (`App.vane`, `main.go`, `src/`)** - the Vane wasm frontend. `src/api/client.go`
  is a typed HTTP client for the routes above; `src/store/auth.go` holds the
  session (token + current user) as package-level signals, persisted to
  `localStorage` so a page refresh doesn't log you out.

The frontend authenticates with a **bearer token** (returned in the JSON body on
login/register, sent back as `Authorization: Bearer <token>`) rather than
cookies - that sidesteps CORS credentials complexity entirely, since the API
runs on a different port than the frontend dev server.

## Run

Two terminals, from the repo root:

```powershell
# terminal 1 - backend API on :8081
go run ./examples/fullstack-app/api

# terminal 2 - wasm frontend on :8080
.\vane.exe run examples/fullstack-app
# open http://localhost:8080
```

Register an account, you'll land on `/dashboard`. `/dashboard/users` lists every
registered account; opening your own row lets you rename or delete your
account, opening someone else's shows a read-only view. Each user has their own
notes at `/dashboard/users/:id/notes` - editable only by their owner, visible
(read-only) to anyone signed in - and the dashboard's "Recent notes" widget
shows your own latest three.

## Where to look

| Concern | File |
|---|---|
| Route guard (redirect to `/login` if logged out) | [`src/layouts/DashboardShell.vane`](src/layouts/DashboardShell.vane) |
| Session persistence across reloads | [`src/store/auth.go`](src/store/auth.go) `RestoreSession` |
| Reusable controlled `<input>` without losing focus on re-render | [`src/components/ui/Field.vane`](src/components/ui/Field.vane) |
| Fetching from a goroutine into signals | [`src/pages/Dashboard.vane`](src/pages/Dashboard.vane), [`src/pages/Users.vane`](src/pages/Users.vane) |
| Reading `router.Params()` during setup, using it inside `Effect` | [`src/pages/UserDetail.vane`](src/pages/UserDetail.vane) |
| Same route pattern reused across different `:id` values (no remount) | [`src/pages/UserNotes.vane`](src/pages/UserNotes.vane) |
| CORS + bearer-token middleware | [`api/middleware.go`](api/middleware.go) |
| In-memory repository standing in for a DB, one-to-many relation + cascade delete | [`api/store.go`](api/store.go) |
| Per-resource ownership checks (not just per-route auth) | [`api/notes.go`](api/notes.go) |
