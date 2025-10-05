package api

import (
	"net/http"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/util"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware provides a configurable CORS middleware.
func CORSMiddleware(cfg config.CORS) gin.HandlerFunc {
	return func(c *gin.Context) {
		// If no origins are configured, do nothing.
		if len(cfg.AllowedOrigins) == 0 {
			c.Next()
			return
		}

		origin := c.Request.Header.Get("Origin")
		allowOrigin := ""

		// Check if the origin is in the allowed list
		for _, o := range cfg.AllowedOrigins {
			if o == "*" {
				allowOrigin = "*"
				break
			}
			if o == origin {
				allowOrigin = origin
				break
			}
		}

		// Only set headers if the origin is allowed.
		if allowOrigin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")

			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

func AuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			util.Error(c, http.StatusUnauthorized, "Authorization header is required")
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			util.Error(c, http.StatusUnauthorized, "Authorization header format must be Bearer {token}")
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := auth.ValidateJWT(tokenString, secret)
		if err != nil {
			util.Error(c, http.StatusUnauthorized, err.Error())
			c.Abort()
			return
		}

		c.Set("userID", claims.Subject)
		c.Next()
	}
}
