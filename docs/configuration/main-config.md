# Main Config (config.yaml)

`config.yaml` is the bootstrap configuration file for the CSOJ system. It carries **only boot facts** — the listen address, storage paths, and the JWT signing secret. Everything else (logger, CORS, local-auth, GitLab OIDC, JWT expiry, and all Kubernetes cluster config including kubeconfig text) lives in the database and is managed at runtime via the admin API.

The judger is Kubernetes-based: each workflow step runs as a Pod (one per step), and MPI steps run as `MPIJob`s (mpi-operator). Runtime data — contests, problems, announcements, assets, node-pool resource caps, nav links, runtime settings, and cluster connection rows — is **stored in the database** and managed via the admin API, not on disk. See [Runtime Settings (database)](#runtime-settings-database) and [Runtime Data Stored in the Database](#runtime-data-stored-in-the-database) below.

## Full Configuration Example

```yaml
listen: ":8080"

storage:
  database: "data/csoj.db"
  user_avatar: "data/avatars"
  submission_content: "data/submissions"   # MUST be a shared RWX PVC mount point (csoj-submissions)
  submission_log: "data/logs"

auth:
  jwt:
    secret: "change-me"        # JWT signing secret — boot fact, restart-required to rotate
    expire_hours: 72           # overridden by the `auth.jwt.expire_hours` setting at runtime
```

> **RWX PVC:** `storage.submission_content` must be a shared `ReadWriteMany` volume mount point. The API server and the judger pods both mount the `csoj-submissions` PVC there — the API server writes user-submitted files into it, and the judger pods read/write the step's working directory (including `/mnt/work/.csoj/result.json`) on the same volume.

> **`auth.jwt.secret` is a boot fact.** It is the only secret that must be present in `config.yaml` before first boot. Rotating it requires editing `config.yaml` and restarting the server. All other auth configuration (`local`, `gitlab`, JWT expiry) is DB-managed and live-reload.

-----

## Runtime Settings (database)

The following settings are stored in the `settings` DB table and managed via `GET /api/v1/admin/settings` + `PUT /api/v1/admin/settings/:key`. Each setting's value is a JSON document. Some settings are live-reload (applied on the next request); others require a restart.

| Key | Value JSON shape | Reload behavior |
|-----|------------------|-----------------|
| `logger` | `{"level":"debug","file":"csoj.log"}` | **Restart-required** to change (logger is built once at boot). |
| `cors` | `{"allowed_origins":["https://oj.example.com"]}` | **Live-reload** — read on every request by the CORS middleware. |
| `auth.local` | `{"enabled":true}` | **Live-reload** — read per request by `/auth/status`, `/auth/local/login`, `/auth/local/register`. A **missing row defaults to enabled** so a fresh install can register the first superadmin; once any value is written, that value is authoritative. |
| `auth.gitlab` | `{"url":"https://gitlab.com","client_id":"...","client_secret":"...","redirect_uri":"...","frontend_callback_url":"..."}` | **Live-reload** — the OIDC provider + OAuth2 client are built lazily per request; if the row is missing or incomplete, GitLab endpoints return `503 gitlab not configured` instead of crashing the server. |
| `auth.jwt.expire_hours` | `72` | **Live-reload** — read at token-issuance time. Falls back to `auth.jwt.expire_hours` from `config.yaml` if unset. |
| `clusters` | (managed via the cluster-row CRUD endpoints, not the settings key/value API — see below) | Use `POST /api/v1/admin/clusters/reload` to rebuild K8s clientsets after row changes. |

### Boot facts returned by `GET /api/v1/admin/settings`

The settings endpoint additionally returns boot-only fields that are **not** writable through `PUT /api/v1/admin/settings/:key`:

- `listen` (from `config.yaml`)
- `storage` (from `config.yaml`)
- `jwt_secret_present` (boolean — whether `auth.jwt.secret` is set in `config.yaml`; the secret itself is never returned)

To rotate `auth.jwt.secret`, edit `config.yaml` and restart the server.

### Clusters (database rows)

Kubernetes clusters are now DB rows managed via the cluster CRUD endpoints (not via `config.yaml`). Each row stores the full kubeconfig **text** (so the judger can build a `kubernetes.Interface` + dynamic client without a file on disk), plus `context`, `namespace`, `concurrency`, and `heartbeat_ttl`.

- `GET /api/v1/admin/clusters` — list cluster rows (kubeconfig text omitted from the response).
- `POST /api/v1/admin/clusters` — create a cluster (body: `{name, kubeconfig, context, namespace, concurrency, heartbeat_ttl}`).
- `PUT /api/v1/admin/clusters/:name` — update a cluster (including kubeconfig text).
- `DELETE /api/v1/admin/clusters/:name` — delete a cluster row.
- `POST /api/v1/admin/clusters/reload` — rebuild the scheduler's in-memory K8s clientsets from the DB without restarting; returns `{warnings: [...]}` for any cluster that failed to initialize.

Per-cluster node-pool caps (`cpu`/`memory`/`node_selector`/`is_paused`) remain DB-managed via the pool endpoints (`PUT /api/v1/admin/clusters/:cluster/pools/:pool`). Pool rows are keyed by `(cluster_name, pool_name)`; the scheduler treats a pool name with no caps row as "skipped".

-----

## Runtime Data Stored in the Database

The following are stored in the database and managed via the admin API — **not** on disk:

- **Runtime settings** (logger, CORS, local-auth, GitLab, JWT expiry) — `GET/PUT /api/v1/admin/settings`
- **Clusters** (kubeconfig text, namespace, concurrency, heartbeat_ttl) — `POST/PUT/DELETE /api/v1/admin/clusters` + `POST /api/v1/admin/clusters/reload`
- **Cluster node-pool caps** (`cpu`/`memory`/`node_selector`/`is_paused`) — `PUT /api/v1/admin/clusters/:c/pools/:p`
- **Contests** — `POST /api/v1/admin/contests`
- **Problems** — `POST /api/v1/admin/contests/:id/problems` (and `PUT /api/v1/admin/contests/:id/problems/order` to reorder)
- **Announcements** — `POST /api/v1/admin/contests/:id/announcements`
- **Assets** (contest/problem static files) — `POST /api/v1/admin/contests/:id/assets` and `POST /api/v1/admin/problems/:id/assets`
- **Nav links** — `POST /api/v1/admin/links`

`config.yaml` carries only the boot fields `listen`, `storage`, and `auth.jwt.secret` (+ the `auth.jwt.expire_hours` fallback).

-----

## Field Reference

### `listen`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: The listen address and port for the user-facing API service.

-----

### `storage`

  - **Type**: `object`
  - **Required**: Yes
  - **Description**: Defines storage paths for various system files.
      - `user_avatar`: (string) Directory to store user-uploaded avatars.
      - `submission_content`: (string) Directory to store user-submitted code/files. **This must be a shared `ReadWriteMany` (RWX) volume mount point** — the API server writes submission files here, and judger pods mount the same `csoj-submissions` PVC (subPath per submission ID) at `/mnt/work`. The dispatcher reads `/mnt/work/.csoj/result.json` from this path after a pod completes.
      - `database`: (string) Path to the SQLite database file. This database holds all admin-managed runtime data (settings, clusters, contests, problems, announcements, assets, cluster node-pool caps, links) in addition to users, submissions, and containers.
      - `submission_log`: (string) Directory to store log files generated by each judging pod.

-----

### `auth.jwt`

  - **Type**: `object`
  - **Required**: Yes
  - **Description**: Bootstrap JWT settings.
      - `secret`: (string) The secret key used to sign and verify JWTs. **You must change this to a complex random string in production.** This is a boot fact — rotating it requires editing `config.yaml` and restarting.
      - `expire_hours`: (integer) Fallback JWT expiration in hours. Overridden at runtime by the `auth.jwt.expire_hours` setting if it is set in the DB.
