package embedui

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:sites
var embedFS embed.FS

// RegisterUIHandlers registers a middleware to serve the embedded UI.
func RegisterUIHandlers(router *gin.Engine, sitename string) {
	// Create a sub-filesystem that starts from the 'sites' directory.
	uiFS, err := fs.Sub(embedFS, "sites/"+sitename)
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

		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "" { // Handle root path explicitly
			path = "index.html"
		}

		// Check if the file exists as is
		_, err = uiFS.Open(path)
		if err != nil {
			// If the file doesn't exist AND the path has no extension,
			// try appending .html.
			if filepath.Ext(path) == "" {
				htmlPath := path + ".html"
				_, err = uiFS.Open(htmlPath)
				if err == nil {
					// If the .html version exists, rewrite the path to serve it.
					c.Request.URL.Path = "/" + htmlPath
				} else {
					// Otherwise, fall back to the SPA's root index.html.
					c.Request.URL.Path = "/"
				}
			} else {
				// If the file with an extension doesn't exist, fall back to the SPA root.
				c.Request.URL.Path = "/"
			}
		}

		// Let the standard http.FileServer handle the request with the (potentially modified) path.
		fileServer.ServeHTTP(c.Writer, c.Request)

		// Abort the middleware chain.
		c.Abort()
	})
}
