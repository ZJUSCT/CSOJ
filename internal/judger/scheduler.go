package judger

import (
	"sync"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AppState holds the shared, reloadable state of contests and problems.
type AppState struct {
	sync.RWMutex
	Contests            map[string]*Contest
	Problems            map[string]*Problem
	ProblemToContestMap map[string]*Contest
}

type NodeState struct {
	sync.Mutex
	*config.Node
	UsedCPU    int   `json:"used_cpu"`
	UsedMemory int64 `json:"used_memory"`
}

type ClusterState struct {
	sync.Mutex
	*config.Cluster
	Nodes map[string]*NodeState `json:"nodes"`
}

type QueuedSubmission struct {
	Submission *models.Submission
	Problem    *Problem
}

type Scheduler struct {
	cfg        *config.Config
	db         *gorm.DB
	clusters   map[string]*ClusterState
	appState   *AppState
	queues     map[string]chan QueuedSubmission
	dispatcher *Dispatcher
}

func NewScheduler(cfg *config.Config, db *gorm.DB, appState *AppState) *Scheduler {
	clusters := make(map[string]*ClusterState)
	queues := make(map[string]chan QueuedSubmission)
	for i := range cfg.Cluster {
		cluster := cfg.Cluster[i]
		clusterState := &ClusterState{
			Cluster: &cluster,
			Nodes:   make(map[string]*NodeState),
		}
		for j := range cluster.Nodes {
			node := cluster.Nodes[j]
			clusterState.Nodes[node.Name] = &NodeState{
				Node:       &node,
				UsedCPU:    0,
				UsedMemory: 0,
			}
		}
		clusters[cluster.Name] = clusterState
		queues[cluster.Name] = make(chan QueuedSubmission, 1024)
	}

	scheduler := &Scheduler{
		cfg:      cfg,
		db:       db,
		clusters: clusters,
		queues:   queues,
		appState: appState,
	}
	scheduler.dispatcher = NewDispatcher(cfg, db, scheduler, appState)
	return scheduler
}

// RequeuePendingSubmissions loads submissions with 'Queued' status from the DB
// and adds them back to the scheduler's queue on startup.
func RequeuePendingSubmissions(db *gorm.DB, s *Scheduler, appState *AppState) error {
	var pendingSubs []models.Submission
	if err := db.Model(&models.Submission{}).Where("status = ?", models.StatusQueued).Order("created_at asc").Find(&pendingSubs).Error; err != nil {
		return err
	}

	if len(pendingSubs) == 0 {
		zap.S().Info("no pending submissions to requeue")
		return nil
	}

	zap.S().Infof("requeueing %d pending submissions...", len(pendingSubs))
	appState.RLock()
	defer appState.RUnlock()
	for _, sub := range pendingSubs {
		submission := sub // Create a new variable to avoid pointer issues with the loop variable
		problem, ok := appState.Problems[submission.ProblemID]
		if !ok {
			zap.S().Warnf("problem %s for submission %s not found, skipping requeue", submission.ProblemID, submission.ID)
			continue
		}
		s.Submit(&submission, problem)
	}
	zap.S().Info("finished requeueing pending submissions")
	return nil
}

func (s *Scheduler) GetClusterStates() map[string]ClusterState {
	snapshot := make(map[string]ClusterState)
	for name, cluster := range s.clusters {
		cluster.Lock()
		nodeSnapshots := make(map[string]*NodeState)
		for nodeName, node := range cluster.Nodes {
			node.Lock()
			nodeSnapshots[nodeName] = &NodeState{
				Node:       node.Node,
				UsedCPU:    node.UsedCPU,
				UsedMemory: node.UsedMemory,
			}
			node.Unlock()
		}
		snapshot[name] = ClusterState{
			Cluster: cluster.Cluster,
			Nodes:   nodeSnapshots,
		}
		cluster.Unlock()
	}
	return snapshot
}

func (s *Scheduler) Submit(submission *models.Submission, problem *Problem) {
	clusterName := problem.Cluster
	if queue, ok := s.queues[clusterName]; ok {
		queue <- QueuedSubmission{Submission: submission, Problem: problem}
		zap.S().Infof("submission %s for problem %s added to queue for cluster '%s'", submission.ID, problem.ID, clusterName)
	} else {
		zap.S().Errorf("submission %s for problem %s has an invalid cluster '%s', dropping", submission.ID, problem.ID, clusterName)
		// Mark submission as failed
		submission.Status = models.StatusFailed
		submission.Info = models.JSONMap{"error": "Invalid cluster specified in problem definition"}
		if err := s.db.Save(submission).Error; err != nil {
			zap.S().Errorf("failed to update submission %s status to failed: %v", submission.ID, err)
		}
	}
}

func (s *Scheduler) Run() {
	for clusterName, queue := range s.queues {
		go s.clusterWorker(clusterName, queue)
	}
}

func (s *Scheduler) clusterWorker(clusterName string, queue <-chan QueuedSubmission) {
	zap.S().Infof("starting worker for cluster '%s'", clusterName)
	for job := range queue {
		var node *NodeState
		zap.S().Infof("processing submission %s for cluster '%s'", job.Submission.ID, clusterName)

		// This loop implements the FIFO retry logic.
		// The worker will be blocked here until resources are available for this job.
		for {
			// Refetch from DB to check for interruptions while waiting.
			var currentSub models.Submission
			if err := s.db.First(&currentSub, "id = ?", job.Submission.ID).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					zap.S().Warnf("submission %s was deleted from DB (likely via reload), dropping job.", job.Submission.ID)
				} else {
					zap.S().Errorf("failed to refetch submission %s from DB: %v", job.Submission.ID, err)
				}
				node = nil // Ensure node is nil to break outer loop
				break
			}
			if currentSub.Status != models.StatusQueued {
				zap.S().Infof("submission %s is no longer in queued status (%s), skipping processing.", currentSub.ID, currentSub.Status)
				node = nil // Ensure node is nil to break outer loop
				break
			}

			// Use the latest state from the database.
			job.Submission = &currentSub

			zap.S().Debugf("searching for available node for submission %s in cluster %s", currentSub.ID, clusterName)
			node = s.findAvailableNode(clusterName, job.Problem.CPU, job.Problem.Memory)
			if node != nil {
				break // Found a node, exit the retry loop.
			}

			// Wait before retrying
			time.Sleep(1 * time.Second)
		}

		if node == nil {
			// This happens if the job was dropped (e.g., DB error or cancelled).
			// The loop above will have logged the reason.
			continue
		}

		zap.S().Infof("node %s assigned to submission %s", node.Name, job.Submission.ID)

		job.Submission.Node = node.Name
		job.Submission.Status = models.StatusRunning
		if err := s.db.Save(job.Submission).Error; err != nil {
			zap.S().Errorf("failed to update submission status for %s: %v", job.Submission.ID, err)
			s.ReleaseResources(job.Problem.Cluster, node.Name, job.Problem.CPU, job.Problem.Memory)
			continue
		}

		go s.dispatcher.Dispatch(job.Submission, job.Problem, node)
	}
}

func (s *Scheduler) findAvailableNode(clusterName string, requiredCPU int, requiredMemory int64) *NodeState {
	cluster, ok := s.clusters[clusterName]
	if !ok {
		return nil
	}

	cluster.Lock()
	defer cluster.Unlock()

	for _, node := range cluster.Nodes {
		node.Lock()
		if node.CPU-node.UsedCPU >= requiredCPU && node.Memory-node.UsedMemory >= requiredMemory {
			node.UsedCPU += requiredCPU
			node.UsedMemory += requiredMemory
			node.Unlock()
			return node
		}
		node.Unlock()
	}
	return nil
}

func (s *Scheduler) ReleaseResources(clusterName, nodeName string, cpu int, memory int64) {
	if cluster, ok := s.clusters[clusterName]; ok {
		if node, ok := cluster.Nodes[nodeName]; ok {
			node.Lock()
			node.UsedCPU -= cpu
			if node.UsedCPU < 0 {
				node.UsedCPU = 0
			}
			node.UsedMemory -= memory
			if node.UsedMemory < 0 {
				node.UsedMemory = 0
			}
			node.Unlock()
			zap.S().Infof("released resources (cpu: %d, mem: %dMB) from node %s", cpu, memory, nodeName)
		}
	}
}
