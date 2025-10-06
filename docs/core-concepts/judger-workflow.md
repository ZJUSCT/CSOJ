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
  - image: "gcc:latest"
    timeout: 10
    show: true
    steps:
      - ["g++", "main.cpp", "-o", "main", "-O2"]

  # Step 2: Judging
  - image: "zjusct/oj-judger:latest"
    timeout: 5
    show: false
    steps:
      - ["/judge", "--bin", "./main"]
```

### How It Works

  - **File System**: When the first step starts, the user's submitted files are copied into the container's working directory, `/mnt/work/`. This directory is persistent across all steps for a single submission.
  - **Step 1 (Compilation)**:
      - A container is created from the `gcc:latest` image.
      - The command `g++ main.cpp -o main -O2` is executed inside the container.
      - This command compiles the user's code and creates an executable file named `main` inside `/mnt/work/`.
      - After this step completes, the container is destroyed, but the `/mnt/work/` directory (containing `main.cpp` and the new `main` executable) is preserved for the next step.
  - **Step 2 (Judging)**:
      - A new container is created from the `zjusct/oj-judger:latest` image.
      - The `/mnt/work/` directory, which now contains the compiled `main` executable, is mounted into this new container.
      - The command `/judge --bin ./main` is executed. This is a hypothetical script or program that runs the user's executable against test cases.
      - The `/judge` program is responsible for determining the score and result.

## Result Reporting

The **final step** of the workflow has a special responsibility: it must report the judging result back to CSOJ. It does this by printing a specific JSON object to its **standard output (stdout)**.

### Result JSON Format

```json
{
  "score": 100,
  "info": {
    "message": "Accepted",
    "time": "54ms",
    "memory": "1.2MB"
  }
}
```

  - `score` (integer, required): The final score for the submission.
  - `info` (object, required): A map containing any other relevant details. This data is stored in the submission record and can be displayed to the user.

If the final step exits with a non-zero status code or fails to produce a valid JSON output, the submission will be marked as `Failed`.

This step-by-step, containerized approach allows for immense flexibility. You can create workflows for any programming language, use custom interactor programs, run static analysis tools, or perform any other action required for judging.
