# Judger Workflow

The core of CSOJ is its flexible, container-based judging workflow. This workflow defines a series of steps that are executed for every submission to a problem. It is defined in the `workflow` section of a `problem.yaml` file.

## The Lifecycle of a Submission

1.  **Submission**: A user submits their files (e.g., `main.cpp`) to a specific problem via the API.
2.  **Queuing**: The submission is received, saved to the storage, and a record is created in the database with the status `Queued`. It is then passed to the [Scheduler](./scheduler-cluster.md).
3.  **Scheduling**: The Scheduler waits for a node in the problem's specified cluster to have enough CPU and memory resources.
4.  **Dispatching**: Once resources are available, the submission is assigned to a node. Its status is updated to `Running`.
5.  **Workflow Execution**: The Dispatcher on the assigned node begins executing the steps defined in the problem's `workflow`.

## Workflow Steps

The `workflow` is an array of steps, executed sequentially. Each step runs in a new, clean Docker container.

### Example Workflow from `problem.yaml`

```yaml
workflow:
  # Step 1: Compilation
  - name: "Compile"
    image: "gcc:latest"
    timeout: 10
    show: true
    steps:
      - ["g++", "main.cpp", "-o", "main", "-O2"]

  # Step 2: Judging
  - name: "Judge"
    image: "zjusct/oj-judger:latest"
    timeout: 5
    show: false
    steps:
      - ["/judge", "--bin", "./main"]
```

### How It Works

  - **File System Persistence**: For each submission, the system creates a temporary working directory on the host judger node. This host directory is then bind-mounted into the working directory (`/mnt/work/`) of **every container** in the workflow. This ensures that any files created or modified in one step (like a compiled binary) are available to all subsequent steps.
  - **Step 1 (Compilation)**:
      - A container is created from the `gcc:latest` image.
      - The user's submitted files are copied from storage into the host working directory, which is then mounted into the container at `/mnt/work/`.
      - The command `g++ main.cpp -o main -O2` is executed, creating an executable file `main` in `/mnt/work/`.
      - Because this directory is on the host, the `main` executable persists after the compilation container is destroyed.
  - **Step 2 (Judging)**:
      - A new container is created from the `zjusct/oj-judger:latest` image.
      - The same host working directory, which now contains both `main.cpp` and the compiled `main` executable, is mounted into this new container at `/mnt/work/`.
      - The command `/judge --bin ./main` is executed to run the user's program against test cases.

## Result Reporting

The **final step** of the workflow has a special responsibility: it must report the judging result back to CSOJ. It does this by printing a specific JSON object to its **standard output (stdout)**.

### Result JSON Format

The system parses a JSON object with the following structure:

```json
{
  "score": 100,
  "performance": 123.45,
  "info": {
    "message": "Accepted",
    "time": "54ms",
    "memory": "1.2MB"
  }
}
```

  - `score` (integer): Used when the problem's `score.mode` is `"score"`.
  - `performance` (number): Used when the problem's `score.mode` is `"performance"`.
  - `info` (object, optional): A map containing any other relevant details. This data is stored in the submission record and can be displayed to the user.

If the final step exits with a non-zero status code or fails to produce a valid JSON output, the submission will be marked as `Failed`.

This step-by-step, containerized approach allows for immense flexibility. You can create workflows for any programming language, use custom interactor programs, run static analysis tools, or perform any other action required for judging.
