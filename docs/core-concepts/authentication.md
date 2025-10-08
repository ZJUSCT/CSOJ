# Authentication

CSOJ provides a flexible authentication system to manage user identity. Authenticated users are issued a JSON Web Token (JWT) which they must provide in the `Authorization` header for protected API endpoints.

## Authentication Methods

CSOJ supports two primary authentication methods, which can be enabled in `config.yaml`.

### 1. Local Authentication

  - **Provider**: CSOJ
  - **How it works**: This is the traditional username and password system. Users can register for a new account and log in directly through the CSOJ API. Passwords are   securely stored using the `bcrypt` hashing algorithm.
  - **Configuration (`config.yaml`)**:
    ```yaml
    auth:
      local:
        enabled: true
    ```

  - **Relevant API Endpoints**:
      - `POST /api/v1/auth/local/register`
      - `POST /api/v1/auth/local/login`

### 2. GitLab OAuth2

  - **Provider**: GitLab (e.g., gitlab.com or a self-hosted instance)
  - **How it works**: CSOJ integrates with GitLab's OAuth2 protocol to delegate authentication. The process is as follows:
    1.  A user clicks a "Login with GitLab" button on the frontend.
    2.  The frontend directs the user to CSOJ's `GET /api/v1/auth/gitlab/login` endpoint.
    3.  CSOJ redirects the user to the GitLab authorization page.
    4.  The user approves the authorization request on GitLab.
    5.  GitLab redirects the user back to CSOJ's configured `redirect_uri` (`/api/v1/auth/gitlab/callback`).
    6.  CSOJ's callback handler receives an authorization code, exchanges it for an access token, and fetches the user's profile.
    7.  If the user exists in the CSOJ database (matched by GitLab ID), they are logged in. If not, a new user is created.
    8.  CSOJ issues its own JWT and finally redirects the user to the `frontend_callback_url` with the token appended as a query parameter (e.g., `http://frontend.com/callback?token=...`).
  - **Configuration (`config.yaml`)**:
    ```yaml
    auth:
      gitlab:
        url: "[https://gitlab.com](https://gitlab.com)"
        client_id: "YOUR_GITLAB_CLIENT_ID"
        client_secret: "YOUR_GITLAB_CLIENT_SECRET"
        redirect_uri: "[http://your-csoj-host.com/api/v1/auth/gitlab/callback](http://your-csoj-host.com/api/v1/auth/gitlab/callback)"
        frontend_callback_url: "[http://your-frontend-host.com/auth/callback](http://your-frontend-host.com/auth/callback)"
    ```

## JSON Web Token (JWT)

Regardless of the login method, a successful authentication results in the issuance of a JWT.

  - **Usage**: The client must include the JWT in the `Authorization` header for all subsequent requests to protected endpoints.
    ```
    Authorization: Bearer <your_jwt_token>
    ```
  - **Claims**: The JWT payload contains standard claims like `exp` (expiration time) and `iat` (issued at), with the `sub` (subject) claim holding the CSOJ User ID.
  - **Security**: The JWT is signed using HMAC with SHA-256 (HS256). The secret key used for signing is defined in `config.yaml` and is critical to the security of the system.
  - **Configuration (`config.yaml`)**:
    ```yaml
    auth:
      jwt:
        secret: "a_very_secret_key_change_me" # MUST be changed in production
        expire_hours: 72
    ```
