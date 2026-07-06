package judger

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
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
	Name       string              `json:"name"`
	Docker     config.DockerConfig `json:"docker"`
	CPU        int                 `json:"cpu"`
	Memory     int64               `json:"memory"`
	UsedMemory int64               `json:"used_memory"`
	UsedCores  []bool              `json:"used_cores"`
	IsPaused   bool                `json:"is_paused"`
}

type NodeDetail struct {
	Name       string              `json:"name"`
	Docker     config.DockerConfig `json:"docker"`
	CPU        int                 `json:"cpu"`
	Memory     int64               `json:"memory"`
	UsedMemory int64               `json:"used_memory"`
	UsedCores  []bool              `json:"used_cores"`
	IsPaused   bool                `json:"is_paused"`
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

	// Load runtime-mutable resource caps from the DB, keyed by (cluster, node).
	dbNodes, err := database.GetAllClusterNodes(db)
	if err != nil {
		zap.S().Fatalf("failed to load cluster nodes from DB: %v", err)
	}
	caps := make(map[string]models.ClusterNode, len(dbNodes))
	for _, n := range dbNodes {
		caps[n.ClusterName+"\x00"+n.NodeName] = n
	}

	for i := range cfg.Cluster {
		cluster := cfg.Cluster[i]
		clusterState := &ClusterState{
			Cluster: &cluster,
			Nodes:   make(map[string]*NodeState),
		}
		for j := range cluster.Nodes {
			node := cluster.Nodes[j]
			cap, ok := caps[cluster.Name+"\x00"+node.Name]
			if !ok {
				zap.S().Warnf("node %s/%s has no DB resource caps; skipping", cluster.Name, node.Name)
				continue
			}
			nodeCores := make([]bool, cap.CPU)
			clusterState.Nodes[node.Name] = &NodeState{
				Name:       node.Name,
				Docker:     node.Docker,
				CPU:        cap.CPU,
				Memory:     cap.Memory,
				UsedMemory: 0,
				UsedCores:  nodeCores,
				IsPaused:   false,
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
			// Create a copy to avoid exposing internal state directly
			nodeSnapshots[nodeName] = &NodeState{
				Name:       node.Name,
				Docker:     node.Docker,
				CPU:        node.CPU,
				Memory:     node.Memory,
				UsedMemory: node.UsedMemory,
				IsPaused:   node.IsPaused,
				UsedCores:  append([]bool(nil), node.UsedCores...),
			}
			node.Unlock()
		}
		clusterConfigCopy := *cluster.Cluster
		snapshot[name] = ClusterState{
			Cluster: &clusterConfigCopy,
			Nodes:   nodeSnapshots,
		}
		cluster.Unlock()
	}
	return snapshot
}

func (s *Scheduler) GetNodeDetails(clusterName, nodeName string) (*NodeDetail, error) {
	cluster, ok := s.clusters[clusterName]
	if !ok {
		return nil, fmt.Errorf("cluster '%s' not found", clusterName)
	}

	node, ok := cluster.Nodes[nodeName]
	if !ok {
		return nil, fmt.Errorf("node '%s' not found in cluster '%s'", nodeName, clusterName)
	}

	node.Lock()
	defer node.Unlock()

	details := &NodeDetail{
		Name:       node.Name,
		Docker:     node.Docker,
		CPU:        node.CPU,
		Memory:     node.Memory,
		UsedMemory: node.UsedMemory,
		IsPaused:   node.IsPaused,
		UsedCores:  append([]bool(nil), node.UsedCores...),
	}

	return details, nil
}

func (s *Scheduler) PauseNode(clusterName, nodeName string) error {
	cluster, ok := s.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster '%s' not found", clusterName)
	}

	node, ok := cluster.Nodes[nodeName]
	if !ok {
		return fmt.Errorf("node '%s' not found in cluster '%s'", nodeName, clusterName)
	}

	node.Lock()
	defer node.Unlock()
	node.IsPaused = true
	zap.S().Warnf("admin paused node '%s/%s'", clusterName, nodeName)
	return nil
}

func (s *Scheduler) ResumeNode(clusterName, nodeName string) error {
	cluster, ok := s.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster '%s' not found", clusterName)
	}

	node, ok := cluster.Nodes[nodeName]
	if !ok {
		return fmt.Errorf("node '%s' not found in cluster '%s'", nodeName, clusterName)
	}

	node.Lock()
	defer node.Unlock()
	node.IsPaused = false
	zap.S().Infof("admin resumed node '%s/%s'", clusterName, nodeName)
	return nil
}

func (s *Scheduler) GetQueueLengths() map[string]int {
	lengths := make(map[string]int)
	for name, queue := range s.queues {
		lengths[name] = len(queue)
	}
	return lengths
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
		var allocatedCores []int
		zap.S().Infof("processing submission %s for cluster '%s'", job.Submission.ID, clusterName)

		for {
			var currentSub models.Submission
			if err := s.db.First(&currentSub, "id = ?", job.Submission.ID).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					zap.S().Warnf("submission %s was deleted from DB, dropping job.", job.Submission.ID)
				} else {
					zap.S().Errorf("failed to refetch submission %s from DB: %v", job.Submission.ID, err)
				}
				node = nil
				break
			}
			if currentSub.Status != models.StatusQueued {
				zap.S().Infof("submission %s is no longer in queued status (%s), skipping processing.", currentSub.ID, currentSub.Status)
				node = nil
				break
			}

			job.Submission = &currentSub

			zap.S().Debugf("searching for available node for submission %s in cluster %s", currentSub.ID, clusterName)
			node, allocatedCores = s.findAvailableNode(clusterName, job.Problem.CPU, job.Problem.Memory)
			if node != nil {
				break
			}

			time.Sleep(1 * time.Second)
		}

		if node == nil {
			continue
		}

		zap.S().Infof("node %s assigned to submission %s", node.Name, job.Submission.ID)

		var coreStrs []string
		for _, c := range allocatedCores {
			coreStrs = append(coreStrs, strconv.Itoa(c))
		}

		job.Submission.Node = node.Name
		job.Submission.Status = models.StatusRunning
		job.Submission.AllocatedCores = strings.Join(coreStrs, ",")

		if err := s.db.Save(job.Submission).Error; err != nil {
			zap.S().Errorf("failed to update submission status for %s: %v", job.Submission.ID, err)
			s.ReleaseResources(job.Problem.Cluster, node.Name, allocatedCores, job.Problem.Memory)
			continue
		}

		go s.dispatcher.Dispatch(job.Submission, job.Problem, node, allocatedCores)
	}
}

func (s *Scheduler) findAvailableNode(clusterName string, requiredCPU int, requiredMemory int64) (*NodeState, []int) {
	cluster, ok := s.clusters[clusterName]
	if !ok {
		return nil, nil
	}

	cluster.Lock()
	defer cluster.Unlock()

	for _, node := range cluster.Nodes {
		node.Lock()
		if node.IsPaused {
			node.Unlock()
			continue
		}

		if node.Memory-node.UsedMemory >= requiredMemory {
			startCore := -1
			if requiredCPU > 0 {
				for i := 0; i <= len(node.UsedCores)-requiredCPU; i += requiredCPU {
					isBlockFree := true
					for j := 0; j < requiredCPU; j++ {
						if node.UsedCores[i+j] {
							isBlockFree = false
							break
						}
					}
					if isBlockFree {
						startCore = i
						break
					}
				}
			} else {
				startCore = -2
			}

			if startCore != -1 {
				allocatedCores := make([]int, requiredCPU)
				if startCore != -2 {
					for i := 0; i < requiredCPU; i++ {
						coreID := startCore + i
						node.UsedCores[coreID] = true
						allocatedCores[i] = coreID
					}
				}
				node.UsedMemory += requiredMemory
				node.Unlock()
				return node, allocatedCores
			}
		}
		node.Unlock()
	}
	return nil, nil
}

func (s *Scheduler) ReleaseResources(clusterName, nodeName string, coresToRelease []int, memory int64) {
	if cluster, ok := s.clusters[clusterName]; ok {
		if node, ok := cluster.Nodes[nodeName]; ok {
			node.Lock()
			for _, coreID := range coresToRelease {
				if coreID >= 0 && coreID < len(node.UsedCores) {
					node.UsedCores[coreID] = false
				}
			}
			node.UsedMemory -= memory
			if node.UsedMemory < 0 {
				node.UsedMemory = 0
			}
			node.Unlock()
			var coreStrs []string
			for _, c := range coresToRelease {
				coreStrs = append(coreStrs, strconv.Itoa(c))
			}
			zap.S().Infof("released resources (cores: [%s], mem: %dMB) from node %s", strings.Join(coreStrs, ","), memory, nodeName)
		}
	}
}

// UpdateNodeResources updates the in-memory CPU/Memory caps for a node
// after an admin edits the ClusterNode DB row.
func (s *Scheduler) UpdateNodeResources(clusterName, nodeName string, cpu int, memory int64) error {
	cluster, ok := s.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster '%s' not found", clusterName)
	}
	node, ok := cluster.Nodes[nodeName]
	if !ok {
		return fmt.Errorf("node '%s' not found in cluster '%s'", nodeName, clusterName)
	}
	node.Lock()
	defer node.Unlock()
	node.CPU = cpu
	node.Memory = memory
	// Resize the UsedCores slice; preserve existing usage where possible.
	newCores := make([]bool, cpu)
	copy(newCores, node.UsedCores)
	node.UsedCores = newCores
	return nil
}
