package api

import (
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type InstitutionRequest struct {
	Name   string                `json:"name" binding:"required"`
	Type   model.InstitutionType `json:"type" binding:"required"`
}

func RegisterInstitution(c *gin.Context) {
	var req InstitutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validTypes := []model.InstitutionType{
		model.InstTypeUniversity, model.InstTypePress,
		model.InstTypeLocalGov, model.InstTypeOther,
	}
	if !util.Contains(validTypes, req.Type) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid institution type"})
		return
	}

	userID := c.GetUint64("user_id")

	var count int64
	model.DB.Model(&model.Institution{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "institution name already exists"})
		return
	}

	institution := model.Institution{
		Name:   req.Name,
		Type:   req.Type,
		Status: model.InstStatusPending,
		AdminID: &userID,
	}

	if err := model.DB.Create(&institution).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register institution"})
		return
	}

	util.AuditLog(c, userID, "register_institution", "institution", institution.ID, req)

	c.JSON(http.StatusCreated, gin.H{
		"message": "institution registered successfully, pending approval",
		"id":      institution.ID,
	})
}

func ListInstitutions(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	query := model.DB.Model(&model.Institution{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var institutions []model.Institution
	query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&institutions)

	c.JSON(http.StatusOK, gin.H{
		"items": institutions,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

func GetInstitution(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var institution model.Institution
	if err := model.DB.First(&institution, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "institution not found"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")
	if userRole != string(model.RoleAdmin) && userInstID != nil {
		if *userInstID.(*uint64) != institution.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	c.JSON(http.StatusOK, institution)
}

func ApproveInstitution(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var institution model.Institution
	if err := model.DB.First(&institution, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "institution not found"})
		return
	}

	var req struct {
		Approve bool   `json:"approve" binding:"required"`
		Reason  string `json:"reason,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Approve {
		institution.Status = model.InstStatusApproved
		if institution.AdminID != nil {
			model.DB.Model(&model.User{}).Where("id = ?", *institution.AdminID).Updates(map[string]interface{}{
				"institution_id": institution.ID,
				"status":         model.UserStatusActive,
				"role":           model.RoleInstAdmin,
			})
		}
	} else {
		institution.Status = model.InstStatusRejected
	}

	model.DB.Save(&institution)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "approve_institution", "institution", institution.ID, req)

	c.JSON(http.StatusOK, gin.H{"message": "institution status updated"})
}

func UpdateInstitution(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")
	if userRole != string(model.RoleAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != id {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var institution model.Institution
	if err := model.DB.First(&institution, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "institution not found"})
		return
	}

	var req struct {
		Name string                `json:"name"`
		Type model.InstitutionType `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		institution.Name = req.Name
	}
	if req.Type != "" {
		institution.Type = req.Type
	}

	model.DB.Save(&institution)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "update_institution", "institution", id, req)

	c.JSON(http.StatusOK, institution)
}

func ListInstitutionUsers(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")
	if userRole != string(model.RoleAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != id {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var users []model.User
	model.DB.Where("institution_id = ?", id).Find(&users)

	c.JSON(http.StatusOK, users)
}

func InviteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")
	if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if userRole == string(model.RoleInstAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != id {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var req struct {
		Email    string     `json:"email" binding:"required,email"`
		Password string     `json:"password" binding:"required"`
		Name     string     `json:"name" binding:"required"`
		Role     model.Role `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validRoles := []model.Role{model.RoleInstAdmin, model.RoleProofreader1, model.RoleProofreader2, model.RoleTypesetter}
	if !util.Contains(validRoles, req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	if !util.ValidatePassword(req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 10 characters and contain uppercase, lowercase, and numbers"})
		return
	}

	var count int64
	model.DB.Model(&model.User{}).Where("email = ?", req.Email).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}

	hashedPassword, _ := util.HashPassword(req.Password)
	instID := id
	user := model.User{
		Email:            req.Email,
		PasswordHash:     hashedPassword,
		Name:             req.Name,
		Role:             req.Role,
		InstitutionID:    &instID,
		Status:           model.UserStatusActive,
		LastPasswordChange: time.Now(),
	}
	model.DB.Create(&user)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "invite_user", "user", user.ID, req)

	c.JSON(http.StatusCreated, user)
}

func UpdateUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	currentUserRole := c.GetString("user_role")
	currentUserInstID, _ := c.Get("institution_id")

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if currentUserRole != string(model.RoleAdmin) {
		if currentUserInstID == nil || user.InstitutionID == nil || *currentUserInstID.(*uint64) != *user.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var req struct {
		Name   string     `json:"name"`
		Role   model.Role `json:"role"`
		Status string     `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Status != "" {
		user.Status = model.UserStatus(req.Status)
	}

	model.DB.Save(&user)

	currUserID := c.GetUint64("user_id")
	util.AuditLog(c, currUserID, "update_user", "user", userID, req)

	c.JSON(http.StatusOK, user)
}

func DeleteUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	currentUserRole := c.GetString("user_role")
	currentUserInstID, _ := c.Get("institution_id")

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if currentUserRole != string(model.RoleAdmin) {
		if currentUserInstID == nil || user.InstitutionID == nil || *currentUserInstID.(*uint64) != *user.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	model.DB.Delete(&user)

	currUserID := c.GetUint64("user_id")
	util.AuditLog(c, currUserID, "delete_user", "user", userID, nil)

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

func GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID := c.Query("user_id")
	action := c.Query("action")
	targetType := c.Query("target_type")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := model.DB.Model(&model.AuditLog{}).Order("created_at desc")

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate)
	}

	currentUserRole := c.GetString("user_role")
	if currentUserRole != string(model.RoleAdmin) {
		currentUserInstID, _ := c.Get("institution_id")
		if currentUserInstID != nil {
			query = query.Joins("JOIN users ON users.id = audit_logs.user_id").
				Where("users.institution_id = ?", *currentUserInstID.(*uint64))
		}
	}

	var total int64
	query.Count(&total)

	var logs []model.AuditLog
	query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"items":     logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
