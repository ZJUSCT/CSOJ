# Admin API Reference

The Admin API provides a set of powerful endpoints for system maintenance and management. By default, the Admin API service is separate from the User API and runs on a different port (which must be enabled and configured in `config.yaml`).

## Authentication

The current version of the Admin API has **no built-in authentication mechanism**. It is crucial to ensure that the Admin API's listen address is **only accessible from trusted network environments (e.g., an internal network or localhost)**, or to add an authentication layer using a reverse proxy.

---

### System Management

#### `POST /reload`

  - **Description**: Hot-reloads all contest and problem configurations from disk.
    - The system rescans all directories specified in the `contest` list in `config.yaml`.
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

#### `GET /users`
  - **Description**: Gets a list of all users. Can be filtered by a `query` parameter that searches User ID, username, and nickname.

#### `POST /users`
  - **Description**: Manually creates a new user.
  - **Request Body** (`application/json`):
    ```json
    {
      "username": "admin_created_user",
      "password_hash": "$2a$14$....", // bcrypt hash, required for local auth users
      "nickname": "Test User"
    }
    ```

#### `GET /users/:id`
  - **Description**: Gets a single user by their ID.

#### `PATCH /users/:id`
  - **Description**: Updates a user's nickname and signature.

#### `DELETE /users/:id`
  - **Description**: Deletes a user by their ID.

#### `POST /users/:id/reset-password`
  - **Description**: Resets the password for a local-auth user.
  - **Request Body** (`application/json`): `{"password": "new_secure_password"}`

#### `POST /users/:id/register-contest`
  - **Description**: Manually registers a user for a specific contest.
  - **Request Body** (`application/json`): `{"contest_id": "contest-id-here"}`

#### `GET /users/:id/history`
  - **Description**: Gets a user's score history for a specific contest.
  - **Query Parameter**: `contest_id` (required).

#### `GET /users/:id/scores`
  - **Description**: Gets a user's best scores for all problems they have submitted to.

-----

### Submission Management

#### `GET /submissions`
  - **Description**: Gets a paginated list of all submissions. Supports filtering by `problem_id`, `status`, and `user_query`. Supports pagination with `page` and `limit`.

#### `GET /submissions/:id`
  - **Description**: Gets detailed information for a single submission.

#### `GET /submissions/:id/content`
  - **Description**: Downloads the content of a submission as a zip archive.

#### `PATCH /submissions/:id`
  - **Description**: Manually updates the `status`, `score`, or `info` field of a submission. **Warning: This does not trigger score recalculation.**

#### `DELETE /submissions/:id`
  - **Description**: Permanently deletes a submission record and its content from disk.

#### `POST /submissions/:id/rejudge`
  - **Description**: Re-judges an existing submission.
      - The system marks the original submission as invalid (`is_valid: false`).
      - It then copies the original submission's content, creates a new submission record, and adds it to the judging queue.
      - The scoring system automatically handles score changes resulting from the re-judge.

#### `PATCH /submissions/:id/validity`
  - **Description**: Manually marks a submission as valid or invalid. This **triggers a full score recalculation** for the user on that problem.
  - **Request Body** (`application/json`): `{"is_valid": false}`

#### `POST /submissions/:id/interrupt`
  - **Description**: Forcibly interrupts a queued or running submission, marking it as `Failed`.

#### `GET /submissions/:id/containers/:conID/log`
  - **Description**: Gets the full log for any step (container) of any submission, regardless of the `show` flag. The log is returned in NDJSON format.

-----

### Contest & Problem Management

#### `GET /contests`
  - **Description**: Gets a list of all loaded contests, regardless of start/end times.

#### `GET /contests/:id`
  - **Description**: Gets details for a specific contest, regardless of start/end times.

#### `GET /contests/:id/leaderboard`
  - **Description**: Gets the leaderboard for a contest.

#### `GET /contests/:id/trend`
  - **Description**: Gets score trend data for top users. Supports a `maxnum` query parameter to control the number of users.

#### `GET /problems`
  - **Description**: Gets a list of all loaded problems.

#### `GET /problems/:id`
  - **Description**: Gets the full definition of a single problem.

-----

### Score Management

#### `POST /scores/recalculate`
  - **Description**: Triggers a score recalculation for a specific user on a specific problem.
  - **Request Body** (`application/json`): `{"user_id": "user-uuid", "problem_id": "problem-id"}`

-----

### Cluster & Container Management

#### `GET /clusters/status`
  - **Description**: Gets the current resource usage and queue lengths for all configured clusters and nodes.

#### `GET /clusters/:clusterName/nodes/:nodeName`
  - **Description**: Gets detailed status for a specific node.

#### `POST /clusters/:clusterName/nodes/:nodeName/pause`
  - **Description**: Pauses a node, preventing it from accepting new judging tasks.

#### `POST /clusters/:clusterName/nodes/:nodeName/resume`
  - **Description**: Resumes a paused node.

#### `GET /containers`
  - **Description**: Gets a paginated list of all containers. Supports filtering by `submission_id`, `status`, and `user_query`.

#### `GET /containers/:id`
  - **Description**: Gets details for a single container.

-----

### WebSocket

#### `GET /ws/submissions/:id/containers/:conID/logs`
  - **Description**: Establishes a WebSocket connection to stream the complete log for any container. For finished containers, it streams the saved log file. For running containers, it first sends all historical logs from the cache and then continues to stream new logs in real-time. This is available regardless of the `show` flag.
  - **Authentication**: None.