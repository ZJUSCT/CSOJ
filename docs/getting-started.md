# Getting Started

This document will guide you through compiling and running the CSOJ backend service.

## 1. Prerequisites

- **Go**: Version `1.20` or higher is recommended.
- **Docker**: The Docker service must be installed and running on the judger nodes. CSOJ communicates with the Docker Daemon via a TCP socket.

## 2. Compile the Project

After cloning the project repository, execute the following command in the project root to compile the `CSOJ` executable:

```bash
go build -o csoj ./cmd/CSOJ/main.go
````

This will generate an executable file named `csoj` in the project root directory.

## 3\. Prepare Configuration Files

The core of CSOJ is its configuration. You will need at least one main configuration file.

1.  Create a `configs` folder in the project root.
2.  Create a `config.yaml` file inside the `configs` folder.

Here is a minimal example of `config.yaml`:

```yaml
# configs/config.yaml

listen: ":8080" # Listen address for the user-facing API service

logger:
  level: "debug" # Log level (can be "debug" or "production")

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
        cpu: 4 # Total CPU cores available for judging on this node
        memory: 4096 # Total memory (in MB) available for judging
        docker: "tcp://127.0.0.1:2375" # Address of the Docker Daemon

# Paths to contest and problem configuration files
contest:
  - "contests/sample-contest"

# (Optional) Dynamic links for the frontend navigation bar
links:
  - name: "Project Source"
    url: "[https://github.com/ZJUSCT/CSOJ](https://github.com/ZJUSCT/CSOJ)"
```

For more details on configuration files, please refer to the **[Configuration Guides](https://www.google.com/search?q=./configuration/main-config.md)**.

## 4\. Prepare Contest and Problem Files

Based on the `contest` configuration in `config.yaml`, create the corresponding directories and files. For the example above, you would need the following structure:

```
.
├── contests
│   └── sample-contest
│       ├── contest.yaml
│       ├── index.md
│       └── p1001-aplusb
│           ├── index.md
│           └── problem.yaml
└── ...
```

To learn how to write `contest.yaml` and `problem.yaml`, see **[Contest Config](https://www.google.com/search?q=./configuration/contest-config.md)** and **[Problem Config](https://www.google.com/search?q=./configuration/problem-config.md)**.

## 5\. Run the Service

Once the above steps are complete, you can start the CSOJ service with the following command:

```bash
# The -c flag specifies the path to the main configuration file
./csoj -c configs/config.yaml
```

If everything is configured correctly, you should see output similar to this in your console:

```
ZJUSCT CSOJ dev-build - Fully Containerized Secure Online Judgement

{"level":"info","ts":...,"caller":"...","msg":"database initialized successfully"}
{"level":"info","ts":...,"caller":"...","msg":"successfully recovered and cleaned up interrupted tasks"}
{"level":"info","ts":...,"caller":"...","msg":"loaded 1 contests and 1 problems"}
{"level":"info","ts":...,"caller":"...","msg":"judger scheduler started"}
{"level":"info","ts":...,"caller":"...","msg":"starting user server at :8080"}
```

The CSOJ backend service is now running at `http://localhost:8080`.
