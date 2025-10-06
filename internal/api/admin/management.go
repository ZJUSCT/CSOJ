package admin

import (
	"fmt"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *Handler) reload(c *gin.Context) {
	// Load new data into temporary variables
	zap.S().Info("starting reload process...")
	newContests, newProblems, err := judger.LoadAllContestsAndProblems(h.cfg.Contest)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to load new contests/problems: %w", err))
		return
	}
	zap.S().Infof("successfully loaded %d new contests and %d new problems from disk", len(newContests), len(newProblems))

	newProblemIDs := make(map[string]struct{}, len(newProblems))
	for id := range newProblems {
		newProblemIDs[id] = struct{}{}
	}

	// Find submissions whose problems have been deleted
	var allSubmissions []models.Submission
	// Fetch submissions with their containers to handle running ones
	if err := h.db.Preload("Containers").Find(&allSubmissions).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to get all submissions: %w", err))
		return
	}

	// Process and delete submissions for removed problems
	deletedCount := 0
	for _, sub := range allSubmissions {
		if _, exists := newProblemIDs[sub.ProblemID]; !exists {
			// This submission's problem was removed. Time to delete the submission.
			zap.S().Infof("problem %s for submission %s was removed, preparing to delete submission", sub.ProblemID, sub.ID)

			// For running submissions, we must clean up the container and resources first.
			if sub.Status == models.StatusRunning {
				h.appState.RLock()
				problem, ok := h.appState.Problems[sub.ProblemID]
				h.appState.RUnlock()

				if !ok {
					zap.S().Warnf("problem definition for running submission %s (problem %s) not found in old state during reload. Cannot stop container or release resources cleanly. DB record will be deleted anyway.", sub.ID, sub.ProblemID)
				} else {
					// This logic is adapted from the interrupt handler.
					var nodeDockerHost string
					for _, clusterCfg := range h.cfg.Cluster {
						if clusterCfg.Name == sub.Cluster {
							for _, nodeCfg := range clusterCfg.Nodes {
								if nodeCfg.Name == sub.Node {
									nodeDockerHost = nodeCfg.Docker
									break
								}
							}
							break
						}
					}

					if nodeDockerHost == "" {
						zap.S().Errorf("node config '%s'/'%s' not found for sub %s, cannot stop container", sub.Cluster, sub.Node, sub.ID)
					} else {
						docker, err := judger.NewDockerManager(nodeDockerHost)
						if err != nil {
							zap.S().Errorf("failed to connect to docker on node %s for sub %s cleanup: %v", sub.Node, sub.ID, err)
						} else {
							for _, container := range sub.Containers {
								if container.DockerID != "" {
									zap.S().Infof("forcefully cleaning up container %s for deleted submission %s", container.DockerID, sub.ID)
									docker.CleanupContainer(container.DockerID)
								}
							}
						}
					}
					h.scheduler.ReleaseResources(problem.Cluster, sub.Node, problem.CPU, problem.Memory)
				}
			}

			// Hard delete the submission from the database.
			if err := h.db.Delete(&models.Submission{}, sub.ID).Error; err != nil {
				zap.S().Errorf("failed to delete submission %s during reload: %v", sub.ID, err)
			} else {
				deletedCount++
				zap.S().Infof("deleted submission %s because its problem %s was removed", sub.ID, sub.ProblemID)
			}
		}
	}

	// Create new Problem-to-Contest map
	newProblemToContestMap := make(map[string]*judger.Contest)
	for _, contest := range newContests {
		for _, problemID := range contest.ProblemIDs {
			newProblemToContestMap[problemID] = contest
		}
	}

	// Atomically update the shared state
	h.appState.Lock()
	h.appState.Contests = newContests
	h.appState.Problems = newProblems
	h.appState.ProblemToContestMap = newProblemToContestMap
	h.appState.Unlock()
	zap.S().Info("app state reloaded successfully")

	util.Success(c, gin.H{
		"contests_loaded":     len(newContests),
		"problems_loaded":     len(newProblems),
		"submissions_deleted": deletedCount,
	}, "Reload successful")
}
