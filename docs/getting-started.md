# Getting Started

This document will guide you through compiling and running the CSOJ backend service.

## 1. Prerequisites

- **Go**: Version `1.20` or higher is recommended.
- **Node.js**: Version `20` or higher, with `pnpm` installed.
- **Kubernetes**: A Kubernetes cluster (a local `kind` or `minikube` cluster is fine for development) with:
  - A namespace for judger pods (e.g. `csoj-judger`).
  - A `ReadWriteMany` (RWX) `PersistentVolumeClaim` named `csoj-submissions` in that namespace. The API server and judger pods both mount this PVC (the API server at `storage.submission_content`, judger pods at `/mnt/work`).
  - **Optional:** the [mpi-operator](https://github.com/kubeflow/mpi-operator) CRD (`kubeflow.org/v2beta1` MPIJob), only if you plan to run MPI problems. Non-MPI problems do not require it.

## 2. Compile the Project

After cloning the project repository, execute the following command in the project root to build the `CSOJ` executable (this also builds and embeds the frontend):

```bash
make build
```

This will generate an executable file named `CSOJ` in the project root directory.

The first registered user becomes the superadmin.

## 3. Prepare Configuration Files

The core of CSOJ is its configuration. You will need at least one main configuration file.

1.  Create a `configs` folder in the project root.
2.  Create a `config.yaml` file inside the `configs` folder.

Here is a minimal example of `config.yaml` — it contains **only boot facts** (listen address, storage paths, and the JWT signing secret). Everything else (logger, CORS, local-auth, GitLab OIDC, JWT expiry, and all Kubernetes cluster config) is stored in the database and managed at runtime via the admin API:

```yaml
# configs/config.yaml

listen: ":8080" # Listen address for the user-facing API service

storage:
  database: "data/csoj.db"                # Path to the SQLite database file
  user_avatar: "data/avatars"            # Directory for user avatars
  submission_content: "data/submissions"  # MUST be a shared RWX PVC mount point (csoj-submissions)
  submission_log: "data/logs"             # Directory for judger logs

auth:
  jwt:
    secret: "a_very_secret_key_change_me" # JWT signing secret, MUST be changed (boot fact)
    expire_hours: 72                      # overridden by the `auth.jwt.expire_hours` setting at runtime
```

For more details on configuration, please refer to the **[Configuration Guides](./configuration/main-config.md)**.

> **Note:** Contests, problems, and other runtime data are managed via the admin API and stored in the database. There is no `contests/` directory on disk — all contest and problem definitions live in the database.

> **After first boot** (the first registered user becomes superadmin), configure logger, CORS, GitLab OIDC, and K8s clusters via the admin panel — not via `config.yaml`:
> - `GET /api/v1/admin/settings` / `PUT /api/v1/admin/settings/:key` — runtime settings (logger, cors, auth.local, auth.gitlab, auth.jwt.expire_hours).
> - `GET/POST/PUT/DELETE /api/v1/admin/clusters` — K8s cluster rows (upload the kubeconfig **text**; the judger parses it at boot/reload — no kubeconfig file path needed).
> - `POST /api/v1/admin/clusters/reload` — rebuild the scheduler's in-memory K8s clientsets after changing cluster rows (no restart required).
>
> The `auth.local` setting defaults to **enabled** when the row is missing, so a fresh install can register the first superadmin. Once any value is written (e.g. `{"enabled":false}`), that value is authoritative and live-reloads on the next request.

## 4. Run the Service

Once the above steps are complete, you can start the CSOJ service with the following command:

```bash
# The -c flag specifies the path to the main configuration file
./CSOJ -c configs/config.yaml
```

If everything is configured correctly, you should see output similar to this in your console:

```
ZJUSCT CSOJ dev-build - Fully Containerized Secure Online Judgement

{"level":"info","ts":...,"caller":"...","msg":"database initialized successfully"}
{"level":"info","ts":...,"caller":"...","msg":"successfully recovered and cleaned up interrupted tasks"}
{"level":"info","ts":...,"caller":"...","msg":"loaded 0 contests and 0 problems"}
{"level":"info","ts":...,"caller":"...","msg":"judger scheduler started"}
{"level":"info","ts":...,"caller":"...","msg":"starting user server at :8080"}
```

## 5. First-Boot Setup

After the server starts, complete the initial setup:

1. **Register the first user** — the first user to register (via the frontend or `POST /api/v1/auth/local/register`) is automatically granted the `superadmin` role. (Local auth defaults to **enabled** when the `auth.local` setting row is missing — once you write any value to it, that value is authoritative.)
2. **Configure runtime settings** — set logger, CORS, GitLab OIDC, and JWT expiry via the admin API:
   ```bash
   # Example: enable CORS for the frontend origin
   curl -X PUT /api/v1/admin/settings/cors \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{"value":{"allowed_origins":["http://localhost:3000"]}}'
   ```
   The `logger` setting is **restart-required** to change; `cors`, `auth.local`, `auth.gitlab`, and `auth.jwt.expire_hours` are **live-reload**.
3. **Add a Kubernetes cluster** — clusters are now DB rows managed via the admin API (not `config.yaml`). Upload the kubeconfig **text** (the judger parses it at boot/reload — no kubeconfig file path on the API server's disk):
   ```bash
   curl -X POST /api/v1/admin/clusters \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "default-cluster",
       "kubeconfig": "<full kubeconfig YAML text>",
       "context": "",
       "namespace": "csoj-judger",
       "concurrency": 4,
       "heartbeat_ttl": 30
     }'
   # Then rebuild the scheduler's in-memory clientsets (no restart required):
   curl -X POST /api/v1/admin/clusters/reload \
     -H "Authorization: Bearer <token>"
   ```
   `heartbeat_ttl` is an integer number of seconds.
4. **Set node-pool resource caps** — a pool's `cpu`/`memory`/`node_selector` are not in `config.yaml`. Set them via the admin API so the scheduler will accept tasks for the pool:
   ```bash
   curl -X PUT /api/v1/admin/clusters/default-cluster/pools/default-pool \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{"cpu": 4, "memory": 4096, "node_selector": {}}'
   ```
5. **Create contests and problems** — use the admin UI or the admin REST API (`POST /api/v1/admin/contests`, then `POST /api/v1/admin/contests/:id/problems`). See [Contest Config](./configuration/contest-config.md) and [Problem Config](./configuration/problem-config.md) for the JSON shapes, and the [Admin API Reference](./api-reference/admin-api.md) for the full endpoint list.

The CSOJ backend service is now running at `http://localhost:8080`.
