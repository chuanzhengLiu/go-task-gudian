package util

import (
	"ancient-texts-backend/internal/config"
	"ancient-texts-backend/internal/model"
	"encoding/json"
	"net"
	"regexp"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), config.AppConfig.BCRYPT_COST)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func ValidatePassword(password string) bool {
	if len(password) < 10 {
		return false
	}
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	return hasUpper && hasLower
}

func GetClientIP(c *gin.Context) string {
	ip := c.GetHeader("X-Forwarded-For")
	if ip == "" {
		ip = c.GetHeader("X-Real-IP")
	}
	if ip == "" {
		ip, _, _ = net.SplitHostPort(c.Request.RemoteAddr)
	}
	return ip
}

func AuditLog(c *gin.Context, userID uint64, action string, targetType string, targetID uint64, details interface{}) {
	var detailsStr string
	if details != nil {
		jsonBytes, _ := json.Marshal(details)
		detailsStr = string(jsonBytes)
	}

	log := model.AuditLog{
		UserID:     userID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		IPAddress:  GetClientIP(c),
		Details:    detailsStr,
	}
	model.DB.Create(&log)
}

func Contains[T comparable](slice []T, item T) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
