# Main Config (config.yaml)

`config.yaml` is the primary configuration file for the CSOJ system. It defines the bootstrap behavior of the service: listen addresses, storage paths, authentication methods, and the judger cluster's Kubernetes connections.

The judger is Kubernetes-based: each workflow step runs as a Pod (one per step), and MPI steps run as `MPIJob`s (mpi-operator). Runtime data — contests, problems, announcements, assets, node-pool resource caps, and nav links — is **stored in the database** and managed via the admin API, not on disk. See [Runtime Data Stored in the Database](#runtime-data-stored-in-the-database) below.

## Full Configuration Example

```yaml
# Listen address for the user-facing API
listen: ":8080"

# Logger configuration
logger:
  level: "debug" # Options: "debug", "production"
  file: ""       # Path to log file. If empty, logs to console.

# Storage path configuration
storage:
  user_avatar: "data/avatars"            # User avatars
  submission_content: "data/submissions" # User-submitted files (must be a shared RWX PVC mount — see below)
  database: "data/csoj.db"               # SQLite database file
  submission_log: "data/logs"           # Logs from judging containers

# Authentication configuration
auth:
  jwt:
    secret: "a_very_secret_key_change_me" # JWT signing secret, MUST be changed
    expire_hours: 72                     # JWT expiration time in hours

  # Local username/password authentication
  local:
    enabled: true

  # GitLab OAuth2 authentication
  gitlab:
    url: "https://gitlab.com"
    client_id: "YOUR_GITLAB_CLIENT_ID"
    client_secret: "YOUR_GITLAB_CLIENT_SECRET"
    redirect_uri: "http://localhost:8080/api/v1/auth/gitlab/callback"
    frontend_callback_url: "http://localhost:3000/callback" # URL for frontend to handle the final redirect with the token

# Cross-Origin Resource Sharing (CORS) configuration
cors:
  allowed_origins:
    - "http://localhost:3000"
    - "http://127.0.0.1:3000"

# Judger cluster configuration (Kubernetes)
cluster:
  - name: "gpu-cluster"                        # Cluster name, referenced in problem configs
    kubeconfig: "/etc/csoj/kubeconfigs/gpu.yaml" # Path to a kubeconfig file for this cluster
    context: ""                                 # Optional kubeconfig context (empty = current)
    namespace: "csoj-judger"                    # Namespace where judger pods run
    concurrency: 4                              # Max in-flight submissions for this cluster
    heartbeat_ttl: "30s"                        # HA grace period for judger failover
    node_pools:                                 # Declared node-pool names (caps are DB-managed)
      - name: "gpu-pool"
      - name: "cpu-pool"
```

> **Note:** Node-pool `cpu`/`memory`/`node_selector` are managed at runtime via `PUT /api/v1/admin/clusters/:c/pools/:p` and stored in the database. Adding a new node-pool name requires editing `config.yaml` and restarting — the Kubernetes connection is cluster-level (one kubeconfig per cluster), not per-pool. Use `PUT /api/v1/admin/clusters/:c/concurrency` to set concurrency (restart required to resize).
>
> **RWX PVC:** `storage.submission_content` must be a shared `ReadWriteMany` volume mount point. The API server and the judger pods both mount the `csoj-submissions` PVC there — the API server writes user-submitted files into it, and the judger pods read/write the step's working directory (including `/mnt/work/.csoj/result.json`) on the same volume.

-----

## Runtime Data Stored in the Database

The following are stored in the database and managed via the admin API — **not** on disk:

- **Contests** — `POST /api/v1/admin/contests`
- **Problems** — `POST /api/v1/admin/contests/:id/problems` (and `PUT /api/v1/admin/contests/:id/problems/order` to reorder)
- **Announcements** — `POST /api/v1/admin/contests/:id/announcements`
- **Assets** (contest/problem static files) — `POST /api/v1/admin/contests/:id/assets` and `POST /api/v1/admin/problems/:id/assets`
- **Cluster node-pool caps** (`cpu`/`memory`/`node_selector`/`is_paused`) — `PUT /api/v1/admin/clusters/:c/pools/:p`
- **Nav links** — `POST /api/v1/admin/links`

`config.yaml` carries only bootstrap fields (`listen`, `logger`, `storage`, `auth`, `cors`) and per-cluster Kubernetes connection fields under `cluster` (`kubeconfig`, `context`, `namespace`, `concurrency`, `heartbeat_ttl`, declared `node_pools` names).

-----

## Field Reference

### `listen`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: The listen address and port for the user-facing API service.

-----

### `logger`

  - **Type**: `object`
  - **Required**: Yes
  - **Description**: Configuration for the logging system.
      - `level`: (string) Log level. `debug` provides more verbose output, while `production` is more concise.
      - `file`: (string) Path to a log file. If left empty, logs are written to standard output/error (the console).

-----

### `storage`

  - **Type**: `object`
  - **Required**: Yes
  - **Description**: Defines storage paths for various system files.
      - `user_avatar`: (string) Directory to store user-uploaded avatars.
      - `submission_content`: (string) Directory to store user-submitted code/files. **This must be a shared `ReadWriteMany` (RWX) volume mount point** — the API server writes submission files here, and judger pods mount the same `csoj-submissions` PVC (subPath per submission ID) at `/mnt/work`. The dispatcher reads `/mnt/work/.csoj/result.json` from this path after a pod completes.
      - `database`: (string) Path to the SQLite database file. This database holds all admin-managed runtime data (contests, problems, announcements, assets, cluster node-pool caps, links) in addition to users, submissions, and containers.
      - `submission_log`: (string) Directory to store log files generated by each judging pod.

-----

### `auth`

  - **Type**: `object`
  - **Required**: Yes
  - **Description**: User authentication settings.
      - `jwt`: (object)
          - `secret`: (string) The secret key used to sign and verify JWTs. **You must change this to a complex random string in production.**
          - `expire_hours`: (integer) The validity period of a JWT, in hours.
      - `local`: (object)
          - `enabled`: (boolean) Whether to enable the local username and password registration/login feature.
      - `gitlab`: (object)
          - `url`: (string) The URL of your GitLab instance's OIDC provider.
          - `client_id`: (string) The Client ID obtained after creating an application in GitLab.
          - `client_secret`: (string) The Client Secret obtained after creating an application in GitLab.
          - `redirect_uri`: (string) The callback URL configured in your GitLab application, which must exactly match this URI.
          - `frontend_callback_url`: (string) The URL on your frontend application where users are redirected after a successful login. The JWT will be appended as a `?token=` query parameter.

-----

### `cors`

  - **Type**: `object`
  - **Required**: No
  - **Description**: Configures Cross-Origin Resource Sharing (CORS) for the API.
      - `allowed_origins`: (array of strings) A list of origins that are allowed to access the API. You can add your frontend application's address here. Supports `*` as a wildcard.

-----

### `cluster`

  - **Type**: `array of objects`
  - **Required**: Yes
  - **Description**: Defines one or more judger clusters. Each cluster maps to a single Kubernetes cluster (one kubeconfig). Judger pods run in the configured `namespace`; submissions run as Pods (one per workflow step) or, when an `mpi` step is enabled, as `MPIJob`s (mpi-operator). Only the cluster-level connection and the declared node-pool names are configured here — the per-pool `cpu`/`memory`/`node_selector` caps are managed at runtime via the admin API and stored in the database.
      - `name`: (string) A unique name for the cluster. This name is used in problem configurations to specify which cluster to use for judging.
      - `kubeconfig`: (string) Path to a kubeconfig file for this cluster. The judger uses it to build a `kubernetes.Interface` + dynamic client (for the MPIJob CRD).
      - `context`: (string, optional) The kubeconfig context to use. If empty, the kubeconfig's current context is used.
      - `namespace`: (string) The Kubernetes namespace in which judger Pods and MPIJobs are created. The `csoj-submissions` RWX PVC must exist in this namespace.
      - `concurrency`: (integer) The maximum number of in-flight submissions for this cluster. Implemented as a per-cluster semaphore; resizing requires a restart (use `PUT /api/v1/admin/clusters/:c/concurrency` to set the value, then restart).
      - `heartbeat_ttl`: (duration string, e.g. `"30s"`) The HA grace period. The judger writes a heartbeat row to the database every `ttl/2`; on restart, it waits up to `ttl` for a prior instance's heartbeat to expire before claiming the cluster and recovering (deleting leftover judger pods/MPIJobs and marking `Running` submissions `Failed`).
      - `node_pools`: (array of objects) The list of node-pool names declared for this cluster. Each entry has a single field:
          - `name`: (string) A unique name for the pool. Pool caps (`cpu`/`memory`/`node_selector`/`is_paused`) are not set here — set them via `PUT /api/v1/admin/clusters/:c/pools/:p` after boot. A pool with no caps set is skipped by the scheduler.
