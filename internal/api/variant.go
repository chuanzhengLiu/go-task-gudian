package api

import (
	"ancient-texts-backend/internal/diff"
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type VariantRequest struct {
	VariantChar  string `json:"variant_char" binding:"required,min=1,max=10"`
	StandardChar string `json:"standard_char" binding:"required,min=1,max=10"`
	Source       string `json:"source"`
}

type CompareRequest struct {
	VersionAPageID uint64 `json:"version_a_page_id" binding:"required"`
	VersionBPageID uint64 `json:"version_b_page_id" binding:"required"`
	VersionALabel  string `json:"version_a_label"`
	VersionBLabel  string `json:"version_b_label"`
}

func LoadVariantMap() map[string]string {
	var variants []model.VariantChar
	model.DB.Where("verified = ?", true).Find(&variants)

	variantMap := make(map[string]string)
	for _, v := range variants {
		variantMap[v.VariantChar] = v.StandardChar
	}

	return variantMap
}

func LoadProjectVariantMap(projectID uint64) map[string]string {
	variantMap := LoadVariantMap()

	var customVariants []model.CustomVariant
	model.DB.Where("project_id = ?", projectID).Find(&customVariants)

	for _, v := range customVariants {
		variantMap[v.VariantChar] = v.StandardChar
	}

	return variantMap
}

func CreateVariant(c *gin.Context) {
	var req VariantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var count int64
	model.DB.Model(&model.VariantChar{}).Where("variant_char = ?", req.VariantChar).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "variant character already exists"})
		return
	}

	variant := model.VariantChar{
		VariantChar:  req.VariantChar,
		StandardChar: req.StandardChar,
		Source:       req.Source,
		Verified:     false,
	}
	model.DB.Create(&variant)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "create_variant", "variant_char", variant.ID, req)

	c.JSON(http.StatusCreated, variant)
}

func ListVariants(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if pageSize < 1 {
		pageSize = 50
	}
	search := c.Query("search")
	verified := c.Query("verified")

	query := model.DB.Model(&model.VariantChar{})
	if search != "" {
		query = query.Where("variant_char LIKE ? OR standard_char LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if verified != "" {
		v, _ := strconv.ParseBool(verified)
		query = query.Where("verified = ?", v)
	}

	var total int64
	query.Count(&total)

	var variants []model.VariantChar
	query.Order("frequency desc, variant_char asc").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&variants)

	c.JSON(http.StatusOK, gin.H{
		"items":     variants,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func UpdateVariant(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var variant model.VariantChar
	if err := model.DB.First(&variant, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "variant not found"})
		return
	}

	var req struct {
		StandardChar string `json:"standard_char"`
		Source       string `json:"source"`
		Verified     *bool  `json:"verified"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.StandardChar != "" {
		variant.StandardChar = req.StandardChar
	}
	if req.Source != "" {
		variant.Source = req.Source
	}
	if req.Verified != nil {
		variant.Verified = *req.Verified
	}

	model.DB.Save(&variant)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "update_variant", "variant_char", id, req)

	c.JSON(http.StatusOK, variant)
}

func DeleteVariant(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	model.DB.Delete(&model.VariantChar{}, id)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "delete_variant", "variant_char", id, nil)

	c.JSON(http.StatusOK, gin.H{"message": "variant deleted"})
}

func CreateCustomVariant(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var req VariantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var count int64
	model.DB.Model(&model.CustomVariant{}).Where("project_id = ? AND variant_char = ?",
		projectID, req.VariantChar).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "custom variant already exists for this project"})
		return
	}

	userID := c.GetUint64("user_id")
	variant := model.CustomVariant{
		ProjectID:    projectID,
		VariantChar:  req.VariantChar,
		StandardChar: req.StandardChar,
		CreatedBy:    userID,
	}
	model.DB.Create(&variant)

	util.AuditLog(c, userID, "create_custom_variant", "custom_variant", variant.ID, req)

	c.JSON(http.StatusCreated, variant)
}

func ListCustomVariants(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var variants []model.CustomVariant
	model.DB.Where("project_id = ?", projectID).Order("created_at desc").Find(&variants)

	c.JSON(http.StatusOK, variants)
}

func DeleteCustomVariant(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	id, err := strconv.ParseUint(c.Param("variant_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	model.DB.Where("id = ? AND project_id = ?", id, projectID).Delete(&model.CustomVariant{})

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "delete_custom_variant", "custom_variant", id, nil)

	c.JSON(http.StatusOK, gin.H{"message": "custom variant deleted"})
}

func DetectVariants(c *gin.Context) {
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

	variantMap := LoadProjectVariantMap(page.ProjectID)
	variants := diff.FindVariantChars(page.CorrectedText, variantMap)

	for _, v := range variants {
		vc := v["variant_char"].(string)
		model.DB.Model(&model.VariantChar{}).
			Where("variant_char = ?", vc).
			UpdateColumn("frequency", gorm.Expr("frequency + 1"))
	}

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "detect_variants", "page", pageID, map[string]interface{}{
		"count": len(variants),
	})

	c.JSON(http.StatusOK, gin.H{
		"variants": variants,
		"count":    len(variants),
	})
}

func CompareVersions(c *gin.Context) {
	var req CompareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pageA, pageB model.Page
	if err := model.DB.First(&pageA, req.VersionAPageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version A page not found"})
		return
	}
	if err := model.DB.First(&pageB, req.VersionBPageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version B page not found"})
		return
	}

	if pageA.ProjectID != pageB.ProjectID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pages must be from the same project"})
		return
	}

	labelA := req.VersionALabel
	if labelA == "" {
		labelA = pageA.VersionLabel
		if labelA == "" {
			labelA = pageA.Version
		}
	}
	labelB := req.VersionBLabel
	if labelB == "" {
		labelB = pageB.VersionLabel
		if labelB == "" {
			labelB = pageB.Version
		}
	}

	diffResult := diff.CompareVersions(pageA.CorrectedText, pageB.CorrectedText, [2]string{labelA, labelB})
	emendations := diff.GenerateEmendation(diffResult, [2]string{labelA, labelB})

	diffJSON, _ := json.Marshal(diffResult)
	emendationText := ""
	for _, e := range emendations {
		emendationText += e["note"].(string) + "。\n"
	}

	userID := c.GetUint64("user_id")
	task := model.VersionCompareTask{
		ProjectID:      pageA.ProjectID,
		VersionAPageID: req.VersionAPageID,
		VersionBPageID: req.VersionBPageID,
		VersionALabel:  labelA,
		VersionBLabel:  labelB,
		DiffResultJSON: string(diffJSON),
		EmendationText: emendationText,
		CreatedBy:      userID,
	}
	model.DB.Create(&task)

	util.AuditLog(c, userID, "compare_versions", "version_compare_task", task.ID, req)

	c.JSON(http.StatusOK, gin.H{
		"task":        task,
		"diff_result": diffResult,
		"emendations": emendations,
		"version_a":   pageA,
		"version_b":   pageB,
	})
}

func GetCompareTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("task_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var task model.VersionCompareTask
	if err := model.DB.First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	var diffResult diff.DiffResult
	json.Unmarshal([]byte(task.DiffResultJSON), &diffResult)

	c.JSON(http.StatusOK, gin.H{
		"task":        task,
		"diff_result": diffResult,
	})
}

func ListCompareTasks(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var tasks []model.VersionCompareTask
	model.DB.Where("project_id = ?", projectID).Order("created_at desc").Find(&tasks)

	c.JSON(http.StatusOK, tasks)
}

func GetVariantStats(c *gin.Context) {
	projectID, _ := strconv.ParseUint(c.Query("project_id"), 10, 64)

	var variants []struct {
		VariantChar  string `json:"variant_char"`
		StandardChar string `json:"standard_char"`
		Frequency    int    `json:"frequency"`
		Source       string `json:"source"`
	}

	query := model.DB.Model(&model.VariantChar{}).
		Select("variant_char, standard_char, frequency, source").
		Where("verified = ? AND frequency > 0", true).
		Order("frequency desc").
		Limit(100)

	if projectID > 0 {
		query = query.Joins("LEFT JOIN custom_variants ON custom_variants.variant_char = variant_chars.variant_char").
			Where("custom_variants.project_id = ? OR variant_chars.verified = ?", projectID, true)
	}

	query.Scan(&variants)

	c.JSON(http.StatusOK, gin.H{
		"items": variants,
		"total": len(variants),
	})
}

func BatchImportVariants(c *gin.Context) {
	var req struct {
		Variants []VariantRequest `json:"variants" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	imported := 0
	skipped := 0

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, v := range req.Variants {
			var count int64
			tx.Model(&model.VariantChar{}).Where("variant_char = ?", v.VariantChar).Count(&count)
			if count > 0 {
				skipped++
				continue
			}

			variant := model.VariantChar{
				VariantChar:  v.VariantChar,
				StandardChar: v.StandardChar,
				Source:       v.Source,
				Verified:     true,
			}
			if err := tx.Create(&variant).Error; err != nil {
				return err
			}
			imported++
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import failed: " + err.Error()})
		return
	}

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "batch_import_variants", "variant_char", 0, map[string]interface{}{
		"imported": imported,
		"skipped":  skipped,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":  "import completed",
		"imported": imported,
		"skipped":  skipped,
	})
}
