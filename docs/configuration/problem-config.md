# Problem Config (problem.yaml)

Each problem is defined by a separate directory, the path of which must be declared in the `problems` list of its parent `contest.yaml` file.

A problem directory must contain a `problem.yaml` file and may optionally include an `index.md` file for the problem statement.

## Directory Structure Example

```

...
├── problem.yaml    \# The core configuration file for the problem
├── index.md        \# (Optional) The problem statement in Markdown
└── index.assets/   \#(Optional) Static assets (e.g., images) referenced in the statement

```

---

## `problem.yaml` Examples

### Example 1: Standard File Upload

This is a configuration for a classic A+B problem using the file upload method.

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

# Limits on user-uploaded files (optional)
upload:
  maxnum: 2    # Max number of files allowed
  maxsize: 1   # Max total size for all files in MB

# Judging resource configuration
cluster: "default-cluster"  # Specifies which cluster to judge on
cpu: 1                      # Number of CPU cores to request for judging
memory: 256                 # Amount of memory (in MB) to request for judging

# The judging workflow
workflow:
  # Step 1: Compile the C++ code
  - name: "Compile"
    image: "gcc:latest"
    root: false
    timeout: 10
    show: true
    steps:
      - ["g++", "main.cpp", "-o", "main"]

  # Step 2: Run and judge
  - name: "Run & Judge"
    image: "zjusct/oj-judger:latest"
    root: false
    timeout: 5
    show: false
    network: false
    mounts:
      - type: bind
        source: "/etc/csoj/testcases/aplusb" # Path on the judger node
        target: "/data"                     # Path inside the container
        readonly: true
    steps:
      # This hypothetical command runs the user's program and prints the result JSON to stdout.
      - ["/judge", "--input", "/data/input.txt", "--ans", "/data/ans.txt", "./main"]
```

### Example 2: Online Editor

This problem is configured to use the online editor in the frontend.

```yaml
id: "online-edit-example"
name: "Online Editor Problem"
max_submissions: 5
cluster: "default-cluster"
cpu: 1
memory: 256

# Configure the online editor
upload:
  editor: true
  editor_files:
    - "main.cpp"
    - "CMakeLists.txt"
  maxsize: 1 # Max total size of 1 MB for all editor content

workflow:
  # The workflow remains the same. The judge will receive the editor content
  # as files with the names specified in `editor_files`.
  - name: "Compile"
    image: "gcc:latest"
    timeout: 10
    show: true
    steps:
      - ["g++", "main.cpp", "-o", "main"]
  - name: "Judge"
    image: "zjusct/oj-judger:latest"
    timeout: 5
    show: false
    steps:
      - ["/judge", "./main"]
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

### `upload`

  - **Type**: `object`
  - **Required**: No
  - **Description**: Configures the submission method and its limits.
      - `maxnum`: (integer) The maximum number of files a user can upload in a single submission.
      - `maxsize`: (integer) The maximum **total size** in **megabytes (MB)** for all files in a single submission.
      - `editor`: (boolean, optional) If set to `true`, the frontend will display an online code editor instead of a file upload interface. Defaults to `false`.
      - `editor_files`: (array of strings, optional) When `editor` is `true`, this lists the filenames that will be shown as tabs in the online editor. The content from these editors will be submitted as files with these names.

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
