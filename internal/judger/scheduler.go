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
	queueMode  string
}

type QueuedSubmission struct {
	Submission *models.Submission
	Problem    *Problem
}

type Scheduler struct {
	db         *gorm.DB
	settings   *config.SettingsStore
	cfg        *config.Config
	appState   *AppState
	clusters   map[string]*ClusterState
	mu         sync.RWMutex // guards the clusters map swap in ReloadClusters
	reloadMu   sync.Mutex
	running    bool
	dispatcher *Dispatcher
}

// NewScheduler builds a Scheduler whose cluster clientsets are read from the
// `clusters` DB table (kubeconfig stored as text). The `settings` param is
// accepted for API stability and future use; clusters are read from the DB.
// The `cfg` param threads boot facts (e.g. Storage.SubmissionContent) to the
// dispatcher, which needs them at runtime when reading result.json.
func NewScheduler(db *gorm.DB, settings *config.SettingsStore, cfg *config.Config, appState *AppState) *Scheduler {
	clusters := make(map[string]*ClusterState)

	dbClusters, err := database.GetAllClusters(db)
	if err != nil {
		zap.S().Fatalf("failed to load clusters from DB: %v", err)
	}
	for _, cc := range dbClusters {
		cs, warning, err := buildClusterState(db, cc)
		if err != nil {
			zap.S().Warnf("cluster %s failed to init: %v (skipping)", cc.Name, err)
			continue
		}
		if warning != "" {
			zap.S().Warn(warning)
		}
		clusters[cc.Name] = cs
	}

	s := &Scheduler{db: db, settings: settings, cfg: cfg, appState: appState, clusters: clusters}
	s.dispatcher = NewDispatcher(cfg, db, s, appState)
	return s
}

// buildClusterState parses the kubeconfig text in cc.Kubeconfig, builds the
// K8s clientset + dynamic client, loads the cluster's node-pool caps from the
// DB, and returns a ready *ClusterState (without a queue — the caller sets it).
func buildClusterState(db *gorm.DB, cc models.Cluster) (*ClusterState, string, error) {
	loaded, err := clientcmd.Load([]byte(cc.Kubeconfig))
	if err != nil {
		return nil, "", fmt.Errorf("parse kubeconfig: %w", err)
	}
	clientCfg := clientcmd.NewNonInteractiveClientConfig(*loaded, cc.Context, &clientcmd.ConfigOverrides{}, nil)
	restCfg, err := clientCfg.ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("build rest config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, "", fmt.Errorf("build clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, "", fmt.Errorf("build dynamic client: %w", err)
	}

	pools, err := loadClusterPools(db, cc.Name)
	if err != nil {
		return nil, "", fmt.Errorf("load pools: %w", err)
	}

	conc := cc.Concurrency
	if conc <= 0 {
		conc = 1
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
	cluster.mpiEnabled = probeMPIOperator(cs, cc.Namespace)
	cluster.queueMode = cc.QueueMode
	if cluster.queueMode == "" {
		cluster.queueMode = "channel"
	}
	warning := ""
	if cluster.queueMode == "kueue" {
		if !probeKueue(cs) {
			warning = fmt.Sprintf("cluster %s: queue_mode=kueue but the Kueue Workload CRD (v1 or v1beta1) was not found; falling back to channel", cc.Name)
			cluster.queueMode = "channel"
		}
	}
	return cluster, warning, nil
}

// loadClusterPools reads the cluster's node-pool caps from the DB and returns
// a map keyed by pool name.
func loadClusterPools(db *gorm.DB, clusterName string) (map[string]*PoolState, error) {
	rows, err := database.GetClusterPools(db, clusterName)
	if err != nil {
		return nil, err
	}
	pools := make(map[string]*PoolState, len(rows))
	for _, p := range rows {
		pools[p.PoolName] = &PoolState{
			Name:         p.PoolName,
			NodeSelector: toStringMap(p.NodeSelector),
			CPU:          p.CPU,
			Memory:       p.Memory,
			IsPaused:     p.IsPaused,
		}
	}
	return pools, nil
}

// ReloadClusters re-reads clusters from the DB and rebuilds the clusters map.
// Queues for clusters that still exist are preserved (in-flight work survives);
// queues for removed clusters are dropped (their workers stop on channel close
// is not triggered here — workers range over the channel; callers should drain
// or accept that those goroutines block forever on a buffered channel).
// Returns a per-cluster list of warnings for kubeconfig parse failures.
func (s *Scheduler) ReloadClusters() ([]string, error) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	dbClusters, err := database.GetAllClusters(s.db)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	oldClusters := make(map[string]*ClusterState, len(s.clusters))
	for name, cluster := range s.clusters {
		oldClusters[name] = cluster
	}
	running := s.running
	s.mu.RUnlock()

	newMap := make(map[string]*ClusterState, len(dbClusters))
	newWorkers := make(map[string]*ClusterState)
	var warnings []string
	for _, cc := range dbClusters {
		cs, warning, err := buildClusterState(s.db, cc)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cluster %s: %v", cc.Name, err))
			if old, ok := oldClusters[cc.Name]; ok {
				newMap[cc.Name] = old
			}
			continue
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if old, ok := oldClusters[cc.Name]; ok {
			applyReloadedCluster(old, cs)
			newMap[cc.Name] = old
		} else {
			newMap[cc.Name] = cs
			newWorkers[cc.Name] = cs
		}
	}
	s.mu.Lock()
	s.clusters = newMap
	s.mu.Unlock()
	if running {
		for name, cluster := range newWorkers {
			go s.clusterWorker(name, cluster)
		}
	}
	return warnings, nil
}

// applyReloadedCluster updates a live ClusterState in place so its existing
// queue worker observes new clients, namespace, pools, and queue mode. Queue
// and semaphore identity are preserved to avoid dropping queued submissions.
func applyReloadedCluster(dst, src *ClusterState) {
	dst.Lock()
	defer dst.Unlock()
	dst.Namespace = src.Namespace
	dst.k8s = src.k8s
	dst.dyn = src.dyn
	dst.pools = src.pools
	dst.mpiEnabled = src.mpiEnabled
	dst.queueMode = src.queueMode
}

// Dispatcher exposes the dispatcher (used by admin handlers for interrupt).
func (s *Scheduler) Dispatcher() *Dispatcher { return s.dispatcher }

func (s *Scheduler) Run() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	clusters := make(map[string]*ClusterState, len(s.clusters))
	for name, cluster := range s.clusters {
		clusters[name] = cluster
	}
	s.mu.Unlock()
	for name, cluster := range clusters {
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
		// Pick a pool (FIFO over non-paused, non-zero pools; K8s checks actual resource fit).
		pool := s.findAvailablePool(cluster)
		if pool == nil {
			// No pool; release the slot and requeue after a delay.
			<-cluster.sem
			go func(j QueuedSubmission) {
				time.Sleep(time.Second)
				s.mu.RLock()
				c, ok := s.clusters[name]
				s.mu.RUnlock()
				if !ok {
					zap.S().Warnf("cluster %s removed during requeue; dropping submission %s", name, j.Submission.ID)
					return
				}
				c.queue <- j
			}(job)
			continue
		}
		sub.Node = pool.Name
		sub.Status = models.StatusRunning
		database.UpdateSubmission(s.db, &sub)
		go s.dispatcher.Dispatch(&sub, job.Problem, cluster, pool)
	}
}

func (s *Scheduler) findAvailablePool(cluster *ClusterState) *PoolState {
	cluster.Lock()
	defer cluster.Unlock()
	for _, p := range cluster.pools {
		if p.IsPaused || p.CPU == 0 || p.Memory == 0 {
			continue
		}
		// K8s scheduler checks actual resource fit; caps are a coarse admission
		// gate (pool must be non-zero).
		return p
	}
	return nil
}

func (s *Scheduler) Submit(submission *models.Submission, problem *Problem) {
	s.mu.RLock()
	cluster, ok := s.clusters[problem.Cluster]
	s.mu.RUnlock()
	if !ok {
		submission.Status = models.StatusFailed
		submission.Info = models.JSONMap{"error": "Invalid cluster specified in problem definition"}
		database.UpdateSubmission(s.db, submission)
		pubsubPublishError(submission.ID, "Invalid cluster specified in problem definition")
		return
	}
	if cluster.queueMode == "kueue" {
		// Bypass the in-process queue + semaphore: Kueue manages admission.
		// No pool is needed (Kueue handles scheduling); pass nil.
		submission.Node = ""
		submission.Status = models.StatusRunning
		database.UpdateSubmission(s.db, submission)
		go s.dispatcher.Dispatch(submission, problem, cluster, nil)
		return
	}
	cluster.queue <- QueuedSubmission{Submission: submission, Problem: problem}
}

// ReleaseSlot frees one concurrency slot (called by the dispatcher on completion).
func (s *Scheduler) ReleaseSlot(clusterName string) {
	s.mu.RLock()
	c, ok := s.clusters[clusterName]
	s.mu.RUnlock()
	if !ok {
		return
	}
	select {
	case <-c.sem:
	default:
	}
}

// GetClusterNames returns the names of all loaded clusters.
func (s *Scheduler) GetClusterNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.clusters))
	for name := range s.clusters {
		out = append(out, name)
	}
	return out
}

// GetClusterStates returns a snapshot of every cluster's pools + queue length.
func (s *Scheduler) GetClusterStates() map[string]ClusterStateSnapshot {
	out := make(map[string]ClusterStateSnapshot)
	s.mu.RLock()
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
			QueueMode:   c.queueMode,
		}
	}
	s.mu.RUnlock()
	return out
}

type ClusterStateSnapshot struct {
	Name        string
	Namespace   string
	Pools       map[string]PoolState
	MPIEnabled  bool
	QueueLength int
	Concurrency int
	QueueMode   string
}

// DynamicClientForCluster returns the cached dynamic.Interface + namespace
// for a cluster name. Returns an error if the cluster is not loaded
// (kubeconfig parse failure at startup, or the row was removed). Used by
// the DevPods integration to talk to the devpods CRDs in that cluster.
func (s *Scheduler) DynamicClientForCluster(name string) (dynamic.Interface, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clusters[name]
	if !ok {
		return nil, "", fmt.Errorf("cluster %q not loaded", name)
	}
	return c.dyn, c.Namespace, nil
}

// KubernetesClientForCluster returns the typed clientset used for core
// resources and subresources such as Pod logs.
func (s *Scheduler) KubernetesClientForCluster(name string) (kubernetes.Interface, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clusters[name]
	if !ok {
		return nil, fmt.Errorf("cluster %q not loaded", name)
	}
	return c.k8s, nil
}

func (s *Scheduler) GetQueueLengths() map[string]int {
	out := make(map[string]int)
	s.mu.RLock()
	for name, c := range s.clusters {
		out[name] = len(c.queue)
	}
	s.mu.RUnlock()
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
	s.mu.RLock()
	c, ok := s.clusters[clusterName]
	s.mu.RUnlock()
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
	s.mu.RLock()
	c, ok := s.clusters[clusterName]
	s.mu.RUnlock()
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
