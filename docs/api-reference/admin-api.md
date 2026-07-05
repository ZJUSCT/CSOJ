# Admin API Reference

The Admin API provides a set of powerful endpoints for system maintenance and management. All Admin API routes are mounted under the main CSOJ service and share its listen address (`listen` in `config.yaml`).

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

- **Description**: Hot-reloads all contest and problem configurations from disk.
  - The system rescans the directory specified in `contests_root` in `config.yaml`.
  - New or modified contests/problems will be loaded.
  - If a problem is deleted, all submission records associated with that problem will also be **permanently deleted from the database**, including any running containers associated with them.
- **Success Response** (`200 OK`):
  ```json
  {
    "code": 0,
    "data": {
      "contests_loaded": 2,
      "problems_loaded": 15,
      "submissions_deleted": 5
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

  - **Description**: Creates a new contest by creating the necessary directory and `contest.yaml` file on disk. Requires a `reload` to be active.
  - **Request Body**: A full `Contest` JSON object.

#### `GET /api/v1/admin/contests/:id`

  - **Description**: Gets details for a specific contest, regardless of start/end times.

#### `PUT /api/v1/admin/contests/:id`

  - **Description**: Updates the `contest.yaml` file for a contest. Triggers a system `reload`.
  - **Request Body**: A full `Contest` JSON object.

#### `DELETE /api/v1/admin/contests/:id`

  - **Description**: Deletes a contest's directory and all its contents from disk. Triggers a system `reload`.

#### `POST /api/v1/admin/contests/:id/problems`

  - **Description**: Creates a new problem within a contest. Triggers a system `reload`.
  - **Request Body**: A full `Problem` JSON object.

#### `GET /api/v1/admin/problems`

  - **Description**: Gets a list of all loaded problems.

#### `GET /api/v1/admin/problems/:id`

  - **Description**: Gets the full definition of a single problem.

#### `PUT /api/v1/admin/problems/:id`

  - **Description**: Updates a `problem.yaml` file. Triggers a system `reload`.
  - **Request Body**: A full `Problem` JSON object.

#### `DELETE /api/v1/admin/problems/:id`

  - **Description**: Deletes a problem's directory from disk. Triggers a system `reload`.

-----

### Contest Assets & Announcements

#### `GET /api/v1/admin/contests/:id/assets`

  - **Description**: Lists all static assets for a contest.

#### `POST /api/v1/admin/contests/:id/assets`

  - **Description**: Uploads one or more asset files to a contest's `index.assets` directory.

#### `DELETE /api/v1/admin/contests/:id/assets`

  - **Description**: Deletes an asset (file or directory) from a contest.

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

  - **Description**: Uploads one or more asset files to a problem's `index.assets` directory.

#### `DELETE /api/v1/admin/problems/:id/assets`

  - **Description**: Deletes an asset (file or directory) from a problem.

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

#### `GET /api/v1/admin/containers`

  - **Description**: Gets a paginated list of all containers. Supports filtering by `submission_id`, `status`, and `user_query`.

#### `GET /api/v1/admin/containers/:id`

  - **Description**: Gets details for a single container.

-----

### WebSocket

#### `GET /api/v1/admin/ws/submissions/:id/containers/:conID/logs`

  - **Description**: Establishes a WebSocket connection to stream the complete log for any container. For finished containers, it streams the saved log file. For running containers, it first sends all historical logs from the cache and then continues to stream new logs in real-time. This is available regardless of the `show` flag.
  - **Authentication**: Requires a valid admin JWT (passed as a query parameter or header, depending on the client).
