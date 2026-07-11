# Problem Config

A problem is a **database record** created via `POST /api/v1/admin/contests/:id/problems` and managed through the admin API. The problem definition and its static assets are stored in the database — **not** on disk. There is no `problem.yaml` file and no per-problem directory.

## JSON Examples

### Example 1: Standard Scoring

A classic A+B problem using the standard file upload and a fixed-point scoring system.

```json
{
  "id": "aplusb",
  "name": "A+B Problem",
  "level": "easy",
  "starttime": "2025-10-01T09:00:00+08:00",
  "endtime": "2025-10-01T12:00:00+08:00",
  "max_submissions": 10,
  "cluster": "default-cluster",
  "cpu": 1,
  "memory": 256,
  "upload": {
    "upload_form": true,
    "maxnum": 2,
    "maxsize": 1
  },
  "score": {
    "mode": "score"
  },
  "workflow": [
    {
      "name": "Compile",
      "image": "gcc:latest",
      "root": false,
      "timeout": 10,
      "show": true,
      "network": false,
      "steps": [["g++", "main.cpp", "-o", "main"]]
    },
    {
      "name": "Run & Judge",
      "image": "zjusct/oj-judger:latest",
      "root": false,
      "timeout": 5,
      "show": false,
      "network": false,
      "mounts": [
        {
          "type": "bind",
          "source": "/path/on/node/testcases/aplusb",
          "target": "/data",
          "readonly": true
        }
      ],
      "steps": [["/judge", "--input", "/data/input.txt", "--ans", "/data/ans.txt", "./main"]]
    }
  ],
  "description": "# A+B Problem\n\nGiven two integers..."
}
```

### Example 2: Performance-Based Scoring

A dynamic scoring rule where a user's score is relative to the best-performing submission.

```json
{
  "id": "performance-example",
  "name": "Performance Optimization",
  "level": "hard",
  "max_submissions": 5,
  "cluster": "default-cluster",
  "cpu": 1,
  "memory": 256,
  "score": {
    "mode": "performance",
    "max_performance_score": 120
  },
  "upload": {
    "editor": true,
    "editor_files": ["main.cpp", "CMakeLists.txt"],
    "maxsize": 1
  },
  "workflow": [
    {
      "name": "Compile",
      "image": "gcc:latest",
      "timeout": 10,
      "show": true,
      "steps": [["cmake", "."], ["make"]]
    },
    {
      "name": "Judge",
      "image": "zjusct/oj-judger:latest",
      "timeout": 5,
      "show": false,
      "steps": [["/judge", "./main"]]
    }
  ],
  "description": "# Performance Optimization\n\n..."
}
```

## Creating a Problem

```bash
curl -X POST /api/v1/admin/contests/sample-contest-1/problems \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{ ... full Problem JSON ... }'
```

The new problem's ID is appended to the parent contest's ordered `problems` list automatically.

## Updating and Deleting

- **Update** — `PUT /api/v1/admin/problems/:id` with the full Problem JSON.
- **Delete** — `DELETE /api/v1/admin/problems/:id` removes the problem record and removes its ID from the parent contest's `problems` list.

All write endpoints trigger an in-memory `reload` so the running server picks up the change immediately.

-----

## Field Reference

### `id`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: A globally unique identifier for the problem.

-----

### `name`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: The display name of the problem.

-----

### `level`

  - **Type**: `string`
  - **Required**: No
  - **Description**: A label for the problem's difficulty (e.g., `"easy"`, `"medium"`, `"hard"`). Display-only.

-----

### `starttime` / `endtime`

  - **Type**: `string` (ISO 8601 format)
  - **Required**: No
  - **Description**: The independent start/end time for the problem. This is useful for contests where problems are unlocked in stages. If set, this time window must be within the parent contest's `starttime` and `endtime`.

-----

### `max_submissions`

  - **Type**: `integer`
  - **Required**: No
  - **Default**: `0` (unlimited)
  - **Description**: Limits the number of valid submissions a user can make for this problem.

-----

### `cluster`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: Specifies which cluster the judging tasks for this problem should be scheduled to. This name must match a `name` defined in the `cluster` section of `config.yaml`.

-----

### `cpu`

  - **Type**: `integer`
  - **Required**: Yes
  - **Description**: The number of CPU cores to request from the scheduler for a judging task. (Per-pool upper bounds are set via the admin API — see [Main Config](./main-config.md).)

-----

### `memory`

  - **Type**: `integer`
  - **Required**: Yes
  - **Description**: The amount of memory (in MB) to request from the scheduler for a judging task.

-----

### `upload`

  - **Type**: `object`
  - **Required**: No
  - **Description**: Configures the submission method and its limits. One of `upload_form` or `editor` should be true.
      - `upload_form`: (boolean) If `true`, the frontend will display a file upload interface. Defaults to `false`.
      - `editor`: (boolean) If `true`, the frontend will display an online code editor. Defaults to `false`.
      - `editor_files`: (array of strings) When `editor` is `true`, this lists the filenames that will be shown as tabs in the online editor. The content from these editors will be submitted as files with these names.
      - `maxnum`: (integer) The maximum number of files a user can upload in a single submission.
      - `maxsize`: (integer) The maximum **total size** in **megabytes (MB)** for all files in a single submission.

-----

### `workflow`

  - **Type**: `array of objects`
  - **Required**: Yes
  - **Description**: Defines the core judging process as an array of steps that are executed sequentially. Each step runs as a Pod (one per step) with a generated shell entrypoint built from `steps`. When `mpi.enabled` is true, the step runs as an `MPIJob` (mpi-operator) instead — see [`mpi`](#mpi) below. Each object in the array represents a step with the following fields:
      - `name`: (string) An optional name for the step (e.g., "Compile", "Judge").
      - `image`: (string, required) The container image to be used for this step.
      - `root`: (boolean) Whether commands inside the container run as the `root` user. For security, this should be `false` whenever possible. Defaults to `false`. (When `false`, the pod runs as UID/GID 1000.)
      - `timeout`: (integer, required) The total timeout for this step, in seconds. Applied as the Pod's `activeDeadlineSeconds`.
      - `show`: (boolean) Whether to allow regular users to view the logs for this step. Typically, compile logs are public (`true`), while judge logs (which might contain test case info) should be hidden (`false`). Defaults to `false`.
      - `network`: (boolean) Whether to enable network access for this step's pod. Defaults to `false` (network disabled).
      - `steps`: (array of arrays of strings, required for non-MPI steps) A list of commands to be executed sequentially inside the container. Each command is an array of strings, like `["command", "arg1", "arg2"]`. They are joined into a `/bin/sh -c` entrypoint that runs each command in order and aborts on the first non-zero exit.
      - `mounts`: (array of objects, optional) A list of additional volumes to mount into the container. Each mount object has:
          - `type`: (string, optional) The mount type. Defaults to `bind`.
          - `source`: (string, required) The path on the host machine (the judger node).
          - `target`: (string, required) The path inside the container.
          - `readonly`: (boolean, optional) Whether to mount the volume as read-only. Defaults to `true`.
      - `resources`: (object, optional) Kubernetes resources for this step. CPU and memory accept Kubernetes quantity strings. GPU is an extended resource and is written with the same value in requests and limits.
          - `cpu_request` / `cpu_limit`: CPU quantities such as `500m` or `2`.
          - `memory_request` / `memory_limit`: Memory quantities such as `256Mi` or `2Gi`.
          - `gpu_count`: Non-negative integer GPU count per Pod/container. `0` or omitted disables GPU allocation.
          - `gpu_resource`: Extended resource advertised by the device plugin. Defaults to `nvidia.com/gpu`; examples include `amd.com/gpu` and NVIDIA MIG resource names.
      - `mpi`: (object, optional) Marks this step as a multi-node MPI job. See [`mpi`](#mpi) below.

-----

### `mpi`

  - **Type**: `object`
  - **Required**: No
  - **Description**: When present and `enabled` is `true`, the step runs as an `MPIJob` (mpi-operator CRD) instead of a single Pod. The launcher pod runs `mpirun -np <worker_replicas*slots_per_worker> <launcher_cmd...>`. Worker pods run `sleep infinity` and provide MPI ranks; the launcher writes `result.json`. When GPU resources are configured, `gpu_count` applies to each launcher and worker Pod. **Requires the mpi-operator CRD installed in the cluster** (probe happens at startup; MPI steps on a cluster without the CRD will fail).
      - `enabled`: (boolean) Whether to run this step as an MPIJob.
      - `worker_replicas`: (integer) The number of worker pods.
      - `slots_per_worker`: (integer) The number of MPI ranks per worker.
      - `launcher_cmd`: (array of strings) The command run after `mpirun -np <N>` in the launcher pod, e.g. `["./a.out"]`.

Example:

```json
{
  "name": "run",
  "image": "openmpi:4",
  "mpi": {
    "enabled": true,
    "worker_replicas": 2,
    "slots_per_worker": 2,
    "launcher_cmd": ["./a.out"]
  },
  "timeout": 300
}
```

Non-MPI steps are unchanged: a Pod with the step's `steps` array joined into a generated `/bin/sh -c` entrypoint.

-----

### `score`

  - **Type**: `object`
  - **Required**: No
  - **Description**: Configures the scoring mechanism for the problem.
      - `mode`: (string) The scoring mode to use.
          - `"score"`: (Default) The judger directly returns a `score` value.
          - `"performance"`: The judger returns a `performance` value (a number), and the system calculates the score based on the ratio of the user's performance to the current best performance across all users.
      - `max_performance_score`: (integer) **Required** when `mode` is `"performance"`. This is the score awarded to the submission with the highest performance.

-----

### `description`

  - **Type**: `string` (Markdown)
  - **Required**: No
  - **Description**: The problem statement shown on the frontend, written in Markdown. Static assets referenced from the description are uploaded and managed via the problem asset endpoints (`POST /api/v1/admin/problems/:id/assets`).

-----

## Judge Result JSON Contract

The **final step** of the workflow is responsible for reporting the result by writing a JSON file to `/mnt/work/.csoj/result.json` on the shared submission PVC (the same `csoj-submissions` PVC the API server mounts at `storage.submission_content`). After the pod completes, the dispatcher reads this file from the PVC; if the file is missing or invalid, the submission is marked `Failed`.

The required fields in the JSON depend on the `score.mode`.

#### `score.mode: "score"`

The JSON must contain a `score` field. A `performance` field can be included but will be ignored by the scoring system.

```json
{
  "score": 100,
  "performance": 0,
  "info": {
    "message": "All test cases passed",
    "time_usage_ms": 50,
    "memory_usage_kb": 1024
  }
}
```

  - `score`: (integer, required) The final score awarded for this submission.
  - `performance`: (number, optional) Ignored in `"score"` mode. Defaults to `0` if omitted.
  - `info`: (object, optional) Any additional information you wish to store and display.

#### `score.mode: "performance"`

The JSON must contain a `performance` field. A `score` field can be included but will be ignored.

```json
{
  "score": 0,
  "performance": 153.28,
  "info": {
    "message": "Calculation finished",
    "iterations": 1000000,
    "time_usage_ms": 120
  }
}
```

  - `performance`: (number, required) A metric indicating the quality of the solution. A higher value is considered better. The system will automatically calculate the final `score` based on this value relative to other users.
  - `score`: (integer, optional) Ignored in `"performance"` mode. Defaults to `0` if omitted.
  - `info`: (object, optional) Any additional information to store and display.

> **Tip — writing the file from the step's command:** use a step like
> `["sh", "-c", "mkdir -p /mnt/work/.csoj && echo '{\"score\":100,\"performance\":0,\"info\":{}}' > /mnt/work/.csoj/result.json"]`.
> Do not print the JSON to stdout — stdout is streamed to the user's log view and is **not** parsed for the score.
