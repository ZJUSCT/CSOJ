package user

import (
	"bufio"
	"net/http"
	"os"
	"sort"

	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *Handler) handleUserContainerWs(c *gin.Context) {
	submissionID := c.Param("subID")
	containerID := c.Param("conID")
	tokenString := c.Query("token")

	if tokenString == "" {
		c.String(http.StatusUnauthorized, "token query parameter is required")
		return
	}

	claims, err := auth.ValidateJWT(tokenString, h.cfg.Auth.JWT.Secret)
	if err != nil {
		c.String(http.StatusUnauthorized, "invalid token")
		return
	}
	userID := claims.Subject

	// --- Authorization Checks ---
	sub, err := database.GetSubmission(h.db, submissionID)
	if err != nil {
		c.String(http.StatusNotFound, "submission not found")
		return
	}
	if sub.UserID != userID {
		c.String(http.StatusForbidden, "you can only view your own submissions")
		return
	}

	var targetContainer *models.Container
	var containerIndex = -1
	sort.Slice(sub.Containers, func(i, j int) bool {
		return sub.Containers[i].CreatedAt.Before(sub.Containers[j].CreatedAt)
	})
	for i, container := range sub.Containers {
		if container.ID == containerID {
			targetContainer = &sub.Containers[i]
			containerIndex = i
			break
		}
	}

	if targetContainer == nil {
		c.String(http.StatusNotFound, "container not found in this submission")
		return
	}

	h.appState.RLock()
	problem, ok := h.appState.Problems[sub.ProblemID]
	h.appState.RUnlock()
	if !ok {
		c.String(http.StatusInternalServerError, "problem definition not found")
		return
	}

	if containerIndex >= len(problem.Workflow) || !problem.Workflow[containerIndex].Show {
		c.String(http.StatusForbidden, "you are not allowed to view the log for this step")
		return
	}
	// --- End Authorization ---

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.S().Errorf("failed to upgrade websocket: %v", err)
		return
	}
	defer conn.Close()

	if targetContainer.Status == models.StatusRunning {
		// Real-time streaming
		msgChan, unsubscribe := pubsub.GetBroker().Subscribe(containerID)
		defer unsubscribe()

		clientClosed := make(chan struct{})
		go func() {
			defer close(clientClosed)
			for msg := range msgChan {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					zap.S().Warnf("error writing to websocket: %v", err)
					return
				}
			}
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					zap.S().Infof("websocket unexpected close error: %v", err)
				}
				break
			}
		}
		<-clientClosed

	} else { // StatusSuccess or StatusFailed: Stream the stored log file
		if targetContainer.LogFilePath == "" {
			msg := pubsub.FormatMessage("error", "Log file path not recorded.")
			conn.WriteMessage(websocket.TextMessage, msg)
			return
		}

		file, err := os.Open(targetContainer.LogFilePath)
		if err != nil {
			msg := pubsub.FormatMessage("error", "Log file not found on disk.")
			conn.WriteMessage(websocket.TextMessage, msg)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			msg := scanner.Bytes() // The file content is already NDJSON
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return // Client disconnected
			}
		}
		if err := scanner.Err(); err != nil {
			zap.S().Errorf("error reading log file for container %s: %v", containerID, err)
		}
		msg := pubsub.FormatMessage("info", "Log stream finished.")
		conn.WriteMessage(websocket.TextMessage, msg)
	}
	zap.S().Infof("websocket connection closed for container %s", containerID)
}
