# Admin API Reference

The Admin API provides a set of powerful endpoints for system maintenance and management. All Admin API routes are mounted under the main CSOJ service and share its listen address (`listen` in `config.yaml`).

All admin-managed data (contests, problems, announcements, assets, links, cluster node resource caps) is persisted in the database. There are no on-disk `contest.yaml` / `problem.yaml` files to edit — every write goes through the endpoints below and triggers an in-memory `reload` so the running server picks up the change immediately.

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

### Cluster & Container Management

#### `GET /api/v1/admin/clusters/status`

  - **Description**: Gets the current resource usage and queue lengths for all configured clusters and nodes.

#### `GET /api/v1/admin/clusters/:clusterName/nodes/:nodeName`

  - **Description**: Gets detailed status for a specific node.

#### `POST /api/v1/admin/clusters/:clusterName/nodes/:nodeName/pause`

  - **Description**: Pauses a node, preventing it from accepting new judging tasks.

#### `POST /api/v1/admin/clusters/:clusterName/nodes/:nodeName/resume`

  - **Description**: Resumes a paused node.

#### `PUT /api/v1/admin/clusters/:clusterName/nodes/:nodeName`

  - **Description**: Updates a node's `cpu` and `memory` resource caps. The values are persisted to the `cluster_nodes` table and applied to the running scheduler in place (no restart required). **Requires the `admin` or `superadmin` role.**
  - **Request Body** (`application/json`):
    ```json
    {
      "cpu": 4,
      "memory": 4096
    }
    ```
    - `cpu`: (integer, required, positive) Total CPU cores the scheduler may use on this node.
    - `memory`: (integer, required, positive) Total memory (in MB) the scheduler may use on this node.
  - **Note**: A node must already be present in `config.yaml` (its Docker connection) for the scheduler to recognize it. This endpoint only sets the runtime resource caps — adding a new node's Docker connection requires editing `config.yaml` and restarting.

#### `GET /api/v1/admin/containers`

  - **Description**: Gets a paginated list of all containers. Supports filtering by `submission_id`, `status`, and `user_query`.

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
