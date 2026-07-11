package judger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger/podspec"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

type Dispatcher struct {
	cfg       *config.Config
	db        *gorm.DB
	scheduler *Scheduler
	appState  *AppState
}

func NewDispatcher(cfg *config.Config, db *gorm.DB, scheduler *Scheduler, appState *AppState) *Dispatcher {
	return &Dispatcher{cfg: cfg, db: db, scheduler: scheduler, appState: appState}
}

// JudgeResult is the score contract written by the final workflow step to
// /mnt/work/.csoj/result.json (mirrored on the API server's PVC mount).
type JudgeResult struct {
	Score       int                    `json:"score"`
	Performance float64                `json:"performance"`
	Info        map[string]interface{} `json:"info"`
}

// Dispatch runs a submission's workflow: one Pod (or MPIJob) per step,
// tails logs into pubsub, reads /mnt/work/.csoj/result.json for the score.
func (d *Dispatcher) Dispatch(sub *models.Submission, prob *Problem, cluster *ClusterState, pool *PoolState) {
	km := NewKubeManager(cluster.k8s, cluster.dyn, cluster.Namespace)
	defer func() {
		// Clean up all pods/MPIJobs/Jobs for this submission, release the slot (channel
		// mode only — Kueue mode bypasses the in-process semaphore), close the topic.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = km.DeleteSubmissionPods(ctx, sub.ID)
		_ = km.DeleteSubmissionMPIJobs(ctx, sub.ID)
		if cluster.queueMode == "kueue" {
			_ = km.DeleteSubmissionJobs(ctx, sub.ID)
		}
		cancel()
		if cluster.queueMode != "kueue" {
			d.scheduler.ReleaseSlot(cluster.Name)
		}
		pubsub.GetBroker().CloseTopic(sub.ID)
	}()

	env := []corev1.EnvVar{
		{Name: "CSOJ_SUBMIT_DIR", Value: "/mnt/work"},
		{Name: "CSOJ_USERNAME", Value: usernameFor(d.db, sub.UserID)},
	}

	for i, flow := range prob.Workflow {
		sub.CurrentStep = i
		database.UpdateSubmission(d.db, sub)

		var err error
		if flow.MPI != nil && flow.MPI.Enabled {
			err = d.runMPIStep(km, sub, prob, flow, cluster, pool, env, i)
		} else {
			err = d.runPodStep(km, sub, prob, flow, cluster, pool, env, i)
		}
		if err != nil {
			d.failSubmission(sub, fmt.Sprintf("workflow step %d failed: %v", i+1, err))
			return
		}
	}

	// Read the score from the result file on the shared PVC (the API server mounts it too).
	resultPath := filepath.Join(d.cfg.Storage.SubmissionContent, sub.ID, ".csoj", "result.json")
	data, err := os.ReadFile(resultPath)
	if err != nil {
		d.failSubmission(sub, fmt.Sprintf("result.json not found: %v", err))
		return
	}
	var jr JudgeResult
	if err := json.Unmarshal(data, &jr); err != nil {
		d.failSubmission(sub, fmt.Sprintf("failed to parse result.json: %v", err))
		return
	}

	contestID := d.findContestIDForProblem(prob.ID)
	sub.Info = jr.Info
	if prob.Score.Mode == "performance" && contestID != "" {
		sub.Performance = jr.Performance
		if err := database.UpdateScoresForPerformanceSubmission(d.db, sub, contestID, prob.Score.MaxPerformanceScore); err != nil {
			zap.S().Errorf("failed to update performance scores for %s: %v", sub.ID, err)
		}
		var updated models.Submission
		if errDb := d.db.Select("score").Where("id = ?", sub.ID).First(&updated).Error; errDb == nil {
			sub.Score = updated.Score
		}
	} else {
		sub.Score = jr.Score
		if contestID != "" {
			if err := database.UpdateScoresForNewSubmission(d.db, sub, contestID, sub.Score); err != nil {
				zap.S().Errorf("failed to update scores for %s: %v", sub.ID, err)
			}
		}
	}

	sub.Status = models.StatusSuccess
	if err := database.UpdateSubmission(d.db, sub); err != nil {
		zap.S().Errorf("failed to update successful submission %s: %v", sub.ID, err)
		return
	}
	zap.S().Infof("submission %s finished with score %d", sub.ID, sub.Score)
}

// runPodStep builds + creates a Pod, tails its logs into pubsub, waits for completion.
// In Kueue mode it delegates to runJobStep (creates a batch/v1 Job instead of a
// bare Pod, so Kueue can admit it through a LocalQueue).
func (d *Dispatcher) runPodStep(km *KubeManager, sub *models.Submission, prob *Problem, flow WorkflowStep, cluster *ClusterState, pool *PoolState, env []corev1.EnvVar, step int) error {
	if cluster.queueMode == "kueue" {
		return d.runJobStep(km, sub, prob, flow, cluster, env, step)
	}
	podName := fmt.Sprintf("%s-%d", sub.ID, step)
	script := podspec.GenerateEntrypointScript(flow.Steps)
	pod := podspec.BuildPodSpec(podspec.PodSpecInput{
		Name:              podName,
		Namespace:         km.ns,
		Image:             flow.Image,
		Script:            script,
		CPURequest:        stepCPURequest(flow),
		CPULimit:          stepCPULimit(flow),
		MemoryRequest:     stepMemoryRequest(flow),
		MemoryLimit:       stepMemoryLimit(flow),
		GPUCount:          stepGPUCount(flow),
		GPUResource:       stepGPUResource(flow),
		NodeSel:           pool.NodeSelector,
		NodeAffinity:      stepNodeAffinity(flow),
		Tolerations:       stepTolerations(flow),
		PriorityClassName: stepPriorityClass(flow),
		RuntimeClassName:  stepRuntimeClass(flow),
		Env:               env,
		SubID:             sub.ID,
		Step:              step,
		AsRoot:            flow.Root,
		Network:           flow.Network,
		TimeoutSec:        int64(flow.Timeout),
	})

	cont := d.newContainerRecord(sub, flow.Image, step)
	cont.PodName = podName
	database.CreateContainer(d.db, cont)
	defer pubsub.GetBroker().CloseTopic(cont.ID)

	ctx, cancel := context.WithTimeout(context.Background(), durationWithDefault(flow.Timeout, 30*60))
	defer cancel()

	if err := km.CreatePod(ctx, pod); err != nil {
		d.failContainer(cont, -1, fmt.Sprintf("failed to create pod: %v", err))
		return err
	}

	// Stream logs in the background.
	logCtx, logCancel := context.WithCancel(ctx)
	go func() {
		_ = km.StreamPodLogs(logCtx, podName, func(chunk string) {
			pubsub.GetBroker().Publish(cont.ID, pubsub.FormatMessage("stdout", chunk))
		})
	}()

	phase, err := km.WaitForPod(ctx, podName)
	logCancel()
	if err != nil && phase == "" {
		d.failContainer(cont, -1, fmt.Sprintf("timeout/error: %v", err))
		return err
	}
	if phase == corev1.PodFailed {
		d.failContainer(cont, -1, "pod failed")
		return fmt.Errorf("pod %s failed", podName)
	}
	cont.Status = models.StatusSuccess
	cont.FinishedAt = time.Now()
	database.UpdateContainer(d.db, cont)
	return nil
}

// runJobStep is the Kueue-mode equivalent of runPodStep: it builds a batch/v1
// Job (wrapping the same Pod spec) and lets Kueue admit it through a LocalQueue.
// Kueue handles scheduling, so no node-pool is consulted.
func (d *Dispatcher) runJobStep(km *KubeManager, sub *models.Submission, prob *Problem, flow WorkflowStep, cluster *ClusterState, env []corev1.EnvVar, step int) error {
	jobName := fmt.Sprintf("%s-%d", sub.ID, step)
	script := podspec.GenerateEntrypointScript(flow.Steps)

	job := podspec.BuildJobSpec(podspec.JobSpecInput{
		PodSpecInput: podspec.PodSpecInput{
			Name:              jobName,
			Namespace:         km.ns,
			Image:             flow.Image,
			Script:            script,
			CPURequest:        stepCPURequest(flow),
			CPULimit:          stepCPULimit(flow),
			MemoryRequest:     stepMemoryRequest(flow),
			MemoryLimit:       stepMemoryLimit(flow),
			GPUCount:          stepGPUCount(flow),
			GPUResource:       stepGPUResource(flow),
			NodeSel:           nil, // Kueue handles scheduling
			NodeAffinity:      stepNodeAffinity(flow),
			Tolerations:       stepTolerations(flow),
			PriorityClassName: stepPriorityClass(flow),
			RuntimeClassName:  stepRuntimeClass(flow),
			Env:               env,
			SubID:             sub.ID,
			Step:              step,
			AsRoot:            flow.Root,
			Network:           flow.Network,
			TimeoutSec:        int64(flow.Timeout),
		},
		QueueName: cluster.Name + "-queue",
	})

	cont := d.newContainerRecord(sub, flow.Image, step)
	cont.PodName = jobName
	database.CreateContainer(d.db, cont)
	defer pubsub.GetBroker().CloseTopic(cont.ID)

	ctx, cancel := context.WithTimeout(context.Background(), durationWithDefault(flow.Timeout, 30*60))
	defer cancel()

	if err := km.CreateJob(ctx, job); err != nil {
		d.failContainer(cont, -1, fmt.Sprintf("failed to create job: %v", err))
		return err
	}

	logCtx, logCancel := context.WithCancel(ctx)
	go func() {
		_ = km.StreamJobLogs(logCtx, jobName, func(chunk string) {
			pubsub.GetBroker().Publish(cont.ID, pubsub.FormatMessage("stdout", chunk))
		})
	}()

	cond, err := km.WaitForJob(ctx, jobName)
	logCancel()
	if err != nil && cond == "" {
		d.failContainer(cont, -1, fmt.Sprintf("timeout/error: %v", err))
		return err
	}
	if cond == batchv1.JobFailed {
		d.failContainer(cont, -1, "job failed")
		return fmt.Errorf("job %s failed", jobName)
	}
	cont.Status = models.StatusSuccess
	cont.FinishedAt = time.Now()
	database.UpdateContainer(d.db, cont)
	return nil
}

// runMPIStep builds + creates an MPIJob, tails the launcher logs, waits for completion.
func (d *Dispatcher) runMPIStep(km *KubeManager, sub *models.Submission, prob *Problem, flow WorkflowStep, cluster *ClusterState, pool *PoolState, env []corev1.EnvVar, step int) error {
	if !cluster.mpiEnabled {
		return fmt.Errorf("cluster does not support MPI")
	}
	name := fmt.Sprintf("%s-%d", sub.ID, step)
	totalRanks := flow.MPI.WorkerReplicas * flow.MPI.SlotsPerWorker
	launcherScript := fmt.Sprintf("mpirun -np %d %s", totalRanks, joinArgs(flow.MPI.LauncherCmd))
	obj := podspec.BuildMPIJobSpec(podspec.MPIJobSpecInput{
		Name:              name,
		Namespace:         km.ns,
		Image:             flow.Image,
		LauncherScript:    launcherScript,
		WorkerReplicas:    flow.MPI.WorkerReplicas,
		CPURequest:        stepCPURequest(flow),
		CPULimit:          stepCPULimit(flow),
		MemoryRequest:     stepMemoryRequest(flow),
		MemoryLimit:       stepMemoryLimit(flow),
		GPUCount:          stepGPUCount(flow),
		GPUResource:       stepGPUResource(flow),
		NodeSel:           pool.NodeSelector,
		NodeAffinity:      stepNodeAffinity(flow),
		Tolerations:       stepTolerations(flow),
		PriorityClassName: stepPriorityClass(flow),
		RuntimeClassName:  stepRuntimeClass(flow),
		SubID:             sub.ID,
		Step:              step,
	})

	cont := d.newContainerRecord(sub, flow.Image, step)
	cont.PodName = name + "-launcher"
	database.CreateContainer(d.db, cont)
	defer pubsub.GetBroker().CloseTopic(cont.ID)

	ctx, cancel := context.WithTimeout(context.Background(), durationWithDefault(flow.Timeout, 30*60))
	defer cancel()

	if err := km.CreateMPIJob(ctx, obj); err != nil {
		d.failContainer(cont, -1, fmt.Sprintf("failed to create MPIJob: %v", err))
		return err
	}

	// Tail the launcher pod's logs once it exists. The mpi-operator names the
	// launcher pod <mpijob-name>-launcher; we just attempt to stream and let
	// the follow-handler retry internally (best-effort).
	go func() {
		launcherName := name + "-launcher"
		_ = km.StreamPodLogs(ctx, launcherName, func(chunk string) {
			pubsub.GetBroker().Publish(cont.ID, pubsub.FormatMessage("stdout", chunk))
		})
	}()

	phase, err := km.WaitForMPIJob(ctx, name)
	if err != nil && phase == "" {
		d.failContainer(cont, -1, fmt.Sprintf("timeout/error: %v", err))
		return err
	}
	if phase == corev1.PodFailed {
		d.failContainer(cont, -1, "MPIJob failed")
		return fmt.Errorf("MPIJob %s failed", name)
	}
	cont.Status = models.StatusSuccess
	cont.FinishedAt = time.Now()
	database.UpdateContainer(d.db, cont)
	return nil
}

// helpers

func (d *Dispatcher) newContainerRecord(sub *models.Submission, image string, step int) *models.Container {
	return &models.Container{
		ID:           uuid.NewString(),
		SubmissionID: sub.ID,
		UserID:       sub.UserID,
		Image:        image,
		Status:       models.StatusRunning,
		StartedAt:    time.Now(),
	}
}

func (d *Dispatcher) failSubmission(sub *models.Submission, reason string) {
	pubsub.GetBroker().Publish(sub.ID, pubsub.FormatMessage("error", reason))
	sub.Status = models.StatusFailed
	sub.Info = models.JSONMap{"error": reason}
	database.UpdateSubmission(d.db, sub)
	zap.S().Errorf("submission %s failed: %s", sub.ID, reason)
}

func (d *Dispatcher) failContainer(cont *models.Container, exitCode int, reason string) {
	cont.Status = models.StatusFailed
	cont.ExitCode = exitCode
	cont.FinishedAt = time.Now()
	pubsub.GetBroker().Publish(cont.ID, pubsub.FormatMessage("error", reason))
	database.UpdateContainer(d.db, cont)
}

func (d *Dispatcher) findContestIDForProblem(problemID string) string {
	d.appState.RLock()
	defer d.appState.RUnlock()
	if c, ok := d.appState.ProblemToContestMap[problemID]; ok {
		return c.ID
	}
	return ""
}

func usernameFor(db *gorm.DB, userID string) string {
	u, err := database.GetUserByID(db, userID)
	if err != nil || u == nil {
		return ""
	}
	return u.Username
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += strconv.Quote(a)
	}
	return out
}

// Per-step resource/scheduling extractors. Each returns the zero value when
// flow.Resources / flow.Scheduling is nil, letting podspec apply its defaults.

func stepCPURequest(flow WorkflowStep) string {
	if flow.Resources == nil {
		return ""
	}
	return flow.Resources.CPURequest
}

func stepCPULimit(flow WorkflowStep) string {
	if flow.Resources == nil {
		return ""
	}
	return flow.Resources.CPULimit
}

func stepMemoryRequest(flow WorkflowStep) string {
	if flow.Resources == nil {
		return ""
	}
	return flow.Resources.MemoryRequest
}

func stepMemoryLimit(flow WorkflowStep) string {
	if flow.Resources == nil {
		return ""
	}
	return flow.Resources.MemoryLimit
}

func stepGPUCount(flow WorkflowStep) int {
	if flow.Resources == nil {
		return 0
	}
	return flow.Resources.GPUCount
}

func stepGPUResource(flow WorkflowStep) string {
	if flow.Resources == nil {
		return ""
	}
	return flow.Resources.GPUResource
}

func stepNodeAffinity(flow WorkflowStep) []podspec.NodeAffinityTerm {
	if flow.Scheduling == nil || len(flow.Scheduling.NodeAffinity) == 0 {
		return nil
	}
	out := make([]podspec.NodeAffinityTerm, 0, len(flow.Scheduling.NodeAffinity))
	for _, a := range flow.Scheduling.NodeAffinity {
		out = append(out, podspec.NodeAffinityTerm{Key: a.Key, Operator: a.Operator, Values: a.Values})
	}
	return out
}

func stepTolerations(flow WorkflowStep) []podspec.Toleration {
	if flow.Scheduling == nil || len(flow.Scheduling.Tolerations) == 0 {
		return nil
	}
	out := make([]podspec.Toleration, 0, len(flow.Scheduling.Tolerations))
	for _, t := range flow.Scheduling.Tolerations {
		out = append(out, podspec.Toleration{Key: t.Key, Operator: t.Operator, Value: t.Value, Effect: t.Effect})
	}
	return out
}

func stepPriorityClass(flow WorkflowStep) string {
	if flow.Scheduling == nil {
		return ""
	}
	return flow.Scheduling.PriorityClassName
}

func stepRuntimeClass(flow WorkflowStep) string {
	if flow.Scheduling == nil {
		return ""
	}
	return flow.Scheduling.RuntimeClassName
}

func durationWithDefault(timeoutSec, defaultSec int) time.Duration {
	if timeoutSec <= 0 {
		return time.Duration(defaultSec) * time.Second
	}
	return time.Duration(timeoutSec) * time.Second
}
