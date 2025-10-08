package judger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func (d *Dispatcher) Dispatch(sub *models.Submission, prob *Problem, node *NodeState, allocatedCores []int) {
	zap.S().Infof("dispatching submission %s to node %s", sub.ID, node.Name)

	// Ensure resources are released
	defer func() {
		d.scheduler.ReleaseResources(prob.Cluster, node.Name, allocatedCores, prob.Memory)
		zap.S().Infof("finished dispatching submission %s", sub.ID)
	}()

	docker, err := NewDockerManager(node.Docker)
	if err != nil {
		d.failSubmission(sub, fmt.Sprintf("failed to create docker client: %v", err))
		pubsub.GetBroker().CloseTopic(sub.ID)
		return
	}

	var successfulContainerIDs []string

	var lastStdout string
	var coreStrs []string
	for _, c := range allocatedCores {
		coreStrs = append(coreStrs, strconv.Itoa(c))
	}
	cpusetCpus := strings.Join(coreStrs, ",")

	for i, flow := range prob.Workflow {
		sub.CurrentStep = i
		database.UpdateSubmission(d.db, sub)

		containerID, stdout, stderr, err := d.runWorkflowStep(docker, sub, prob, flow, cpusetCpus, i == 0)

		if err != nil {
			d.failSubmission(sub, fmt.Sprintf("workflow step %d failed: %v\nStderr: %s", i, err, stderr))
			pubsub.GetBroker().CloseTopic(sub.ID)
			return
		}

		successfulContainerIDs = append(successfulContainerIDs, containerID)
		lastStdout = stdout
	}

	defer func() {
		for _, cid := range successfulContainerIDs {
			docker.CleanupContainer(cid)
		}
	}()

	var result JudgeResult
	if err := json.Unmarshal([]byte(lastStdout), &result); err != nil {
		d.failSubmission(sub, fmt.Sprintf("failed to parse judge result: %v. Raw output: %s", err, lastStdout))
		pubsub.GetBroker().CloseTopic(sub.ID)
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
	pubsub.GetBroker().CloseTopic(sub.ID)

	contestID := d.findContestIDForProblem(prob.ID)
	if contestID == "" {
		zap.S().Warnf("cannot find contest for problem %s, skipping score update", prob.ID)
		return
	}
	if err := database.UpdateScoresForNewSubmission(d.db, sub, contestID, sub.Score); err != nil {
		zap.S().Errorf("failed to update scores for submission %s: %v", sub.ID, err)
	}
}

func (d *Dispatcher) runWorkflowStep(docker *DockerManager, sub *models.Submission, prob *Problem, flow WorkflowStep, cpusetCpus string, isFirstStep bool) (containerID, stdout, stderr string, err error) {
	zap.S().Debugf("Creating timeout context for step. Raw timeout value from config: %d seconds", flow.Timeout)
	stepCtx, cancel := context.WithTimeout(context.Background(), time.Duration(flow.Timeout)*time.Second)
	defer cancel()

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
	defer pubsub.GetBroker().CloseTopic(cont.ID)

	type result struct {
		ContainerID string
		Stdout      string
		Stderr      string
		Err         error
	}
	doneChan := make(chan result, 1)
	cidChan := make(chan string, 1)

	go func() {
		var execStdout, execStderr string
		var cid string

		defer func() {
			if r := recover(); r != nil {
				zap.S().Errorf("Recovered from panic in dispatcher goroutine: %v", r)
				doneChan <- result{ContainerID: cid, Err: fmt.Errorf("panic recovered: %v", r)}
			}
		}()

		remoteWorkDir := filepath.Join("/tmp", "submission", sub.ID)
		var err error
		cid, err = docker.CreateContainer(flow.Image, remoteWorkDir, prob.CPU, cpusetCpus, prob.Memory, flow.Root, flow.Mounts, flow.Network)
		if err != nil {
			doneChan <- result{Err: fmt.Errorf("failed to create container: %w", err)}
			return
		}

		cidChan <- cid
		cont.DockerID = cid
		database.UpdateContainer(d.db, cont)

		if err := docker.StartContainer(cid); err != nil {
			doneChan <- result{ContainerID: cid, Err: fmt.Errorf("failed to start container: %w", err)}
			return
		}

		if isFirstStep {
			localWorkDir := filepath.Join(d.cfg.Storage.SubmissionContent, sub.ID)
			zap.S().Infof("copying files from %s to container %s:/mnt/work/", localWorkDir, cid)
			if err := docker.CopyToContainer(cid, localWorkDir, "/mnt/work/"); err != nil {
				doneChan <- result{ContainerID: cid, Err: fmt.Errorf("failed to copy files to container: %w", err)}
				return
			}
		}

		var combinedLog bytes.Buffer
		for j, stepCmd := range flow.Steps {
			outputCallback := func(streamType string, data []byte) {
				msg := pubsub.FormatMessage(streamType, string(data))
				pubsub.GetBroker().Publish(cont.ID, msg)
			}

			execResult, err := docker.ExecInContainer(stepCtx, cid, stepCmd, outputCallback)

			combinedLog.WriteString(fmt.Sprintf("\n--- Executing Command %d ---\n", j+1))
			combinedLog.WriteString("STDOUT:\n")
			combinedLog.WriteString(execResult.Stdout)
			combinedLog.WriteString("\nSTDERR:\n")
			combinedLog.WriteString(execResult.Stderr)
			combinedLog.WriteString(fmt.Sprintf("\n--- Exit Code: %d ---\n", execResult.ExitCode))

			if err != nil || execResult.ExitCode != 0 {
				d.failContainer(cont, execResult.ExitCode, combinedLog.String())
				errMsg := fmt.Errorf("exec failed with exit code %d: %w", execResult.ExitCode, err)
				doneChan <- result{ContainerID: cid, Stdout: execResult.Stdout, Stderr: execResult.Stderr, Err: errMsg}
				return
			}
			execStdout = execResult.Stdout
			execStderr = execResult.Stderr
		}
		os.WriteFile(logFilePath, combinedLog.Bytes(), 0644)
		doneChan <- result{ContainerID: cid, Stdout: execStdout, Stderr: execStderr, Err: nil}
	}()

	var finalRes result
	var cidForCleanup string

	zap.S().Debugf("Entering select block for submission %s, waiting for completion or timeout...", sub.ID)
	select {
	case cidForCleanup = <-cidChan:
		select {
		case <-stepCtx.Done():
			zap.S().Warnf("TIMEOUT branch selected for submission %s. Cleaning up container %s.", sub.ID, cidForCleanup)
			docker.CleanupContainer(cidForCleanup)
			d.failContainer(cont, -1, "overall step timeout exceeded")
			return cidForCleanup, "", "Timeout exceeded", stepCtx.Err()

		case finalRes = <-doneChan:
			zap.S().Debugf("DONE_CHAN branch selected for submission %s. Error from goroutine: %v", sub.ID, finalRes.Err)
		}
	case <-stepCtx.Done():
		zap.S().Warnf("TIMEOUT branch selected for submission %s. Container was not even created.", sub.ID)
		d.failContainer(cont, -1, "overall step timeout exceeded before container creation")
		return "", "", "Timeout exceeded", stepCtx.Err()

	case finalRes = <-doneChan:
		zap.S().Debugf("DONE_CHAN (early) branch selected for submission %s. Error from goroutine: %v", sub.ID, finalRes.Err)
	}

	// Process the final result
	if finalRes.Err != nil {
		if finalRes.ContainerID != "" {
			docker.CleanupContainer(finalRes.ContainerID)
		}
		return finalRes.ContainerID, finalRes.Stdout, finalRes.Stderr, finalRes.Err
	}

	cont.Status = models.StatusSuccess
	cont.FinishedAt = time.Now()
	database.UpdateContainer(d.db, cont)
	return finalRes.ContainerID, finalRes.Stdout, finalRes.Stderr, nil
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
