# Scheduler & Cluster

CSOJ is designed for scalability. It can distribute the judging load across multiple machines, which are organized into clusters. The Scheduler is the brain that manages this process.

## Key Concepts

- **Node**: A single machine (physical or virtual) that is capable of running judging tasks. Each node must have Docker installed and accessible via a TCP socket.
- **Cluster**: A logical grouping of one or more Nodes. You might have a cluster of high-CPU machines, a cluster of machines with GPUs, or just a single "default" cluster.
- **Scheduler**: The central component in CSOJ that receives all submissions and assigns them to an appropriate Node for execution.

## Configuration

Clusters and nodes are defined in the main `config.yaml` file.

```yaml
# config.yaml
cluster:
  - name: "default-cluster" # Cluster 1
    node:
      - name: "node-1"
        cpu: 4       # 4 CPU cores available
        memory: 4096 # 4096 MB memory available
        docker: "tcp://192.168.1.101:2375"
      - name: "node-2"
        cpu: 8
        memory: 8192
        docker: "tcp://192.168.1.102:2375"
  
  - name: "gpu-cluster" # Cluster 2
    node:
      - name: "gpu-node-1"
        cpu: 16
        memory: 32768
        docker: "tcp://192.168.1.201:2375"
```

Problems are then assigned to a specific cluster in their `problem.yaml` configuration.

```yaml
# problem.yaml
id: "cuda-problem"
# ...
cluster: "gpu-cluster" # This problem will only be judged on the gpu-cluster
cpu: 2                 # It requires 2 CPU cores
memory: 4096           # It requires 4096 MB of memory
# ...
```

## The Scheduling Process

1.  **Submission Received**: A user submits a solution to the "cuda-problem".
2.  **Queueing**: The Scheduler sees that this problem belongs to the `"gpu-cluster"`. It places the submission into the queue specifically for that cluster. Each cluster has its own independent FIFO (First-In, First-Out) queue.
3.  **Resource Check**: The Scheduler continuously checks the nodes within the `"gpu-cluster"` (in this case, only `"gpu-node-1"`). It looks for a node that can satisfy the resource request of the problem (2 CPU cores and 4096 MB memory).
4.  **Resource Allocation**: Let's say `"gpu-node-1"` is currently idle. Its available resources are 16 CPU and 32768 MB. This is sufficient. The Scheduler:
      - **Locks** the requested resources on `"gpu-node-1"`. The node's available resources are now tracked internally as 14 CPU and 28672 MB.
      - Assigns the submission to `"gpu-node-1"`.
      - Updates the submission's status to `Running`.
5.  **Dispatching**: The submission is dispatched to the [Judger Workflow](https://www.google.com/search?q=./judger-workflow.md) for execution on `"gpu-node-1"`.
6.  **Resource Release**: Once the judging process is complete (whether it succeeds or fails), the allocated resources (2 CPU, 4096 MB) are released, and the available resources on `"gpu-node-1"` are updated back to 16 CPU and 32768 MB. The Scheduler can now assign another task to it.

This resource-aware scheduling ensures that nodes are not overloaded and that submissions are processed efficiently as resources become available.
