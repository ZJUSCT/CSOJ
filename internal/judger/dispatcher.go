package judger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Dispatcher struct {
	cfg       *config.Config
	db        *gorm.DB
	scheduler *Scheduler
	appState  *AppState
}

type JudgeResult struct {
	Score int                    `json:"score"`
	Info  map[string]interface{} `json:"info"`
}

func NewDispatcher(cfg *config.Config, db *gorm.DB, scheduler *Scheduler, appState *AppState) *Dispatcher {
	return &Dispatcher{
		cfg:       cfg,
		db:        db,
		scheduler: scheduler,
		appState:  appState,
	}
}

func (d *Dispatcher) Dispatch(sub *models.Submission, prob *Problem, node *NodeState) {
	zap.S().Infof("dispatching submission %s to node %s", sub.ID, node.Name)

	// Ensure resources and pubsub topic are cleaned up
	defer func() {
		d.scheduler.ReleaseResources(prob.Cluster, node.Name, prob.CPU, prob.Memory)
		pubsub.GetBroker().CloseTopic(sub.ID)
		zap.S().Infof("finished dispatching submission %s", sub.ID)
	}()

	docker, err := NewDockerManager(node.Docker)
	if err != nil {
		d.failSubmission(sub, fmt.Sprintf("failed to create docker client: %v", err))
		return
	}

	var containerIDs []string
	defer func() {
		for _, cid := range containerIDs {
			docker.CleanupContainer(cid)
		}
	}()

	var lastStdout string
	for i, flow := range prob.Workflow {
		sub.CurrentStep = i
		database.UpdateSubmission(d.db, sub)

		containerID, stdout, stderr, err := d.runWorkflowStep(docker, sub, prob, flow, i == 0)
		if containerID != "" {
			containerIDs = append(containerIDs, containerID)
		}
		if err != nil {
			d.failSubmission(sub, fmt.Sprintf("workflow step %d failed: %v\nStderr: %s", i, err, stderr))
			return
		}
		lastStdout = stdout
	}

	var result JudgeResult
	if err := json.Unmarshal([]byte(lastStdout), &result); err != nil {
		d.failSubmission(sub, fmt.Sprintf("failed to parse judge result: %v. Raw output: %s", err, lastStdout))
		return
	}

	sub.Status = models.StatusSuccess
	sub.Score = result.Score
	sub.Info = result.Info
	if err := database.UpdateSubmission(d.db, sub); err != nil {
		zap.S().Errorf("failed to update successful submission %s: %v", sub.ID, err)
		return
	}

	zap.S().Infof("submission %s finished successfully with score %d", sub.ID, sub.Score)

	contestID := d.findContestIDForProblem(prob.ID)
	if contestID == "" {
		zap.S().Warnf("cannot find contest for problem %s, skipping score update", prob.ID)
		return
	}
	if err := database.UpdateScoresForNewSubmission(d.db, sub, contestID, sub.Score); err != nil {
		zap.S().Errorf("failed to update scores for submission %s: %v", sub.ID, err)
	}
}

func (d *Dispatcher) runWorkflowStep(docker *DockerManager, sub *models.Submission, prob *Problem, flow WorkflowStep, isFirstStep bool) (containerID, stdout, stderr string, err error) {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(d.cfg.Storage.SubmissionLog, 0755); err != nil {
		return "", "", "", fmt.Errorf("failed to create log directory: %w", err)
	}

	logFileName := fmt.Sprintf("%s_%s.log", sub.ID, uuid.New().String())
	logFilePath := filepath.Join(d.cfg.Storage.SubmissionLog, logFileName)

	cont := &models.Container{
		ID:           uuid.New().String(),
		SubmissionID: sub.ID,
		UserID:       sub.UserID,
		Image:        flow.Image,
		Status:       models.StatusRunning,
		StartedAt:    time.Now(),
		LogFilePath:  logFilePath,
	}
	database.CreateContainer(d.db, cont)

	remoteWorkDir := filepath.Join("/tmp", "submission", sub.ID)

	containerID, err = docker.CreateContainer(flow.Image, remoteWorkDir, prob.CPU, prob.Memory, flow.Root)
	if err != nil {
		d.failContainer(cont, -1, "failed to create container")
		return "", "", "", fmt.Errorf("failed to create container: %v", err)
	}
	cont.DockerID = containerID
	database.UpdateContainer(d.db, cont)

	if err := docker.StartContainer(containerID); err != nil {
		d.failContainer(cont, -1, "failed to start container")
		return containerID, "", "", fmt.Errorf("failed to start container: %v", err)
	}

	if isFirstStep {
		localWorkDir := filepath.Join(d.cfg.Storage.SubmissionContent, sub.ID)
		zap.S().Infof("copying files from %s to container %s:/mnt/work/", localWorkDir, containerID)
		if err := docker.CopyToContainer(containerID, localWorkDir, "/mnt/work/"); err != nil {
			d.failContainer(cont, -1, "failed to copy files")
			return containerID, "", "", fmt.Errorf("failed to copy files to container: %v", err)
		}
	}

	var combinedLog bytes.Buffer
	for _, stepCmd := range flow.Steps {
		// Callback for real-time streaming
		outputCallback := func(streamType string, data []byte) {
			msg := pubsub.FormatMessage(streamType, string(data))
			pubsub.GetBroker().Publish(sub.ID, msg)
		}

		execResult, err := docker.ExecInContainer(containerID, stepCmd, time.Duration(flow.Timeout)*time.Second, outputCallback)

		// Append command and output to the combined log buffer
		combinedLog.WriteString(fmt.Sprintf("\n--- Executing: %v ---\n", stepCmd))
		combinedLog.WriteString("STDOUT:\n")
		combinedLog.WriteString(execResult.Stdout)
		combinedLog.WriteString("\nSTDERR:\n")
		combinedLog.WriteString(execResult.Stderr)
		combinedLog.WriteString(fmt.Sprintf("\n--- Exit Code: %d ---\n", execResult.ExitCode))

		if err != nil || execResult.ExitCode != 0 {
			d.failContainer(cont, execResult.ExitCode, combinedLog.String())
			errMsg := fmt.Errorf("exec failed with exit code %d: %w", execResult.ExitCode, err)
			return containerID, execResult.Stdout, execResult.Stderr, errMsg
		}
		stdout = execResult.Stdout
		stderr = execResult.Stderr
	}

	cont.Status = models.StatusSuccess
	cont.FinishedAt = time.Now()
	database.UpdateContainer(d.db, cont)
	// Write the full log to file upon success
	os.WriteFile(logFilePath, combinedLog.Bytes(), 0644)

	return containerID, stdout, stderr, nil
}

func (d *Dispatcher) findContestIDForProblem(problemID string) string {
	d.appState.RLock()
	defer d.appState.RUnlock()
	if contest, ok := d.appState.ProblemToContestMap[problemID]; ok {
		return contest.ID
	}
	zap.S().Warnf("could not find parent contest for problem ID %s", problemID)
	return ""
}

func (d *Dispatcher) failSubmission(sub *models.Submission, reason string) {
	zap.S().Errorf("submission %s failed: %s", sub.ID, reason)
	msg := pubsub.FormatMessage("error", reason)
	pubsub.GetBroker().Publish(sub.ID, msg)
	sub.Status = models.StatusFailed
	sub.Info = map[string]interface{}{"error": reason}
	if err := database.UpdateSubmission(d.db, sub); err != nil {
		zap.S().Errorf("failed to update failed submission status for %s: %v", sub.ID, err)
	}
}

func (d *Dispatcher) failContainer(cont *models.Container, exitCode int, logContent string) {
	cont.Status = models.StatusFailed
	cont.ExitCode = exitCode
	cont.FinishedAt = time.Now()
	// On failure, write the log content to the file
	if err := os.WriteFile(cont.LogFilePath, []byte(logContent), 0644); err != nil {
		zap.S().Errorf("failed to write error log for container %s: %v", cont.ID, err)
	}
	database.UpdateContainer(d.db, cont)
}
