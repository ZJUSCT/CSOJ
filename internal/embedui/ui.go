package embedui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed ui/**
var embedFS embed.FS

// RegisterUIHandlers registers a middleware to serve the embedded UI.
func RegisterUIHandlers(router *gin.Engine) {
	// Create a sub-filesystem that starts from the 'ui' directory.
	uiFS, err := fs.Sub(embedFS, "ui")
	if err != nil {
		panic("embedui: failed to create sub filesystem: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(uiFS))

	// Use a middleware to handle all requests.
	router.Use(func(c *gin.Context) {
		// We only serve static files for GET and HEAD requests.
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}

		// Do not interfere with API or WebSocket routes.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/ws/") {
			c.Next()
			return
		}

		// Attempt to open the file from the embedded filesystem.
		// We check for its existence to decide whether to serve it directly
		// or fall back to index.html for SPA routing.
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		_, err := uiFS.Open(path)

		if err != nil {
			// If the file is not found, we rewrite the request path to serve the root index.html.
			// This is the crucial part for Single-Page Application (SPA) routing to work.
			c.Request.URL.Path = "/"
		}

		// Let the standard http.FileServer handle the request.
		// It will serve the original path if the file exists, or index.html if we rewrote the path.
		fileServer.ServeHTTP(c.Writer, c.Request)

		// Abort the middleware chain. This is important as we've already handled the request.
		c.Abort()
	})
}
