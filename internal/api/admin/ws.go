package admin

import (
	"bufio"
	"net/http"
	"os"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var adminUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *Handler) handleAdminContainerWs(c *gin.Context) {
	submissionID := c.Param("id")
	containerID := c.Param("conID")

	con, err := database.GetContainer(h.db, containerID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.String(http.StatusNotFound, "container not found")
			return
		}
		c.String(http.StatusInternalServerError, "database error")
		return
	}

	if con.SubmissionID != submissionID {
		c.String(http.StatusForbidden, "container does not belong to this submission")
		return
	}

	conn, err := adminUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.S().Errorf("failed to upgrade admin websocket: %v", err)
		return
	}
	defer conn.Close()

	if con.Status == models.StatusRunning {
		// Real-time streaming for a running container
		msgChan, unsubscribe := pubsub.GetBroker().Subscribe(containerID)
		defer unsubscribe()

		// Goroutine to pump messages from pubsub to websocket
		clientClosed := make(chan struct{})
		go func() {
			defer close(clientClosed)
			for msg := range msgChan {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					zap.S().Warnf("error writing to admin websocket: %v", err)
					return
				}
			}
		}()

		// Read loop to detect client close
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					zap.S().Infof("admin websocket unexpected close error: %v", err)
				}
				break
			}
		}
		<-clientClosed // Wait for writer goroutine to finish before returning

	} else { // StatusSuccess or StatusFailed: Stream the stored log file
		if con.LogFilePath == "" {
			msg := pubsub.FormatMessage("error", "Log file path not recorded for this container.")
			conn.WriteMessage(websocket.TextMessage, msg)
			return
		}

		file, err := os.Open(con.LogFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				msg := pubsub.FormatMessage("error", "Log file not found on disk.")
				conn.WriteMessage(websocket.TextMessage, msg)
			} else {
				msg := pubsub.FormatMessage("error", "Failed to open log file.")
				conn.WriteMessage(websocket.TextMessage, msg)
			}
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			// The file content is already NDJSON, send it directly
			if err := conn.WriteMessage(websocket.TextMessage, scanner.Bytes()); err != nil {
				return // Client disconnected
			}
		}

		if err := scanner.Err(); err != nil {
			zap.S().Errorf("error reading log file for container %s: %v", con.ID, err)
		}

		msg := pubsub.FormatMessage("info", "Log stream finished.")
		conn.WriteMessage(websocket.TextMessage, msg)
	}
	zap.S().Infof("admin websocket connection closed for container %s", containerID)
}
