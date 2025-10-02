package judger

import (
	"sync"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

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
	contests   map[string]*Contest // for dispatcher to find contestID
	queue      chan QueuedSubmission
	dispatcher *Dispatcher
}

func NewScheduler(cfg *config.Config, db *gorm.DB) *Scheduler {
	clusters := make(map[string]*ClusterState)
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
	}

	scheduler := &Scheduler{
		cfg:      cfg,
		db:       db,
		clusters: clusters,
		queue:    make(chan QueuedSubmission, 1024),
	}
	scheduler.dispatcher = NewDispatcher(cfg, db, scheduler)
	return scheduler
}

// RequeuePendingSubmissions loads submissions with 'Queued' status from the DB
// and adds them back to the scheduler's queue on startup.
func RequeuePendingSubmissions(db *gorm.DB, s *Scheduler, problems map[string]*Problem) error {
	var pendingSubs []models.Submission
	if err := db.Model(&models.Submission{}).Where("status = ?", models.StatusQueued).Order("created_at asc").Find(&pendingSubs).Error; err != nil {
		return err
	}

	if len(pendingSubs) == 0 {
		zap.S().Info("no pending submissions to requeue")
		return nil
	}

	zap.S().Infof("requeueing %d pending submissions...", len(pendingSubs))
	for _, sub := range pendingSubs {
		submission := sub // Create a new variable to avoid pointer issues with the loop variable
		problem, ok := problems[submission.ProblemID]
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
	s.queue <- QueuedSubmission{Submission: submission, Problem: problem}
	zap.S().Infof("submission %s for problem %s added to queue", submission.ID, problem.ID)
}

func (s *Scheduler) Run() {
	contests, _, _ := LoadAllContestsAndProblems(s.cfg.Contest)
	s.dispatcher.scheduler.contests = contests // allow dispatcher to find contestID

	for job := range s.queue {
		go s.process(job)
	}
}

func (s *Scheduler) process(job QueuedSubmission) {
	// Refetch from DB to check for interruptions while in queue
	var currentSub models.Submission
	if err := s.db.First(&currentSub, "id = ?", job.Submission.ID).Error; err != nil {
		zap.S().Errorf("failed to refetch submission %s from DB: %v", job.Submission.ID, err)
		return // Can't proceed, drop the job
	}

	if currentSub.Status != models.StatusQueued {
		zap.S().Infof("submission %s is no longer in queued status (%s), skipping processing.", currentSub.ID, currentSub.Status)
		return // Job was cancelled/interrupted while in queue
	}

	zap.S().Infof("searching for available node for submission %s", currentSub.ID)
	node := s.findAvailableNode(job.Problem.Cluster, job.Problem.CPU, job.Problem.Memory)

	if node == nil {
		zap.S().Warnf("no available node for submission %s, requeueing", currentSub.ID)
		s.queue <- job // Requeue
		return
	}

	zap.S().Infof("node %s assigned to submission %s", node.Name, currentSub.ID)

	currentSub.Node = node.Name
	currentSub.Status = models.StatusRunning
	if err := s.db.Save(&currentSub).Error; err != nil {
		zap.S().Errorf("failed to update submission status: %v", err)
		s.ReleaseResources(job.Problem.Cluster, node.Name, job.Problem.CPU, job.Problem.Memory)
		return
	}

	go s.dispatcher.Dispatch(&currentSub, job.Problem, node)
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
