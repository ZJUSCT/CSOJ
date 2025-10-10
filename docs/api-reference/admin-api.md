# Admin API Reference

The Admin API provides a set of powerful endpoints for system maintenance and management. By default, the Admin API service is separate from the User API and runs on a different port (which must be enabled and configured in `config.yaml`).

## Authentication
The current version of the Admin API has **no built-in authentication mechanism**. It is crucial to ensure that the Admin API's listen address is **only accessible from trusted network environments (e.g., an internal network or localhost)**, or to add an authentication layer using a reverse proxy.

---

### System Management

#### `POST /reload`
  - **Description**: Hot-reloads all contest and problem configurations.
    - The system rescans all directories specified in the `contest` list in `config.yaml`.
    - New or modified contests/problems will be loaded.
    - If a problem is deleted, all submission records associated with that problem will also be **permanently deleted from the database**.
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

  - **Description**: Gets a list of all users.

#### `POST /users`

  - **Description**: Manually creates a new user.
  - **Request Body** (`application/json`):
    ```json
    {
      "username": "admin_created_user",
      "password_hash": "$2a$14$....", // bcrypt hash, optional
      "nickname": "Test User"
    }
    ```
    > Note: `password_hash` is optional and mainly for migration purposes. You should generally use the public registration API to create users with passwords.

#### `DELETE /users/:id`

  - **Description**: Deletes a user by their ID.

-----

### Submission Management

#### `GET /submissions`

  - **Description**: Gets a list of all submissions in the system.

#### `GET /submissions/:id`

  - **Description**: Gets the detailed information for any submission.

#### `GET /submissions/:id/containers/:conID/log`

  - **Description**: Gets the full log for any step (container) of any submission, regardless of the `show` flag. The log is returned in NDJSON format.

#### `POST /submissions/:id/rejudge`

  - **Description**: Re-judges an existing submission.
      - The system marks the original submission as invalid (`is_valid: false`).
      - It then copies the original submission's content, creates a new submission record, and adds it to the judging queue.
      - The scoring system automatically handles score changes resulting from the re-judge.

#### `PATCH /submissions/:id/validity`

  - **Description**: Manually marks a submission as valid or invalid. This can trigger a score recalculation.
  - **Request Body** (`application/json`):
    ```json
    {
      "is_valid": false
    }
    ```

#### `POST /submissions/:id/interrupt`

  - **Description**: Forcibly interrupts a queued or running submission.

-----

### Cluster Management

#### `GET /clusters/status`

  - **Description**: Gets the current resource usage status of all configured clusters and nodes.

-----

### WebSocket

#### `GET /ws/submissions/:id/containers/:conID/logs`

  - **Description**: Establishes a WebSocket connection to stream the complete log for any container. For finished containers, it streams the saved log file. For running containers, it first sends all historical logs from the cache and then continues to stream new logs in real-time. This is available regardless of the `show` flag.
  - **Authentication**: None.
