package api

import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware provides a configurable CORS middleware that reads the
// `cors` settings row per-request via the SettingsStore.
func CORSMiddleware(settings *config.SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cors config.CORSConfig
		_ = settings.Get("cors", &cors)
		// If no origins are configured, do nothing.
		if len(cors.AllowedOrigins) == 0 {
			c.Next()
			return
		}

		origin := c.Request.Header.Get("Origin")
		allowOrigin := ""

		// Check if the origin is in the allowed list
		for _, o := range cors.AllowedOrigins {
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

func AuthMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
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

		userID := claims.Subject
		user, err := database.GetUserByID(db, userID)
		if err != nil {
			util.Error(c, http.StatusUnauthorized, "User not found")
			c.Abort()
			return
		}

		if user.BannedUntil != nil && time.Now().Before(*user.BannedUntil) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    -1,
				"message": "You have been banned from this service.",
				"data": gin.H{
					"ban_reason":   user.BanReason,
					"banned_until": user.BannedUntil.Format(time.RFC3339),
				},
			})
			c.Abort()
			return
		}

		c.Set("userID", claims.Subject)
		c.Set("role", string(user.Role))
		c.Next()
	}
}

// OptionalAuthMiddleware validates the Bearer token when present and sets
// "userID"/"role" in the context, but never rejects the request — callers
// with no or invalid token proceed as anonymous. Used on public routes that
// personalize their response (e.g. the tag-based effective deadline) for
// logged-in users.
func OptionalAuthMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Next()
			return
		}
		claims, err := auth.ValidateJWT(parts[1], secret)
		if err != nil {
			c.Next()
			return
		}
		user, err := database.GetUserByID(db, claims.Subject)
		if err != nil {
			c.Next()
			return
		}
		if user.BannedUntil != nil && time.Now().Before(*user.BannedUntil) {
			c.Next()
			return
		}
		c.Set("userID", claims.Subject)
		c.Set("role", string(user.Role))
		c.Next()
	}
}
func AssetsAuthMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		expires := c.Query("expires")

		if token == "" || expires == "" {
			util.Error(c, http.StatusUnauthorized, "Token and expires query parameters are required")
			c.Abort()
			return
		}

		expireTime, err := strconv.ParseInt(expires, 10, 64)
		if err != nil || time.Now().Unix() > expireTime {
			util.Error(c, http.StatusUnauthorized, "Token has expired")
			c.Abort()
			return
		}

		assetPath := c.Request.URL.Path
		message := fmt.Sprintf("%s|%d", assetPath, expireTime)

		mac := hmac.New(sha512.New, []byte(secret))
		mac.Write([]byte(message))
		expectedMAC := fmt.Sprintf("%x", mac.Sum(nil))

		if !hmac.Equal([]byte(expectedMAC), []byte(token)) {
			util.Error(c, http.StatusUnauthorized, "Invalid token")
			c.Abort()
			return
		}

		c.Next()
	}
}

// requireRole is a shared helper that runs JWT auth then enforces a role.
func requireRole(secret string, db *gorm.DB, allowSuperAdminOnly bool) gin.HandlerFunc {
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

		claims, err := auth.ValidateJWT(parts[1], secret)
		if err != nil {
			util.Error(c, http.StatusUnauthorized, err.Error())
			c.Abort()
			return
		}

		userID := claims.Subject
		user, err := database.GetUserByID(db, userID)
		if err != nil {
			util.Error(c, http.StatusUnauthorized, "User not found")
			c.Abort()
			return
		}

		if user.BannedUntil != nil && time.Now().Before(*user.BannedUntil) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    -1,
				"message": "You have been banned from this service.",
				"data": gin.H{
					"ban_reason":   user.BanReason,
					"banned_until": user.BannedUntil.Format(time.RFC3339),
				},
			})
			c.Abort()
			return
		}

		// Use the DB-loaded role (source of truth), not the JWT claim, so a
		// demoted admin loses access even with a still-valid token.
		role := string(user.Role)
		if role != string(models.RoleAdmin) && role != string(models.RoleSuperAdmin) {
			util.Error(c, http.StatusForbidden, "admin privileges required")
			c.Abort()
			return
		}
		if allowSuperAdminOnly && role != string(models.RoleSuperAdmin) {
			util.Error(c, http.StatusForbidden, "superadmin privileges required")
			c.Abort()
			return
		}

		c.Set("userID", claims.Subject)
		c.Set("role", role)
		c.Next()
	}
}

// AdminMiddleware allows admin and superadmin users.
func AdminMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
	return requireRole(secret, db, false)
}

// SuperAdminMiddleware allows only superadmin users.
func SuperAdminMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
	return requireRole(secret, db, true)
}
