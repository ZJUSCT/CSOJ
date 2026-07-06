# Admin API Reference

The Admin API provides a set of powerful endpoints for system maintenance and management. All Admin API routes are mounted under the main CSOJ service and share its listen address (`listen` in `config.yaml`).

All admin-managed data (contests, problems, announcements, assets, links, runtime settings, K8s cluster rows, node-pool caps) is persisted in the database. There are no on-disk `contest.yaml` / `problem.yaml` files to edit — every write goes through the endpoints below and triggers an in-memory `reload` so the running server picks up the change immediately.

The judger is Kubernetes-based: submissions run as Pods (one per workflow step) or, for MPI steps, as `MPIJob`s (mpi-operator). Cluster connection details (`kubeconfig` text, `namespace`, `concurrency`, `heartbeat_ttl`) are stored as rows in the `clusters` DB table and managed via the cluster CRUD endpoints below; per-pool `cpu`/`memory`/`node_selector`/`is_paused` are DB-managed via the pool endpoints below. Logger, CORS, local-auth, GitLab OIDC, and JWT expiry are DB-managed via the settings endpoints below.

## Authentication

All Admin API routes are prefixed with `/api/v1/admin` and require a valid JWT
belonging to a user with the `admin` or `superadmin` role. Send the token in the
`Authorization: Bearer <token>` header. Role-management endpoints
(`PATCH /users/:id/role`) require `superadmin`.

The first user to register (local or GitLab) is automatically granted the
`superadmin` role.

---

### System Management

#### `POST /api/v1/admin/reload`

- **Description**: Hot-reloads all contest and problem state from the database.
  - The system re-reads the `contests`, `problems`, and `announcements` tables and rebuilds the in-memory app state.
  - New or modified contests/problems will be loaded.
  - If a problem has been deleted, any submission records whose problem no longer exists are flagged accordingly (running containers associated with them are cleaned up).
- **Success Response** (`200 OK`):
  ```json
  {
    "code": 0,
    "data": {
      "contests_loaded": 2,
      "problems_loaded": 15
    },
    "message": "Reload successful"
  }
  ```

-----

### User Management

#### `GET /api/v1/admin/users`

  - **Description**: Gets a list of all users. Can be filtered by a `query` parameter that searches User ID, username, and nickname.

#### `POST /api/v1/admin/users`

  - **Description**: Manually creates a new user.
  - **Request Body** (`application/json`):
    ```json
    {
      "username": "admin_created_user",
      "password_hash": "$2a$14$....", // bcrypt hash, required for local auth users
      "nickname": "Test User"
    }
    ```

#### `GET /api/v1/admin/users/:id`

  - **Description**: Gets a single user by their ID.

#### `PATCH /api/v1/admin/users/:id`

  - **Description**: Updates a user's nickname and signature.

#### `DELETE /api/v1/admin/users/:id`

  - **Description**: Deletes a user by their ID.

#### `PATCH /api/v1/admin/users/:id/role`

  - **Description**: Updates a user's role. **Requires the `superadmin` role.**
  - **Request Body** (`application/json`):
    ```json
    {
      "role": "admin"
    }
    ```
  - **Allowed values**: `"admin"`, `"user"`.

#### `POST /api/v1/admin/users/:id/reset-password`

  - **Description**: Resets the password for a local-auth user.
  - **Request Body** (`application/json`): `{"password": "new_secure_password"}`

#### `POST /api/v1/admin/users/:id/register-contest`

  - **Description**: Manually registers a user for a specific contest.
  - **Request Body** (`application/json`): `{"contest_id": "contest-id-here"}`

#### `GET /api/v1/admin/users/:id/history`

  - **Description**: Gets a user's score history for a specific contest.
  - **Query Parameter**: `contest_id` (required).

#### `GET /api/v1/admin/users/:id/scores`

  - **Description**: Gets a user's best scores for all problems they have submitted to.

-----

### Contest & Problem Management

#### `GET /api/v1/admin/contests`

  - **Description**: Gets a list of all loaded contests, regardless of start/end times.

#### `POST /api/v1/admin/contests`

  - **Description**: Creates a new contest record in the database. Triggers a system `reload`.
  - **Request Body**: A full `Contest` JSON object (see [Contest Config](../configuration/contest-config.md)).

#### `GET /api/v1/admin/contests/:id`

  - **Description**: Gets details for a specific contest, regardless of start/end times.

#### `PUT /api/v1/admin/contests/:id`

  - **Description**: Updates the contest record in the database. The `problems` list is preserved (manage it via the problem endpoints below). Triggers a system `reload`.
  - **Request Body**: A full `Contest` JSON object.

#### `DELETE /api/v1/admin/contests/:id`

  - **Description**: Deletes a contest record from the database. Triggers a system `reload`.

#### `POST /api/v1/admin/contests/:id/problems`

  - **Description**: Creates a new problem record in the database and appends its ID to the contest's ordered `problems` list. Triggers a system `reload`.
  - **Request Body**: A full `Problem` JSON object (see [Problem Config](../configuration/problem-config.md)).

#### `PUT /api/v1/admin/contests/:id/problems/order`

  - **Description**: Reorders the problems in a contest. The request must contain the same set of problem IDs as the current list (just reordered); duplicates or foreign IDs are rejected. Triggers a system `reload`.
  - **Request Body** (`application/json`): `{ "problem_ids": ["p1002-fizzbuzz", "p1001-aplusb"] }`

#### `GET /api/v1/admin/problems`

  - **Description**: Gets a list of all loaded problems.

#### `GET /api/v1/admin/problems/:id`

  - **Description**: Gets the full definition of a single problem.

#### `PUT /api/v1/admin/problems/:id`

  - **Description**: Updates the problem record in the database. Triggers a system `reload`.
  - **Request Body**: A full `Problem` JSON object.

#### `DELETE /api/v1/admin/problems/:id`

  - **Description**: Deletes a problem record from the database and removes its ID from the parent contest's `problems` list. Triggers a system `reload`.

-----

### Contest Assets & Announcements

#### `GET /api/v1/admin/contests/:id/assets`

  - **Description**: Lists all static assets for a contest.

#### `POST /api/v1/admin/contests/:id/assets`

  - **Description**: Uploads one or more asset files for a contest. Assets are stored as BLOB rows in the database (no on-disk `index.assets/` directory).

#### `DELETE /api/v1/admin/contests/:id/assets`

  - **Description**: Deletes an asset (file or directory subtree) for a contest.

#### `GET /api/v1/admin/contests/:id/announcements`

  - **Description**: Gets all announcements for a contest.

#### `POST /api/v1/admin/contests/:id/announcements`

  - **Description**: Creates a new announcement for a contest.

#### `PUT /api/v1/admin/contests/:id/announcements/:announcementId`

  - **Description**: Updates an existing announcement.

#### `DELETE /api/v1/admin/contests/:id/announcements/:announcementId`

  - **Description**: Deletes an announcement.

-----

### Problem Assets

#### `GET /api/v1/admin/problems/:id/assets`

  - **Description**: Lists all static assets for a problem.

#### `POST /api/v1/admin/problems/:id/assets`

  - **Description**: Uploads one or more asset files for a problem. Assets are stored as BLOB rows in the database (no on-disk `index.assets/` directory).

#### `DELETE /api/v1/admin/problems/:id/assets`

  - **Description**: Deletes an asset (file or directory subtree) for a problem.

-----

### Submission Management

#### `GET /api/v1/admin/submissions`

  - **Description**: Gets a paginated list of all submissions. Supports filtering by `problem_id`, `status`, and `user_query`. Supports pagination with `page` and `limit`.

#### `GET /api/v1/admin/submissions/:id`

  - **Description**: Gets detailed information for a single submission.

#### `GET /api/v1/admin/submissions/:id/content`

  - **Description**: Downloads the content of a submission as a zip archive.

#### `PATCH /api/v1/admin/submissions/:id`

  - **Description**: Manually updates the `status`, `score`, or `info` field of a submission. **Warning: This does not trigger score recalculation.**

#### `DELETE /api/v1/admin/submissions/:id`

  - **Description**: Permanently deletes a submission record and its content from disk.

#### `POST /api/v1/admin/submissions/:id/rejudge`

  - **Description**: Re-judges an existing submission.
      - The system marks the original submission as invalid (`is_valid: false`).
      - It then copies the original submission's content, creates a new submission record, and adds it to the judging queue.
      - The scoring system automatically handles score changes resulting from the re-judge.

#### `PATCH /api/v1/admin/submissions/:id/validity`

  - **Description**: Manually marks a submission as valid or invalid. This **triggers a full score recalculation** for the user on that problem.
  - **Request Body** (`application/json`): `{"is_valid": false}`

#### `POST /api/v1/admin/submissions/:id/interrupt`

  - **Description**: Forcibly interrupts a queued or running submission, marking it as `Failed`.

#### `GET /api/v1/admin/submissions/:id/containers/:conID/log`

  - **Description**: Gets the full log for any step (container) of any submission, regardless of the `show` flag. The log is returned in NDJSON format.

-----

### Score & Leaderboard Management

#### `POST /api/v1/admin/scores/recalculate`

  - **Description**: Triggers a score recalculation for a specific user on a specific problem.
  - **Request Body** (`application/json`): `{"user_id": "user-uuid", "problem_id": "problem-id"}`

#### `GET /api/v1/admin/contests/:id/leaderboard`

  - **Description**: Gets the leaderboard for a contest.

#### `GET /api/v1/admin/contests/:id/trend`

  - **Description**: Gets score trend data for top users. Supports a `maxnum` query parameter to control the number of users.

-----

### Runtime Settings

Runtime settings (logger, CORS, local-auth, GitLab OIDC, JWT expiry) are stored as JSON-encoded rows in the `settings` DB table. The `logger` setting is **restart-required** to change (the logger is built once at boot); all others are **live-reload** (read on the next request).

#### `GET /api/v1/admin/settings`

  - **Description**: Lists all settings rows (key → JSON value) plus boot-only facts from `config.yaml` that are not writable through this endpoint.
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": {
        "settings": {
          "logger": "{\"level\":\"debug\",\"file\":\"csoj.log\"}",
          "cors": "{\"allowed_origins\":[\"https://oj.example.com\"]}",
          "auth.local": "{\"enabled\":true}",
          "auth.gitlab": "{\"url\":\"https://gitlab.com\",\"client_id\":\"...\",\"client_secret\":\"...\",\"redirect_uri\":\"...\",\"frontend_callback_url\":\"...\"}",
          "auth.jwt.expire_hours": "72"
        },
        "boot": {
          "listen": ":8080",
          "storage": {
            "database": "data/csoj.db",
            "user_avatar": "data/avatars",
            "submission_content": "data/submissions",
            "submission_log": "data/logs"
          },
          "jwt_secret_present": true
        }
      },
      "message": "Settings retrieved"
    }
    ```
      - `settings`: map of key → JSON value (the raw stored string). A missing key means "no row written" — for `auth.local`, that means "default to enabled" (bootstrapping-friendly).
      - `boot.listen` / `boot.storage`: echoed from `config.yaml` for display. Rotate `auth.jwt.secret` by editing `config.yaml` and restarting.
      - `boot.jwt_secret_present`: boolean. The secret itself is never returned.

#### `PUT /api/v1/admin/settings/:key`

  - **Description**: Writes a settings value (JSON-encodes the supplied `value`). Most keys are live-reload; `logger` is restart-required (the response flags it).
  - **Path Parameter**: `:key` — the settings key. Known keys: `logger`, `cors`, `auth.local`, `auth.gitlab`, `auth.jwt.expire_hours`.
  - **Request Body** (`application/json`):
    ```json
    { "value": {"allowed_origins": ["https://oj.example.com"]} }
    ```
      - `value`: (any) The JSON-encodable value to store. For `cors` it is `{"allowed_origins": [...]}`; for `auth.local` it is `{"enabled": true|false}`; for `auth.gitlab` it is `{"url":"...","client_id":"...","client_secret":"...","redirect_uri":"...","frontend_callback_url":"..."}`; for `auth.jwt.expire_hours` it is an integer; for `logger` it is `{"level":"debug|production","file":"..."}`.
  - **Success Response**: `{"restart_required": false}` (or `true` when `:key` is `logger`).
  - **Notes**:
      - `cors`, `auth.local`, `auth.gitlab`, `auth.jwt.expire_hours` are live-reload — the next request reads the new value.
      - `logger` requires a restart to take effect (the response still confirms the write).
      - A missing `auth.local` row defaults to enabled (so a fresh install can register the first superadmin). Once any value is written, that value is authoritative.
      - A missing or incomplete `auth.gitlab` row makes the GitLab endpoints return `503 gitlab not configured` (the server does not crash at boot).

-----

### Cluster Management

Cluster rows are stored in the `clusters` DB table. Each row carries the full kubeconfig **text** (so the judger can build a `kubernetes.Interface` + dynamic client without a kubeconfig file on disk), plus `context`, `namespace`, `concurrency`, and `heartbeat_ttl`. Changes to cluster rows are picked up by calling `POST /api/v1/admin/clusters/reload` (no server restart required).

#### `GET /api/v1/admin/clusters`

  - **Description**: Lists all cluster rows. The `kubeconfig` text is **omitted** from the response (it may contain secrets).
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": [
        {
          "name": "gpu-cluster",
          "context": "",
          "namespace": "csoj-judger",
          "concurrency": 4,
          "heartbeat_ttl": 30
        }
      ],
      "message": "Clusters retrieved"
    }
    ```
      - `heartbeat_ttl`: integer seconds (the HA grace period for judger failover; the judger writes a heartbeat row every `ttl/2`, and on restart waits up to `ttl` for a prior instance's heartbeat to expire before claiming the cluster).

#### `POST /api/v1/admin/clusters`

  - **Description**: Creates a new cluster row (upserts if the name already exists). After creating, call `POST /api/v1/admin/clusters/reload` to rebuild the scheduler's in-memory clientsets without restarting.
  - **Request Body** (`application/json`):
    ```json
    {
      "name": "gpu-cluster",
      "kubeconfig": "<full kubeconfig YAML text>",
      "context": "",
      "namespace": "csoj-judger",
      "concurrency": 4,
      "heartbeat_ttl": 30
    }
    ```
      - `name`: (string, required) Unique cluster name. Used in problem configs to specify which cluster to use for judging.
      - `kubeconfig`: (string, required) The full kubeconfig YAML text. The judger parses it via `clientcmd.Load` at boot/reload — no kubeconfig file path is needed on the API server's disk.
      - `context`: (string, optional) The kubeconfig context to use. If empty, the kubeconfig's current context is used.
      - `namespace`: (string) The Kubernetes namespace in which judger Pods and MPIJobs are created. The `csoj-submissions` RWX PVC must exist in this namespace.
      - `concurrency`: (integer) Max in-flight submissions for this cluster (per-cluster semaphore; resizing requires a restart).
      - `heartbeat_ttl`: (integer, seconds) HA grace period for judger failover.

#### `PUT /api/v1/admin/clusters/:name`

  - **Description**: Updates an existing cluster row (including the `kubeconfig` text — e.g. to rotate credentials). The `name` in the path must match the `name` in the body. Call `POST /api/v1/admin/clusters/reload` afterward to apply.
  - **Request Body**: same shape as `POST /api/v1/admin/clusters`.

#### `DELETE /api/v1/admin/clusters/:name`

  - **Description**: Deletes a cluster row by name. In-flight work on the cluster is not interrupted; call `POST /api/v1/admin/clusters/reload` to remove it from the scheduler's in-memory map.

#### `POST /api/v1/admin/clusters/reload`

  - **Description**: Rebuilds the scheduler's in-memory K8s clientsets from the `clusters` DB table without restarting the server. Preserves the existing per-cluster queue for clusters that still exist (so in-flight work continues). Clusters that failed to initialize are skipped and reported in `warnings`.
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": { "warnings": ["cluster bad-cluster: invalid kubeconfig: ..."] },
      "message": "Clusters reloaded"
    }
    ```

#### `GET /api/v1/admin/clusters/status`

  - **Description**: Gets the current resource status and queue lengths for all configured clusters. The response contains two top-level fields:
      - `resource_status`: per-cluster snapshot, keyed by cluster name. Each entry includes:
          - `name`, `namespace`
          - `pools`: map of pool name → `{name, node_selector, cpu, memory, is_paused}` (caps from the DB; `cpu`/`memory` of `0` means "no caps set — skipped by scheduler").
          - `mpi_enabled`: (boolean) whether the mpi-operator CRD was detected in this cluster.
          - `queue_length`: (integer) number of submissions waiting in this cluster's queue.
          - `concurrency`: (integer) the configured max in-flight submissions (semaphore capacity).
      - `queue_lengths`: map of cluster name → queue length (same value as `queue_length` above, provided separately for convenience).
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": {
        "resource_status": {
          "gpu-cluster": {
            "name": "gpu-cluster",
            "namespace": "csoj-judger",
            "pools": {
              "gpu-pool": {"name": "gpu-pool", "node_selector": {"pool": "gpu"}, "cpu": 8, "memory": 16384, "is_paused": false},
              "cpu-pool": {"name": "cpu-pool", "node_selector": {}, "cpu": 0, "memory": 0, "is_paused": false}
            },
            "mpi_enabled": true,
            "queue_length": 0,
            "concurrency": 4
          }
        },
        "queue_lengths": {"gpu-cluster": 0}
      },
      "message": "Cluster status retrieved"
    }
    ```

#### `GET /api/v1/admin/clusters/:cluster/pools`

  - **Description**: Lists all node-pools for a cluster (from the database).
  - **Success Response**: an array of pool objects, each `{cluster_name, pool_name, node_selector, cpu, memory, is_paused, created_at, updated_at}`.

#### `POST /api/v1/admin/clusters/:cluster/pools`

  - **Description**: Creates a new node-pool record for the cluster.
  - **Request Body** (`application/json`):
    ```json
    {
      "pool_name": "gpu-pool",
      "cpu": 8,
      "memory": 16384,
      "node_selector": {"pool": "gpu"},
      "is_paused": false
    }
    ```
      - `pool_name`: (string, required) The pool name.
      - `cpu`: (integer, required, positive) Total CPU cores the scheduler may use on this pool.
      - `memory`: (integer, required, positive) Total memory (in MB) the scheduler may use on this pool.
      - `node_selector`: (object, optional) Kubernetes `nodeSelector` labels applied to judger pods scheduled on this pool. Defaults to `{}`.
      - `is_paused`: (boolean, optional) Whether the pool should skip new tasks. Defaults to `false`.

#### `PUT /api/v1/admin/clusters/:cluster/pools/:pool`

  - **Description**: Updates a node-pool's `cpu`/`memory`/`node_selector`/`is_paused`. Values are persisted to the `cluster_node_pools` table and applied to the running scheduler in place (no restart required). **Requires the `admin` or `superadmin` role.**
  - **Request Body** (`application/json`):
    ```json
    {
      "cpu": 4,
      "memory": 8192,
      "node_selector": {"pool": "gpu"},
      "is_paused": false
    }
    ```
      - `cpu`: (integer, required, positive) Total CPU cores the scheduler may use on this pool.
      - `memory`: (integer, required, positive) Total memory (in MB) the scheduler may use on this pool.
      - `node_selector`: (object, optional) Kubernetes `nodeSelector` labels.
      - `is_paused`: (boolean, optional) Whether the pool should skip new tasks.

#### `DELETE /api/v1/admin/clusters/:cluster/pools/:pool`

  - **Description**: Deletes a node-pool record from the database. The pool will no longer be considered by the scheduler (with no caps it is skipped).

#### `PUT /api/v1/admin/clusters/:cluster/concurrency`

  - **Description**: Sets the cluster's max in-flight submissions. Concurrency is implemented as a fixed-capacity semaphore; **resizing requires a restart**. This endpoint validates and stores the requested value (use it together with a restart to resize).
  - **Request Body** (`application/json`): `{"concurrency": 8}`
  - **Success Response**: `{"cluster": "gpu-cluster", "concurrency": 8}` with message `"Concurrency updated (restart to resize)"`.

#### `GET /api/v1/admin/containers`

  - **Description**: Gets a paginated list of all containers (one per workflow step / pod). Supports filtering by `submission_id`, `status`, and `user_query`.

#### `GET /api/v1/admin/containers/:id`

  - **Description**: Gets details for a single container.

-----

### Nav Links

Nav links (the entries in the frontend navigation bar) are stored in the database and managed via the endpoints below. The user-facing `GET /api/v1/links` reads the same rows.

#### `GET /api/v1/admin/links`

  - **Description**: Lists all nav links, ordered by `position` ascending.

#### `POST /api/v1/admin/links`

  - **Description**: Creates a new nav link. If `position` is omitted, the link is appended to the end of the list.
  - **Request Body** (`application/json`):
    ```json
    {
      "name": "Project Source",
      "url": "https://github.com/ZJUSCT/CSOJ",
      "position": 0
    }
    ```
    - `name`: (string, required) The display text for the link.
    - `url`: (string, required) The destination URL. Can be an internal path (e.g., `/about`) or an external URL.
    - `position`: (integer, optional) Sort order. Lower values appear earlier. Defaults to the end of the list.

#### `PUT /api/v1/admin/links/:id`

  - **Description**: Updates an existing nav link. All three fields in the body are written through.
  - **Request Body** (`application/json`): `{ "name": "...", "url": "...", "position": 1 }`

#### `DELETE /api/v1/admin/links/:id`

  - **Description**: Deletes a nav link.

-----

### WebSocket

#### `GET /api/v1/admin/ws/submissions/:id/containers/:conID/logs`

  - **Description**: Establishes a WebSocket connection to stream the complete log for any container. For finished containers, it streams the saved log file. For running containers, it first sends all historical logs from the cache and then continues to stream new logs in real-time. This is available regardless of the `show` flag.
  - **Authentication**: Requires a valid admin JWT (passed as a query parameter or header, depending on the client).
