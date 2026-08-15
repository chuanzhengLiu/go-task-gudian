package middleware

import (
	"ancient-texts-backend/internal/auth"
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
}

var (
	limiter = &rateLimiter{requests: make(map[string][]time.Time)}
)

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		claims, err := auth.ParseAccessToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", string(claims.Role))
		c.Set("institution_id", claims.InstID)
		c.Next()
	}
}

func RoleRequired(roles ...model.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := c.GetString("user_role")
		if userRole == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		roleStrs := make([]string, len(roles))
		for i, r := range roles {
			roleStrs[i] = string(r)
		}

		if !util.Contains(roleStrs, userRole) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return RoleRequired(model.RoleAdmin)
}

func InstAdminOrHigher() gin.HandlerFunc {
	return RoleRequired(model.RoleAdmin, model.RoleInstAdmin)
}

func ProofreaderOrHigher() gin.HandlerFunc {
	return RoleRequired(model.RoleAdmin, model.RoleInstAdmin, model.RoleProofreader1, model.RoleProofreader2)
}

func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := util.GetClientIP(c)
		key := ip + ":" + c.FullPath()

		limiter.mu.Lock()
		defer limiter.mu.Unlock()

		now := time.Now()
		var recent []time.Time
		for _, t := range limiter.requests[key] {
			if now.Sub(t) < window {
				recent = append(recent, t)
			}
		}

		if len(recent) >= limit {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}

		limiter.requests[key] = append(recent, now)
		c.Next()
	}
}

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("X-DNS-Prefetch-Control", "off")
		c.Header("X-Download-Options", "noopen")
		c.Header("X-Permitted-Cross-Domain-Policies", "none")
		c.Next()
	}
}

func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		publicPaths := map[string]bool{
			"/api/auth/login":   true,
			"/api/auth/register": true,
			"/api/auth/refresh": true,
			"/api/health":       true,
		}
		path := c.Request.URL.Path
		if publicPaths[path] {
			c.Next()
			return
		}

		if c.Request.Method == "GET" || c.Request.Method == "HEAD" {
			csrfToken := c.GetHeader("X-CSRF-Token")
			if csrfToken == "" {
				csrfToken = generateCSRFToken()
			}
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie("csrf_token", csrfToken, 3600*24, "/", "", false, false)
			c.Header("X-CSRF-Token", csrfToken)
			c.Set("csrf_token", csrfToken)
			c.Next()
			return
		}

		cookieToken, err := c.Cookie("csrf_token")
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "CSRF token missing"})
			c.Abort()
			return
		}

		headerToken := c.GetHeader("X-CSRF-Token")
		if headerToken == "" || headerToken != cookieToken {
			c.JSON(http.StatusForbidden, gin.H{"error": "CSRF token invalid"})
			c.Abort()
			return
		}

		c.SetSameSite(http.SameSiteStrictMode)
		c.Next()
	}
}

func generateCSRFToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
