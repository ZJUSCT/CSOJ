# Getting Started

This document will guide you through compiling and running the CSOJ backend service.

## 1. Prerequisites

- **Go**: Version `1.20` or higher is recommended.
- **Node.js**: Version `20` or higher, with `pnpm` installed.
- **Docker**: The Docker service must be installed and running on the judger nodes. CSOJ communicates with the Docker Daemon via a TCP socket.

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

Here is a minimal example of `config.yaml`:

```yaml
# configs/config.yaml

listen: ":8080" # Listen address for the user-facing API service

logger:
  level: "debug" # Log level (can be "debug" or "production")
  file: "csoj.log" # (Optional) Path to log file.

storage:
  database: "data/csoj.db" # Path to the SQLite database file
  user_avatar: "data/avatars" # Directory for user avatars
  submission_content: "data/submissions" # Directory for user submission content
  submission_log: "data/logs" # Directory for judger logs

auth:
  jwt:
    secret: "a_very_secret_key_change_me" # JWT signing secret, MUST be changed
    expire_hours: 72
  local:
    enabled: true # Enable local username/password registration and login

# Define a judger cluster
cluster:
  - name: "default-cluster"
    node:
      - name: "local-node"
        # cpu and memory for this node are set at runtime via the admin API
        # (PUT /api/v1/admin/clusters/:clusterName/nodes/:nodeName) and stored in the database.
        docker:
          host: "tcp://127.0.0.1:2375" # Address of the Docker Daemon
```

For more details on configuration files, please refer to the **[Configuration Guides](./configuration/main-config.md)**.

> **Note:** Contests, problems, and other runtime data are managed via the admin API and stored in the database. After first boot (the first registered user becomes superadmin), use the admin UI or admin REST API to create contests and problems. There is no `contests/` directory on disk — all contest and problem definitions live in the database.

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

1. **Register the first user** — the first user to register (via the frontend or `POST /api/v1/auth/local/register`) is automatically granted the `superadmin` role.
2. **Set node resource caps** — a node's `cpu`/`memory` are not in `config.yaml`. Set them via the admin API so the scheduler will accept tasks for the node:
   ```bash
   curl -X PUT /api/v1/admin/clusters/default-cluster/nodes/local-node \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{"cpu": 4, "memory": 4096}'
   ```
3. **Create contests and problems** — use the admin UI or the admin REST API (`POST /api/v1/admin/contests`, then `POST /api/v1/admin/contests/:id/problems`). See [Contest Config](./configuration/contest-config.md) and [Problem Config](./configuration/problem-config.md) for the JSON shapes, and the [Admin API Reference](./api-reference/admin-api.md) for the full endpoint list.

The CSOJ backend service is now running at `http://localhost:8080`.
