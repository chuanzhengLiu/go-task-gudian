package api

import (
	"ancient-texts-backend/internal/config"
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"gorm.io/gorm"
)

type PageCreateRequest struct {
	ProjectID    uint64 `json:"project_id" binding:"required"`
	PageNumber   int    `json:"page_number" binding:"required,min=1"`
	ImagePath    string `json:"image_path"`
	TilePath     string `json:"tile_path"`
	OCRText      string `json:"ocr_text"`
	Version      string `json:"version"`
	VersionLabel string `json:"version_label"`
}

type CorrectionRequest struct {
	Type          model.CorrectionType `json:"type" binding:"required"`
	StartPosition int                  `json:"start_position" binding:"required,min=0"`
	EndPosition   int                  `json:"end_position"`
	OriginalText  string               `json:"original_text"`
	CorrectedText string               `json:"corrected_text"`
	Note          string               `json:"note"`
	Emendation    string               `json:"emendation"`
}

type AutoSaveRequest struct {
	CorrectedText string             `json:"corrected_text"`
	Corrections   []CorrectionRequest `json:"corrections"`
}

var xssPolicy = bluemonday.UGCPolicy()

func CreatePage(c *gin.Context) {
	var req PageCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var project model.Project
	if err := model.DB.First(&project, req.ProjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if userRole == string(model.RoleInstAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var count int64
	model.DB.Model(&model.Page{}).Where("project_id = ? AND page_number = ? AND version = ?",
		req.ProjectID, req.PageNumber, req.Version).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "page already exists"})
		return
	}

	if req.Version == "" {
		req.Version = "main"
	}

	page := model.Page{
		ProjectID:     req.ProjectID,
		PageNumber:    req.PageNumber,
		ImagePath:     req.ImagePath,
		TilePath:      req.TilePath,
		OCRText:       xssPolicy.Sanitize(req.OCRText),
		CorrectedText: xssPolicy.Sanitize(req.OCRText),
		Status:        model.PageStatusUnassigned,
		Version:       req.Version,
		VersionLabel:  req.VersionLabel,
	}

	if err := model.DB.Create(&page).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create page"})
		return
	}

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "create_page", "page", page.ID, req)

	c.JSON(http.StatusCreated, page)
}

func GetPage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var page model.Page
	if err := model.DB.First(&page, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var project model.Project
	model.DB.First(&project, page.ProjectID)

	hasAccess := false
	if userRole == string(model.RoleAdmin) {
		hasAccess = true
	} else if userRole == string(model.RoleInstAdmin) {
		if userInstID != nil && *userInstID.(*uint64) == project.InstitutionID {
			hasAccess = true
		}
	} else if userRole == string(model.RoleTypesetter) {
		if page.Status == model.PageStatusCompleted {
			if userInstID != nil && *userInstID.(*uint64) == project.InstitutionID {
				hasAccess = true
			}
		}
	} else if userInstID != nil && *userInstID.(*uint64) == project.InstitutionID {
		hasAccess = true
	} else if page.AssignedTo != nil && *page.AssignedTo == userID {
		hasAccess = true
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var signedImageURL string
	if page.ImagePath != "" {
		token := util.GenerateImageToken(page.ImagePath, config.AppConfig.IMAGE_SIGN_SECRET, config.AppConfig.IMAGE_SIGN_TTL)
		signedImageURL = fmt.Sprintf("/api/images/%s?token=%s", page.ImagePath, token)
	}

	var signedTileURL string
	if page.TilePath != "" {
		token := util.GenerateImageToken(page.TilePath, config.AppConfig.IMAGE_SIGN_SECRET, config.AppConfig.IMAGE_SIGN_TTL)
		signedTileURL = fmt.Sprintf("/api/tiles/%s.dzi?token=%s", page.TilePath, token)
	}

	var corrections []model.Correction
	model.DB.Where("page_id = ?", id).Order("start_position asc").Find(&corrections)

	var reviews []model.ReviewRound
	model.DB.Where("page_id = ?", id).Order("round_num desc, created_at desc").Find(&reviews)

	c.JSON(http.StatusOK, gin.H{
		"page":            page,
		"corrections":     corrections,
		"reviews":         reviews,
		"signed_image_url": signedImageURL,
		"signed_tile_url": signedTileURL,
	})
}

func UpdatePage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var page model.Page
	if err := model.DB.First(&page, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var project model.Project
	model.DB.First(&project, page.ProjectID)

	hasAccess := false
	if userRole == string(model.RoleAdmin) || userRole == string(model.RoleInstAdmin) {
		if userRole == string(model.RoleInstAdmin) {
			if userInstID != nil && *userInstID.(*uint64) == project.InstitutionID {
				hasAccess = true
			}
		} else {
			hasAccess = true
		}
	} else if page.AssignedTo != nil && *page.AssignedTo == userID {
		hasAccess = true
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		CorrectedText string `json:"corrected_text"`
		Status        string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.CorrectedText != "" {
		page.CorrectedText = xssPolicy.Sanitize(req.CorrectedText)
	}
	if req.Status != "" {
		page.Status = model.PageStatus(req.Status)
	}

	model.DB.Save(&page)

	util.AuditLog(c, userID, "update_page", "page", id, req)

	c.JSON(http.StatusOK, page)
}

func LockPage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var page model.Page
	if err := model.DB.First(&page, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	userID := c.GetUint64("user_id")

	if page.LockBy != nil && *page.LockBy != userID {
		if page.LockAt != nil && time.Since(*page.LockAt) > 5*time.Minute {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "page is locked by another user",
				"lock_by": page.LockBy,
				"lock_at": page.LockAt,
			})
			return
		}
	}

	now := time.Now()
	page.LockBy = &userID
	page.LockAt = &now

	if page.Status == model.PageStatusAssigned {
		page.Status = model.PageStatusProofing
	}

	model.DB.Save(&page)

	c.JSON(http.StatusOK, page)
}

func UnlockPage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var page model.Page
	if err := model.DB.First(&page, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	userID := c.GetUint64("user_id")
	if page.LockBy != nil && *page.LockBy != userID {
		userRole := c.GetString("user_role")
		if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot unlock page locked by another user"})
			return
		}
	}

	page.LockBy = nil
	page.LockAt = nil
	model.DB.Save(&page)

	c.JSON(http.StatusOK, gin.H{"message": "page unlocked"})
}

func CreateCorrection(c *gin.Context) {
	pageID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page id"})
		return
	}

	var page model.Page
	if err := model.DB.First(&page, pageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")

	var project model.Project
	model.DB.First(&project, page.ProjectID)

	hasAccess := false
	if userRole == string(model.RoleAdmin) || userRole == string(model.RoleInstAdmin) {
		hasAccess = true
	} else if page.AssignedTo != nil && *page.AssignedTo == userID {
		hasAccess = true
	} else if userRole == string(model.RoleProofreader2) {
		hasAccess = true
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if page.LockBy != nil && *page.LockBy != userID {
		c.JSON(http.StatusConflict, gin.H{"error": "page is locked by another user"})
		return
	}

	var req CorrectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validTypes := []model.CorrectionType{
		model.CorrectionTypeWrong, model.CorrectionTypeMissing,
		model.CorrectionTypeExtra, model.CorrectionTypeReversed,
		model.CorrectionTypeVariant,
	}
	if !util.Contains(validTypes, req.Type) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid correction type"})
		return
	}

	correction := model.Correction{
		PageID:        pageID,
		Type:          req.Type,
		StartPosition: req.StartPosition,
		EndPosition:   req.EndPosition,
		OriginalText:  xssPolicy.Sanitize(req.OriginalText),
		CorrectedText: xssPolicy.Sanitize(req.CorrectedText),
		Note:          xssPolicy.Sanitize(req.Note),
		Emendation:    xssPolicy.Sanitize(req.Emendation),
		CreatedBy:     userID,
	}

	if err := model.DB.Create(&correction).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create correction"})
		return
	}

	util.AuditLog(c, userID, "create_correction", "correction", correction.ID, req)

	c.JSON(http.StatusCreated, correction)
}

func UpdateCorrection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("cid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var correction model.Correction
	if err := model.DB.First(&correction, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "correction not found"})
		return
	}

	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")

	if correction.CreatedBy != userID && userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req CorrectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type != "" {
		correction.Type = req.Type
	}
	correction.StartPosition = req.StartPosition
	correction.EndPosition = req.EndPosition
	correction.OriginalText = xssPolicy.Sanitize(req.OriginalText)
	correction.CorrectedText = xssPolicy.Sanitize(req.CorrectedText)
	correction.Note = xssPolicy.Sanitize(req.Note)
	correction.Emendation = xssPolicy.Sanitize(req.Emendation)

	model.DB.Save(&correction)

	util.AuditLog(c, userID, "update_correction", "correction", id, req)

	c.JSON(http.StatusOK, correction)
}

func DeleteCorrection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("cid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var correction model.Correction
	if err := model.DB.First(&correction, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "correction not found"})
		return
	}

	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")

	if correction.CreatedBy != userID && userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	model.DB.Delete(&correction)

	util.AuditLog(c, userID, "delete_correction", "correction", id, nil)

	c.JSON(http.StatusOK, gin.H{"message": "correction deleted"})
}

func AutoSave(c *gin.Context) {
	pageID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page id"})
		return
	}

	var page model.Page
	if err := model.DB.First(&page, pageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	userID := c.GetUint64("user_id")

	if page.AssignedTo != nil && *page.AssignedTo != userID {
		userRole := c.GetString("user_role")
		if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var req AutoSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	page.CorrectedText = xssPolicy.Sanitize(req.CorrectedText)
	page.AutoSavedAt = &now
	model.DB.Save(&page)

	if len(req.Corrections) > 0 {
		model.DB.Where("page_id = ? AND created_by = ?", pageID, userID).Delete(&model.Correction{})
		for _, corr := range req.Corrections {
			correction := model.Correction{
				PageID:        pageID,
				Type:          corr.Type,
				StartPosition: corr.StartPosition,
				EndPosition:   corr.EndPosition,
				OriginalText:  xssPolicy.Sanitize(corr.OriginalText),
				CorrectedText: xssPolicy.Sanitize(corr.CorrectedText),
				Note:          xssPolicy.Sanitize(corr.Note),
				Emendation:    xssPolicy.Sanitize(corr.Emendation),
				CreatedBy:     userID,
			}
			model.DB.Create(&correction)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "auto saved",
		"saved_at":    now,
		"corrections_count": len(req.Corrections),
	})
}

func SubmitForReview(c *gin.Context) {
	pageID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page id"})
		return
	}

	var page model.Page
	if err := model.DB.First(&page, pageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	userID := c.GetUint64("user_id")

	if page.AssignedTo != nil && *page.AssignedTo != userID {
		userRole := c.GetString("user_role")
		if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	page.Status = model.PageStatusReviewing
	page.LockBy = nil
	page.LockAt = nil
	model.DB.Save(&page)

	review := model.ReviewRound{
		PageID:     pageID,
		ReviewerID: 0,
		RoundNum:   1,
		Status:     model.ReviewStatusPending,
	}
	model.DB.Create(&review)

	util.AuditLog(c, userID, "submit_for_review", "page", pageID, nil)

	c.JSON(http.StatusOK, gin.H{
		"message": "submitted for review",
		"review_id": review.ID,
	})
}

func GetPageCorrections(c *gin.Context) {
	pageID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page id"})
		return
	}

	var corrections []model.Correction
	model.DB.Where("page_id = ?", pageID).Order("start_position asc").Find(&corrections)

	c.JSON(http.StatusOK, corrections)
}

func BatchCreatePages(c *gin.Context) {
	var req struct {
		ProjectID uint64   `json:"project_id" binding:"required"`
		Pages     []PageCreateRequest `json:"pages" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	var project model.Project
	if err := model.DB.First(&project, req.ProjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if userRole == string(model.RoleInstAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var createdPages []model.Page
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, p := range req.Pages {
			if p.Version == "" {
				p.Version = "main"
			}
			page := model.Page{
				ProjectID:     req.ProjectID,
				PageNumber:    p.PageNumber,
				ImagePath:     p.ImagePath,
				TilePath:      p.TilePath,
				OCRText:       xssPolicy.Sanitize(p.OCRText),
				CorrectedText: xssPolicy.Sanitize(p.OCRText),
				Status:        model.PageStatusUnassigned,
				Version:       p.Version,
				VersionLabel:  p.VersionLabel,
			}
			if err := tx.Create(&page).Error; err != nil {
				return err
			}
			createdPages = append(createdPages, page)
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create pages: " + err.Error()})
		return
	}

	project.PageCount += len(createdPages)
	model.DB.Save(&project)

	userID := c.GetUint64("user_id")
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"project_id": req.ProjectID,
		"count":      len(createdPages),
	})
	util.AuditLog(c, userID, "batch_create_pages", "project", req.ProjectID, json.RawMessage(detailsJSON))

	c.JSON(http.StatusCreated, gin.H{
		"message": "pages created successfully",
		"count":   len(createdPages),
		"pages":   createdPages,
	})
}
