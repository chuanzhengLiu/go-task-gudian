package api

import (
	"ancient-texts-backend/internal/auth"
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	TOTPCode string `json:"totp_code,omitempty"`
}

type RegisterRequest struct {
	Email     string      `json:"email" binding:"required,email"`
	Password  string      `json:"password" binding:"required"`
	Name      string      `json:"name" binding:"required"`
	Role      model.Role  `json:"role" binding:"required"`
	InstID    *uint64     `json:"institution_id,omitempty"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	if err := model.DB.Where("email = ?", strings.ToLower(req.Email)).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱或密码错误"})
		return
	}

	if user.Status != model.UserStatusActive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账户未激活"})
		return
	}

	if !util.CheckPasswordHash(req.Password, user.PasswordHash) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱或密码错误"})
		return
	}

	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			c.JSON(http.StatusOK, gin.H{"totp_required": true, "message": "TOTP code required"})
			return
		}
		if !totp.Validate(req.TOTPCode, user.TOTPSecret) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "TOTP验证码错误"})
			return
		}
	}

	if user.InstitutionID != nil {
		var inst model.Institution
		if err := model.DB.First(&inst, *user.InstitutionID).Error; err == nil {
			if inst.TOTPRequired && !user.TOTPEnabled {
				c.JSON(http.StatusOK, gin.H{"totp_required_by_admin": true, "message": "TOTP required by institution administrator"})
				return
			}
			if inst.PasswordChangeForced && inst.PasswordExpiryDays > 0 {
				expiryDate := user.LastPasswordChange.AddDate(0, 0, inst.PasswordExpiryDays)
				if time.Now().After(expiryDate) {
					user.PasswordExpired = true
					model.DB.Model(&user).Update("password_expired", true)
				}
			}
		}
	}

	tokens, err := auth.GenerateTokens(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate tokens"})
		return
	}

	expiresAt := time.Now().AddDate(0, 0, 7)
	session := model.Session{
		UserID:       user.ID,
		RefreshToken: tokens.RefreshToken,
		UserAgent:    c.GetHeader("User-Agent"),
		IPAddress:    util.GetClientIP(c),
		ExpiresAt:    expiresAt,
		IsActive:     true,
	}
	model.DB.Create(&session)

	util.AuditLog(c, user.ID, "login", "user", user.ID, nil)

	if user.PasswordExpired {
		c.JSON(http.StatusOK, gin.H{
			"tokens": tokens,
			"user":   user,
			"password_expired": true,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tokens": tokens,
		"user":   user,
	})
}

func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !util.ValidatePassword(req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 10 characters and contain uppercase, lowercase, and numbers"})
		return
	}

	var count int64
	model.DB.Model(&model.User{}).Where("email = ?", strings.ToLower(req.Email)).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}

	validRoles := []model.Role{
		model.RoleAdmin, model.RoleInstAdmin,
		model.RoleProofreader1, model.RoleProofreader2, model.RoleTypesetter,
	}
	if !util.Contains(validRoles, req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	hashedPassword, err := util.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := model.User{
		Email:            strings.ToLower(req.Email),
		PasswordHash:     hashedPassword,
		Name:             req.Name,
		Role:             req.Role,
		InstitutionID:    req.InstID,
		Status:           model.UserStatusPending,
		LastPasswordChange: time.Now(),
	}

	if err := model.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	util.AuditLog(c, user.ID, "register", "user", user.ID, map[string]interface{}{
		"role":  req.Role,
		"email": req.Email,
	})

	c.JSON(http.StatusCreated, gin.H{
		"message": "user registered successfully, pending approval",
		"user_id": user.ID,
	})
}

func RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var session model.Session
	if err := model.DB.Where("refresh_token = ? AND is_active = true AND expires_at > ?",
		req.RefreshToken, time.Now()).First(&session).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	claims, err := auth.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		model.DB.Model(&session).Update("is_active", false)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	var user model.User
	if err := model.DB.First(&user, claims.UserID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	tokens, err := auth.GenerateTokens(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate tokens"})
		return
	}

	model.DB.Model(&session).Updates(map[string]interface{}{
		"refresh_token": tokens.RefreshToken,
		"expires_at":    time.Now().AddDate(0, 0, 7),
	})

	c.JSON(http.StatusOK, tokens)
}

func Logout(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model.DB.Where("refresh_token = ? AND user_id = ?", req.RefreshToken, userID).
		Update("is_active", false)

	util.AuditLog(c, userID, "logout", "user", userID, nil)

	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

func GetCurrentUser(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func ChangePassword(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if !util.CheckPasswordHash(req.OldPassword, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid old password"})
		return
	}

	if !util.ValidatePassword(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 10 characters and contain uppercase, lowercase, and numbers"})
		return
	}

	hashedPassword, err := util.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user.PasswordHash = hashedPassword
	user.LastPasswordChange = time.Now()
	user.PasswordExpired = false
	model.DB.Save(&user)

	util.AuditLog(c, userID, "change_password", "user", userID, nil)

	c.JSON(http.StatusOK, gin.H{"message": "password changed successfully"})
}

func GetSessions(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var sessions []model.Session
	model.DB.Where("user_id = ? AND is_active = true AND expires_at > ?", userID, time.Now()).
		Order("created_at desc").Find(&sessions)
	c.JSON(http.StatusOK, sessions)
}

func RevokeSession(c *gin.Context) {
	userID := c.GetUint64("user_id")
	sessionID := c.Param("id")
	var session model.Session
	if err := model.DB.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	model.DB.Model(&session).Update("is_active", false)
	util.AuditLog(c, userID, "revoke_session", "session", session.ID, nil)
	c.JSON(http.StatusOK, gin.H{"message": "session revoked"})
}

func GetTOTPSecret(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var user model.User
	model.DB.First(&user, userID)

	if user.TOTPEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "TOTP already enabled"})
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "AncientTexts",
		AccountName: user.Email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate TOTP secret"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"secret":     key.Secret(),
		"qr_url":     key.URL(),
		"account":    user.Email,
	})
}

func EnableTOTP(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var req struct {
		Secret string `json:"secret" binding:"required"`
		Code   string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !totp.Validate(req.Code, req.Secret) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid TOTP code"})
		return
	}

	model.DB.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"totp_secret": req.Secret,
		"totp_enabled": true,
	})

	util.AuditLog(c, userID, "enable_totp", "user", userID, nil)

	c.JSON(http.StatusOK, gin.H{"message": "TOTP enabled successfully"})
}

func DisableTOTP(c *gin.Context) {
	userID := c.GetUint64("user_id")
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	model.DB.First(&user, userID)

	if !totp.Validate(req.Code, user.TOTPSecret) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid TOTP code"})
		return
	}

	model.DB.Model(&user).Updates(map[string]interface{}{
		"totp_secret": "",
		"totp_enabled": false,
	})

	util.AuditLog(c, userID, "disable_totp", "user", userID, nil)

	c.JSON(http.StatusOK, gin.H{"message": "TOTP disabled successfully"})
}
