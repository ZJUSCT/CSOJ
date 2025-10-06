# User API Reference

The User API is the primary way for regular users to interact with the CSOJ system. All User API routes are prefixed with `/api/v1`.

## Authentication

- **JWT**: Most authenticated endpoints are secured using an `Authorization: Bearer <token>` HTTP header.
- **Obtaining a Token**: Users obtain a JWT through one of the login endpoints.

---

### Auth

#### `POST /auth/local/register`
  - **Description**: Registers a new user (when local auth is enabled).
  - **Authentication**: None
  - **Request Body** (`application/json`):
    ```json
    {
      "username": "newuser",
      "password": "password123",
      "nickname": "New User"
    }
    ```
  
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": { "id": "user-uuid", "username": "newuser" },
      "message": "User registered successfully"
    }
    ```

#### `POST /auth/local/login`

  - **Description**: Logs in a user with a username and password (when local auth is enabled).
  - **Authentication**: None
  - **Request Body** (`application/json`):
    ```json
    {
      "username": "newuser",
      "password": "password123"
    }
    ```
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": { "token": "your_jwt_token_here" },
      "message": "Login successful"
    }
    ```

#### `GET /auth/gitlab/login`

  - **Description**: Redirects the user to GitLab for OAuth2 authentication.
  - **Authentication**: None

#### `GET /auth/gitlab/callback`

  - **Description**: The callback URL for GitLab OAuth2. On success, it returns a JWT.
  - **Authentication**: None

-----

### Contests

#### `GET /contests`

  - **Description**: Gets a list of all available contests.
  - **Authentication**: None
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": {
        "contest-id-1": { "id": "...", "name": "...", ... },
        "contest-id-2": { "id": "...", "name": "...", ... }
      },
      "message": "Contests loaded"
    }
    ```

#### `GET /contests/:id`

  - **Description**: Gets detailed information for a single contest. If the contest has not started or has ended, the `problem_ids` array will be empty.
  - **Authentication**: None
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": {
        "id": "sample-contest",
        "name": "Sample Introductory Contest",
        "starttime": "...",
        "endtime": "...",
        "problem_ids": ["aplusb", "fizzbuzz"],
        "description": "Contest description..."
      },
      "message": "Contest found"
    }
    ```

#### `GET /contests/:id/leaderboard`

  - **Description**: Gets the leaderboard for a contest.
  - **Authentication**: None

#### `GET /contests/:id/trend`

  - **Description**: Gets the score trend data for the top 10 users in a contest.
  - **Authentication**: None

#### `POST /contests/:id/register`

  - **Description**: Registers the current user for an ongoing contest.
  - **Authentication**: JWT
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": null,
      "message": "Successfully registered for contest"
    }
    ```

#### `GET /contests/:id/history`

  - **Description**: Gets the score change history for the current user in a contest.
  - **Authentication**: JWT

-----

### Problems

#### `GET /problems/:id`

  - **Description**: Gets detailed information for a single problem. Only accessible after the contest and problem have both started.
  - **Authentication**: None

#### `POST /problems/:id/submit`

  - **Description**: Submits code/files for a problem. The request must be of type `multipart/form-data`.
  - **Authentication**: JWT
  - **Request Body** (`multipart/form-data`):
      - `files`: One or more file fields.
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": { "submission_id": "new-submission-uuid" },
      "message": "Submission received"
    }
    ```

#### `GET /problems/:id/attempts`

  - **Description**: Gets information about the current user's submission attempts for a problem.
  - **Authentication**: JWT
  - **Success Response** (`200 OK`):
    ```json
    {
      "code": 0,
      "data": {
          "limit": 10,  // Submission limit, or null if unlimited
          "used": 2,    // Submissions used
          "remaining": 8 // Submissions remaining, or null if unlimited
      },
      "message": "Submission attempts retrieved successfully"
    }
    ```

-----

### Submissions

#### `GET /submissions`

  - **Description**: Gets all submissions for the current user.
  - **Authentication**: JWT

#### `GET /submissions/:id`

  - **Description**: Gets a specific submission for the current user.
  - **Authentication**: JWT

#### `POST /submissions/:id/interrupt`

  - **Description**: Interrupts a submission that is currently queued or running.
  - **Authentication**: JWT

#### `GET /submissions/:id/queue_position`

  - **Description**: Gets the queue position for a queued submission.
  - **Authentication**: JWT

#### `GET /submissions/:id/containers/:conID/log`

  - **Description**: Gets the full log for a specific step (container) of a submission. The step must be configured with `show: true` in `problem.yaml`.
  - **Authentication**: JWT

-----

### User Profile

#### `GET /user/profile`

  - **Description**: Gets the current user's profile.
  - **Authentication**: JWT

#### `PATCH /user/profile`

  - **Description**: Updates the current user's nickname and signature.
  - **Authentication**: JWT
  - **Request Body** (`application/json`):
    ```json
    {
      "nickname": "My New Nickname",
      "signature": "Hello World!"
    }
    ```

#### `POST /user/avatar`

  - **Description**: Uploads and updates the current user's avatar.
  - **Authentication**: JWT
  - **Request Body** (`multipart/form-data`):
      - `avatar`: An image file field.

-----

### Assets

These endpoints serve protected static assets like user avatars and problem attachments.

#### `GET /assets/avatars/:filename`

  - **Description**: Gets a user avatar image.
  - **Authentication**: JWT

#### `GET /assets/contests/:id/*assetpath`

  - **Description**: Gets a static asset referenced in a contest's `index.md` description.
  - **Authentication**: JWT

#### `GET /assets/problems/:id/*assetpath`

  - **Description**: Gets a static asset referenced in a problem's `index.md` statement.
  - **Authentication**: JWT

-----

### WebSocket

#### `GET /ws/submissions/:subID/containers/:conID/logs?token=<jwt>`

  - **Description**: Establishes a WebSocket connection to stream real-time logs from a judging container.
  - **Authentication**: JWT passed via the `token` query parameter.
  - **Message Format** (JSON):
    ```json
    {
      "stream": "stdout", // "stdout", "stderr", "info", or "error"
      "data": "log content line"
    }
    ```