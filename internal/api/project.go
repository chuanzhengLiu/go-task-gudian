package api

import (
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ProjectRequest struct {
	Title          string `json:"title" binding:"required"`
	Author         string `json:"author"`
	VersionInfo    string `json:"version_info"`
	StartPage      int    `json:"start_page"`
	EndPage        int    `json:"end_page" binding:"required,gtefield=StartPage"`
	ReviewRequired int    `json:"review_required"`
}

func CreateProject(c *gin.Context) {
	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var req ProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var instID uint64
	if userRole == string(model.RoleAdmin) {
		var reqInst struct {
			InstitutionID uint64 `json:"institution_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&reqInst); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "institution_id required for admin"})
			return
		}
		instID = reqInst.InstitutionID
	} else {
		if userInstID == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user not associated with any institution"})
			return
		}
		instID = *userInstID.(*uint64)
	}

	if req.ReviewRequired <= 0 {
		req.ReviewRequired = 1
	}

	project := model.Project{
		InstitutionID:  instID,
		Title:          req.Title,
		Author:         req.Author,
		VersionInfo:    req.VersionInfo,
		StartPage:      req.StartPage,
		EndPage:        req.EndPage,
		PageCount:      req.EndPage - req.StartPage + 1,
		Status:         model.ProjectStatusActive,
		CreatedBy:      userID,
		ReviewRequired: req.ReviewRequired,
	}

	if err := model.DB.Create(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	util.AuditLog(c, userID, "create_project", "project", project.ID, req)

	c.JSON(http.StatusCreated, project)
}

func ListProjects(c *gin.Context) {
	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	query := model.DB.Model(&model.Project{})

	if userRole != string(model.RoleAdmin) {
		if userInstID != nil {
			query = query.Where("institution_id = ?", *userInstID.(*uint64))
		} else {
			query = query.Where("id IN (SELECT project_id FROM pages WHERE assigned_to = ?)", userID)
		}
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var projects []model.Project
	query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&projects)

	c.JSON(http.StatusOK, gin.H{
		"items":     projects,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func GetProject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var project model.Project
	if err := model.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")
	userID := c.GetUint64("user_id")

	if userRole != string(model.RoleAdmin) {
		hasAccess := false
		if userInstID != nil && *userInstID.(*uint64) == project.InstitutionID {
			hasAccess = true
		} else {
			var count int64
			model.DB.Model(&model.Page{}).Where("project_id = ? AND assigned_to = ?", id, userID).Count(&count)
			hasAccess = count > 0
		}
		if !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var pages []model.Page
	model.DB.Where("project_id = ?", id).Order("page_number asc").Find(&pages)

	var stats struct {
		Total      int64 `json:"total"`
		Unassigned int64 `json:"unassigned"`
		Assigned   int64 `json:"assigned"`
		Proofing   int64 `json:"proofing"`
		Reviewing  int64 `json:"reviewing"`
		Completed  int64 `json:"completed"`
		Rejected   int64 `json:"rejected"`
	}
	model.DB.Model(&model.Page{}).Where("project_id = ?", id).Count(&stats.Total)
	model.DB.Model(&model.Page{}).Where("project_id = ? AND status = ?", id, model.PageStatusUnassigned).Count(&stats.Unassigned)
	model.DB.Model(&model.Page{}).Where("project_id = ? AND status = ?", id, model.PageStatusAssigned).Count(&stats.Assigned)
	model.DB.Model(&model.Page{}).Where("project_id = ? AND status = ?", id, model.PageStatusProofing).Count(&stats.Proofing)
	model.DB.Model(&model.Page{}).Where("project_id = ? AND status = ?", id, model.PageStatusReviewing).Count(&stats.Reviewing)
	model.DB.Model(&model.Page{}).Where("project_id = ? AND status = ?", id, model.PageStatusCompleted).Count(&stats.Completed)
	model.DB.Model(&model.Page{}).Where("project_id = ? AND status = ?", id, model.PageStatusRejected).Count(&stats.Rejected)

	c.JSON(http.StatusOK, gin.H{
		"project": project,
		"pages":   pages,
		"stats":   stats,
	})
}

func UpdateProject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var project model.Project
	if err := model.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	if userRole != string(model.RoleAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var req struct {
		Title          string             `json:"title"`
		Author         string             `json:"author"`
		VersionInfo    string             `json:"version_info"`
		Status         model.ProjectStatus `json:"status"`
		ReviewRequired int                `json:"review_required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title != "" {
		project.Title = req.Title
	}
	if req.Author != "" {
		project.Author = req.Author
	}
	if req.VersionInfo != "" {
		project.VersionInfo = req.VersionInfo
	}
	if req.Status != "" {
		project.Status = req.Status
	}
	if req.ReviewRequired > 0 {
		project.ReviewRequired = req.ReviewRequired
	}

	model.DB.Save(&project)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "update_project", "project", id, req)

	c.JSON(http.StatusOK, project)
}

func DeleteProject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var project model.Project
	if err := model.DB.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	if userRole != string(model.RoleAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	model.DB.Transaction(func(tx *gorm.DB) error {
		tx.Where("project_id = ?", id).Delete(&model.Correction{})
		tx.Where("project_id = ?", id).Delete(&model.CustomVariant{})
		tx.Where("project_id = ?", id).Delete(&model.VersionCompareTask{})
		tx.Where("project_id = ?", id).Delete(&model.ReviewRound{})
		tx.Where("project_id = ?", id).Delete(&model.Page{})
		tx.Delete(&project)
		return nil
	})

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "delete_project", "project", id, nil)

	c.JSON(http.StatusOK, gin.H{"message": "project deleted"})
}

func AssignPages(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var project model.Project
	if err := model.DB.First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if userRole != string(model.RoleAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var req struct {
		PageNumbers []int   `json:"page_numbers" binding:"required"`
		UserID      uint64  `json:"user_id" binding:"required"`
		StartPage   *int    `json:"start_page"`
		EndPage     *int    `json:"end_page"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	if err := model.DB.First(&user, req.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if user.InstitutionID == nil || *user.InstitutionID != project.InstitutionID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user not in project's institution"})
		return
	}

	pageNums := req.PageNumbers
	if req.StartPage != nil && req.EndPage != nil {
		pageNums = []int{}
		for i := *req.StartPage; i <= *req.EndPage; i++ {
			pageNums = append(pageNums, i)
		}
	}

	updates := map[string]interface{}{
		"assigned_to": req.UserID,
		"status":      model.PageStatusAssigned,
	}
	result := model.DB.Model(&model.Page{}).
		Where("project_id = ? AND page_number IN ?", projectID, pageNums).
		Updates(updates)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "assign_pages", "project", projectID, req)

	c.JSON(http.StatusOK, gin.H{
		"message":      "pages assigned successfully",
		"pages_updated": result.RowsAffected,
	})
}

func GetProjectBoard(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var pages []model.Page
	model.DB.Where("project_id = ?", id).Order("page_number asc").Find(&pages)

	board := map[model.PageStatus][]model.Page{
		model.PageStatusUnassigned: {},
		model.PageStatusAssigned:   {},
		model.PageStatusProofing:   {},
		model.PageStatusReviewing:  {},
		model.PageStatusCompleted:  {},
		model.PageStatusRejected:   {},
	}

	for _, page := range pages {
		board[page.Status] = append(board[page.Status], page)
	}

	c.JSON(http.StatusOK, board)
}
