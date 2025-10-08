# Problem Config (problem.yaml)

Each problem is defined by a separate directory, the path of which must be declared in the `problems` list of its parent `contest.yaml` file.

A problem directory must contain a `problem.yaml` file and may optionally include an `index.md` file for the problem statement.

## Directory Structure Example

```

...
└── p1001-aplusb/
├── problem.yaml    \# The core configuration file for the problem
├── index.md        \# (Optional) The problem statement in Markdown
└── index.assets/   \# (Optional) Static assets (e.g., images) referenced in the statement
└── image.png

```

---

## `problem.yaml` Example

This is a configuration example for a classic A+B problem.

```yaml
# The unique ID for the problem
id: "aplusb"

# The name of the problem
name: "A+B Problem"

# Independent open time for the problem (optional)
# If set, it takes precedence over the contest time, but must be within the contest's time range
starttime: "2025-10-01T09:00:00+08:00"
endtime: "2025-10-01T12:00:00+08:00"

# Maximum number of valid submissions per user for this problem. 0 means unlimited.
max_submissions: 10

# Judging resource configuration
cluster: "default-cluster"  # Specifies which cluster to judge on
cpu: 1                      # Number of CPU cores to request for judging
memory: 256                 # Amount of memory (in MB) to request for judging

# The judging workflow
workflow:
  # Step 1: Compile the C++ code
  - name: "Compile"              # (Optional) Name for this step
    image: "gcc:latest"          # The Docker image to use
    root: false                  # Whether to run the container as the root user
    timeout: 10                  # Timeout for this step in seconds
    show: true                   # Whether to allow users to see the log for this step
    steps:
      - ["g++", "main.cpp", "-o", "main"]

  # Step 2: Run and judge
  - name: "Run & Judge"          # (Optional) Name for this step
    image: "zjusct/oj-judger:latest" # Use an image with judging tools
    root: false
    timeout: 5
    show: false                  # Judge logs are usually not shown to users
    network: false               # (Optional) Disable network for this container
    mounts:                      # (Optional) Extra volume mounts
      - type: bind
        source: "/etc/csoj/testcases/aplusb" # Path on the judger node
        target: "/data"                     # Path inside the container
        readonly: true                      # Mount as read-only
    steps:
      # This is a hypothetical judging command
      # It reads standard input, runs the user's program, compares the output,
      # and prints the result (in JSON format) to standard output.
      - ["/judge", "--input", "/data/input.txt", "--ans", "/data/ans.txt", "./main"]

```

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

### `starttime` / `endtime`

  - **Type**: `string` (ISO 8601 format)
  - **Required**: No
  - **Description**: The independent start/end time for the problem. This is useful for contests where problems are unlocked in stages. If set, this time window must be within the parent contest's `starttime` and `endtime`.

-----

### `max_submissions`

  - **Type**: `integer`
  - **Required**: No
  - **Description**: Limits the number of valid submissions a user can make for this problem. `0` or not set means unlimited.

-----

### `cluster`

  - **Type**: `string`
  - **Required**: Yes
  - **Description**: Specifies which cluster the judging tasks for this problem should be scheduled to. This name must match the `name` of a `cluster` defined in `config.yaml`.

-----

### `cpu`

  - **Type**: `integer`
  - **Required**: Yes
  - **Description**: The number of CPU cores to request from the scheduler for a judging task for this problem.

-----

### `memory`

  - **Type**: `integer`
  - **Required**: Yes
  - **Description**: The amount of memory (in MB) to request from the scheduler for a judging task for this problem.

-----

### `workflow`

  - **Type**: `array of objects`
  - **Required**: Yes
  - **Description**: Defines the core judging process as an array of steps that are executed sequentially. Each object in the array represents a step and has the following fields:
      - `name`: (string, optional) An optional name for the step, which will be returned via the API. Defaults to `Step N` if not provided.
      - `image`: (string, required) The Docker image to be used for this step.
      - `root`: (boolean, optional) Whether commands inside the container run as the `root` user. For security, this should be `false` whenever possible. Defaults to `false`.
      - `timeout`: (integer, required) The total timeout for this step, in seconds.
      - `show`: (boolean, optional) Whether to allow regular users to view the logs for this step via the API. Typically, compile logs can be public (`true`), while judge logs (which might contain test case info) should be hidden (`false`). Defaults to `false`.
      - `network`: (boolean, optional) Whether to enable network access for this step's container. Defaults to `false` (network disabled).
      - `steps`: (array of arrays of strings, required) A list of commands to be executed sequentially inside the container. Each command is an array of strings, like `["command", "arg1", "arg2"]`.
      - `mounts`: (array of objects, optional) A list of additional volumes to mount into the container. Each mount object has the following fields:
          - `type`: (string, optional) The mount type. Defaults to `bind`.
          - `source`: (string, required) The path on the host machine (the judger node).
          - `target`: (string, required) The path inside the container.
          - `readonly`: (boolean, optional) Whether to mount the volume as read-only. Defaults to `true`.

#### Judge Result JSON Format

The **final step** of the workflow is responsible for reporting the result by printing a JSON object to standard output.

```json
{
  "score": 100,
  "info": {
    "message": "All test cases passed",
    "time_usage_ms": 50,
    "memory_usage_kb": 1024
  }
}
```

  - `score`: (integer) The score awarded for this submission.
  - `info`: (object) Any additional information you wish to store and display.
