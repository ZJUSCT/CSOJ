package judger

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// AppState holds the shared, reloadable state of contests and problems.
type AppState struct {
	sync.RWMutex
	Contests            map[string]*Contest
	Problems            map[string]*Problem
	ProblemToContestMap map[string]*Contest
}

// PoolState is the in-memory view of a node-pool (caps + pause flag).
// K8s is the scheduler; we do not track per-core usage here.
type PoolState struct {
	Name         string
	NodeSelector map[string]string
	CPU          int
	Memory       int64
	IsPaused     bool
}

type ClusterState struct {
	sync.Mutex
	Name       string
	Namespace  string
	k8s        kubernetes.Interface
	dyn        dynamic.Interface
	pools      map[string]*PoolState
	sem        chan struct{}
	queue      chan QueuedSubmission
	mpiEnabled bool
}

type QueuedSubmission struct {
	Submission *models.Submission
	Problem    *Problem
}

type Scheduler struct {
	cfg        *config.Config
	db         *gorm.DB
	appState   *AppState
	clusters   map[string]*ClusterState
	dispatcher *Dispatcher
}

func NewScheduler(cfg *config.Config, db *gorm.DB, appState *AppState) *Scheduler {
	clusters := make(map[string]*ClusterState)

	// Load all pool caps from the DB.
	allPools, err := database.GetAllClusterPools(db)
	if err != nil {
		zap.S().Fatalf("failed to load cluster node pools: %v", err)
	}
	poolsByCluster := make(map[string]map[string]*PoolState)
	for _, p := range allPools {
		if poolsByCluster[p.ClusterName] == nil {
			poolsByCluster[p.ClusterName] = make(map[string]*PoolState)
		}
		poolsByCluster[p.ClusterName][p.PoolName] = &PoolState{
			Name:         p.PoolName,
			NodeSelector: toStringMap(p.NodeSelector),
			CPU:          p.CPU,
			Memory:       p.Memory,
			IsPaused:     p.IsPaused,
		}
	}

	for i := range cfg.Cluster {
		cc := cfg.Cluster[i]
		conc := cc.Concurrency
		if conc <= 0 {
			conc = 1
		}
		// Build the K8s clientset + dynamic client from the kubeconfig.
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: cc.Kubeconfig},
			&clientcmd.ConfigOverrides{CurrentContext: cc.Context},
		)
		restCfg, err := loader.ClientConfig()
		if err != nil {
			zap.S().Fatalf("failed to build rest config for cluster %s: %v", cc.Name, err)
		}
		cs, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			zap.S().Fatalf("failed to build clientset for cluster %s: %v", cc.Name, err)
		}
		dyn, err := dynamic.NewForConfig(restCfg)
		if err != nil {
			zap.S().Fatalf("failed to build dynamic client for cluster %s: %v", cc.Name, err)
		}

		pools := poolsByCluster[cc.Name]
		if pools == nil {
			pools = make(map[string]*PoolState)
		}
		// Ensure a (disabled) PoolState exists for every config-declared pool name,
		// so admins can PUT caps to enable it.
		for _, np := range cc.NodePools {
			if _, ok := pools[np.Name]; !ok {
				pools[np.Name] = &PoolState{Name: np.Name}
			}
		}

		cluster := &ClusterState{
			Name:      cc.Name,
			Namespace: cc.Namespace,
			k8s:       cs,
			dyn:       dyn,
			pools:     pools,
			sem:       make(chan struct{}, conc),
			queue:     make(chan QueuedSubmission, 1024),
		}
		// Probe the MPI operator CRD (best-effort; non-fatal).
		cluster.mpiEnabled = probeMPIOperator(cs, cc.Namespace)
		clusters[cc.Name] = cluster
	}

	s := &Scheduler{cfg: cfg, db: db, appState: appState, clusters: clusters}
	s.dispatcher = NewDispatcher(cfg, db, s, appState)
	return s
}

// Dispatcher exposes the dispatcher (used by admin handlers for interrupt).
func (s *Scheduler) Dispatcher() *Dispatcher { return s.dispatcher }

func (s *Scheduler) Run() {
	for name, cluster := range s.clusters {
		go s.clusterWorker(name, cluster)
	}
}

func (s *Scheduler) clusterWorker(name string, cluster *ClusterState) {
	for job := range cluster.queue {
		// Refetch; the submission may have been interrupted while queued.
		var sub models.Submission
		if err := s.db.First(&sub, "id = ?", job.Submission.ID).Error; err != nil {
			zap.S().Warnf("submission %s vanished from DB; skipping", job.Submission.ID)
			continue
		}
		if sub.Status != models.StatusQueued {
			continue
		}
		// Acquire a concurrency slot.
		cluster.sem <- struct{}{}
		// Pick a pool (FIFO over non-paused pools with enough caps).
		pool := s.findAvailablePool(cluster, job.Problem.CPU, job.Problem.Memory)
		if pool == nil {
			// No pool; release the slot and requeue after a delay.
			<-cluster.sem
			go func(j QueuedSubmission) {
				time.Sleep(time.Second)
				s.clusters[name].queue <- j
			}(job)
			continue
		}
		sub.Node = pool.Name
		sub.Status = models.StatusRunning
		database.UpdateSubmission(s.db, &sub)
		go s.dispatcher.Dispatch(&sub, job.Problem, cluster, pool)
	}
}

func (s *Scheduler) findAvailablePool(cluster *ClusterState, cpu int, mem int64) *PoolState {
	cluster.Lock()
	defer cluster.Unlock()
	for _, p := range cluster.pools {
		if p.IsPaused || p.CPU == 0 || p.Memory == 0 {
			continue
		}
		if p.CPU >= cpu && p.Memory >= mem {
			return p
		}
	}
	return nil
}

func (s *Scheduler) Submit(submission *models.Submission, problem *Problem) {
	cluster, ok := s.clusters[problem.Cluster]
	if !ok {
		submission.Status = models.StatusFailed
		submission.Info = models.JSONMap{"error": "Invalid cluster specified in problem definition"}
		database.UpdateSubmission(s.db, submission)
		pubsubPublishError(submission.ID, "Invalid cluster specified in problem definition")
		return
	}
	cluster.queue <- QueuedSubmission{Submission: submission, Problem: problem}
}

// ReleaseSlot frees one concurrency slot (called by the dispatcher on completion).
func (s *Scheduler) ReleaseSlot(clusterName string) {
	if c, ok := s.clusters[clusterName]; ok {
		select {
		case <-c.sem:
		default:
		}
	}
}

// GetClusterStates returns a snapshot of every cluster's pools + queue length.
func (s *Scheduler) GetClusterStates() map[string]ClusterStateSnapshot {
	out := make(map[string]ClusterStateSnapshot)
	for name, c := range s.clusters {
		c.Lock()
		pools := make(map[string]PoolState, len(c.pools))
		for k, v := range c.pools {
			cp := *v
			pools[k] = cp
		}
		c.Unlock()
		out[name] = ClusterStateSnapshot{
			Name:        name,
			Namespace:   c.Namespace,
			Pools:       pools,
			MPIEnabled:  c.mpiEnabled,
			QueueLength: len(c.queue),
			Concurrency: cap(c.sem),
		}
	}
	return out
}

type ClusterStateSnapshot struct {
	Name        string
	Namespace   string
	Pools       map[string]PoolState
	MPIEnabled  bool
	QueueLength int
	Concurrency int
}

func (s *Scheduler) GetQueueLengths() map[string]int {
	out := make(map[string]int)
	for name, c := range s.clusters {
		out[name] = len(c.queue)
	}
	return out
}

// PausePool / ResumePool / UpdatePoolResources / SetConcurrency.

func (s *Scheduler) PausePool(clusterName, pool string) error {
	return s.mutatePool(clusterName, pool, func(p *PoolState) { p.IsPaused = true })
}

func (s *Scheduler) ResumePool(clusterName, pool string) error {
	return s.mutatePool(clusterName, pool, func(p *PoolState) { p.IsPaused = false })
}

func (s *Scheduler) UpdatePoolResources(clusterName, pool string, cpu int, mem int64) error {
	return s.mutatePool(clusterName, pool, func(p *PoolState) { p.CPU = cpu; p.Memory = mem })
}

func (s *Scheduler) mutatePool(clusterName, pool string, fn func(*PoolState)) error {
	c, ok := s.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster %q not found", clusterName)
	}
	c.Lock()
	defer c.Unlock()
	p, ok := c.pools[pool]
	if !ok {
		return fmt.Errorf("pool %q not found", pool)
	}
	fn(p)
	return nil
}

func (s *Scheduler) SetConcurrency(clusterName string, n int) error {
	// Concurrency is fixed at construction (channel size); resizing a live channel
	// is unsafe. Document this as a config-reload-only setting for now.
	return fmt.Errorf("concurrency changes require a restart")
}

// DeleteSubmissionResources deletes all pods + MPIJobs for a submission across its cluster.
// Used by the interrupt handlers to clean up a running submission's K8s resources.
func (s *Scheduler) DeleteSubmissionResources(clusterName, subID string) error {
	c, ok := s.clusters[clusterName]
	if !ok {
		return nil
	}
	km := NewKubeManager(c.k8s, c.dyn, c.Namespace)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = km.DeleteSubmissionPods(ctx, subID)
	_ = km.DeleteSubmissionMPIJobs(ctx, subID)
	return nil
}

// RequeuePendingSubmissions re-enqueues Queued submissions on startup.
func RequeuePendingSubmissions(db *gorm.DB, s *Scheduler, appState *AppState) error {
	var pending []models.Submission
	if err := db.Where("status = ?", models.StatusQueued).Order("created_at asc").Find(&pending).Error; err != nil {
		return err
	}
	if len(pending) == 0 {
		zap.S().Info("no pending submissions to requeue")
		return nil
	}
	zap.S().Infof("requeueing %d pending submissions", len(pending))
	appState.RLock()
	defer appState.RUnlock()
	for i := range pending {
		prob, ok := appState.Problems[pending[i].ProblemID]
		if !ok {
			zap.S().Warnf("problem %s for submission %s not found; skipping", pending[i].ProblemID, pending[i].ID)
			continue
		}
		s.Submit(&pending[i], prob)
	}
	return nil
}

// helpers

func toStringMap(m models.JSONMap) map[string]string {
	out := make(map[string]string)
	for k, v := range m {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}

func pubsubPublishError(topic, msg string) {
	pubsub.GetBroker().Publish(topic, pubsub.FormatMessage("error", msg))
}
