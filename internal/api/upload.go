package api

import (
	"ancient-texts-backend/internal/config"
	"ancient-texts-backend/internal/image"
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func UploadImage(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
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

	if userRole == string(model.RoleInstAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to get file: " + err.Error()})
		return
	}
	defer file.Close()

	tmpFile, err := os.CreateTemp("", "upload-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temp file"})
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	contentType, err := image.ValidateImageFile(tmpFile.Name())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pageNumber, _ := strconv.Atoi(c.DefaultPostForm("page_number", "0"))
	version := c.DefaultPostForm("version", "main")
	versionLabel := c.PostForm("version_label")

	relativePath, _, err := image.SaveUploadedFile(tmpFile.Name(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	tileRelativePath, err := image.GenerateTiles(relativePath, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate tiles: " + err.Error()})
		return
	}

	ocrText := c.PostForm("ocr_text")

	if pageNumber > 0 {
		page := model.Page{
			ProjectID:     projectID,
			PageNumber:    pageNumber,
			ImagePath:     relativePath,
			TilePath:      tileRelativePath,
			OCRText:       ocrText,
			CorrectedText: ocrText,
			Status:        model.PageStatusUnassigned,
			Version:       version,
			VersionLabel:  versionLabel,
		}

		var existingPage model.Page
		result := model.DB.Where("project_id = ? AND page_number = ? AND version = ?",
			projectID, pageNumber, version).First(&existingPage)
		if result.Error == nil {
			existingPage.ImagePath = relativePath
			existingPage.TilePath = tileRelativePath
			if ocrText != "" {
				existingPage.OCRText = ocrText
				existingPage.CorrectedText = ocrText
			}
			if versionLabel != "" {
				existingPage.VersionLabel = versionLabel
			}
			model.DB.Save(&existingPage)
		} else {
			model.DB.Create(&page)
			project.PageCount++
			model.DB.Save(&project)
		}
	}

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "upload_image", "project", projectID, map[string]interface{}{
		"file_name":  header.Filename,
		"page":       pageNumber,
		"image_path": relativePath,
	})

	c.JSON(http.StatusCreated, gin.H{
		"message":    "file uploaded successfully",
		"image_path": relativePath,
		"tile_path":  tileRelativePath,
		"page_number": pageNumber,
		"content_type": contentType,
	})
}

func UploadOCR(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
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

	if userRole == string(model.RoleInstAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to get file"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	startPage, _ := strconv.Atoi(c.DefaultPostForm("start_page", "1"))

	lines := strings.Split(string(content), "\n")

	pageNum := startPage
	pagesCreated := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var page model.Page
		result := model.DB.Where("project_id = ? AND page_number = ?", projectID, pageNum).First(&page)
		linePositions := analyzeLinePositions(line, 1000, 1400)
		linePositionsJSON, _ := json.Marshal(linePositions)

		if result.Error == nil {
			page.OCRText = line
			page.CorrectedText = line
			page.LinePositions = string(linePositionsJSON)
			model.DB.Save(&page)
		} else {
			page = model.Page{
				ProjectID:     projectID,
				PageNumber:    pageNum,
				OCRText:       line,
				CorrectedText: line,
				LinePositions: string(linePositionsJSON),
				Status:        model.PageStatusUnassigned,
				Version:       "main",
			}
			model.DB.Create(&page)
			pagesCreated++
		}
		pageNum++
	}

	project.PageCount = max(project.PageCount, pagesCreated)
	model.DB.Save(&project)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "upload_ocr", "project", projectID, map[string]interface{}{
		"pages_processed": pageNum - startPage,
		"start_page": startPage,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":         "OCR file processed successfully",
		"pages_updated": pageNum - startPage,
		"pages_created": pagesCreated,
	})
}

func GetImage(c *gin.Context) {
	path := c.Param("path")
	token := c.Query("token")

	if !util.ValidateImageToken(path, token, config.AppConfig.IMAGE_SIGN_SECRET) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}

	fullPath := filepath.Join(config.AppConfig.UPLOAD_DIR, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	c.File(fullPath)
}

func GetTile(c *gin.Context) {
	path := c.Param("path")
	token := c.Query("token")

	tokenPath := strings.TrimSuffix(path, ".dzi")
	tokenPath = strings.TrimSuffix(tokenPath, "_files")

	if !util.ValidateImageToken(tokenPath, token, config.AppConfig.IMAGE_SIGN_SECRET) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}

	fullPath := filepath.Join(config.AppConfig.TILE_DIR, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tile not found"})
		return
	}

	if strings.HasSuffix(path, ".dzi") {
		c.Header("Content-Type", "application/xml")
	} else if strings.HasSuffix(path, ".jpg") {
		c.Header("Content-Type", "image/jpeg")
	}

	c.File(fullPath)
}

func BatchUploadImages(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
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

	if userRole == string(model.RoleInstAdmin) {
		if userInstID == nil || *userInstID.(*uint64) != project.InstitutionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files uploaded"})
		return
	}

	startPage, _ := strconv.Atoi(c.DefaultPostForm("start_page", "1"))
	version := c.DefaultPostForm("version", "main")
	versionLabel := c.PostForm("version_label")

	results := make([]gin.H, 0)
	newPageCount := 0

	for i, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			results = append(results, gin.H{
				"file":  fileHeader.Filename,
				"error": err.Error(),
			})
			continue
		}

		tmpFile, err := os.CreateTemp("", "upload-*")
		if err != nil {
			file.Close()
			results = append(results, gin.H{
				"file":  fileHeader.Filename,
				"error": "failed to create temp file",
			})
			continue
		}

		_, err = io.Copy(tmpFile, file)
		file.Close()
		tmpFile.Close()

		if err != nil {
			os.Remove(tmpFile.Name())
			results = append(results, gin.H{
				"file":  fileHeader.Filename,
				"error": "failed to save file",
			})
			continue
		}

		_, err = image.ValidateImageFile(tmpFile.Name())
		if err != nil {
			os.Remove(tmpFile.Name())
			results = append(results, gin.H{
				"file":  fileHeader.Filename,
				"error": err.Error(),
			})
			continue
		}

		pageNumber := startPage + i

		relativePath, _, err := image.SaveUploadedFile(tmpFile.Name(), projectID)
		os.Remove(tmpFile.Name())

		if err != nil {
			results = append(results, gin.H{
				"file":  fileHeader.Filename,
				"error": "failed to save file",
			})
			continue
		}

		tileRelativePath, err := image.GenerateTiles(relativePath, projectID)
		if err != nil {
			results = append(results, gin.H{
				"file":  fileHeader.Filename,
				"error": "failed to generate tiles",
			})
			continue
		}

		var page model.Page
		result := model.DB.Where("project_id = ? AND page_number = ? AND version = ?",
			projectID, pageNumber, version).First(&page)
		if result.Error == nil {
			page.ImagePath = relativePath
			page.TilePath = tileRelativePath
			if versionLabel != "" {
				page.VersionLabel = versionLabel
			}
			model.DB.Save(&page)
		} else {
			page = model.Page{
				ProjectID:    projectID,
				PageNumber:   pageNumber,
				ImagePath:    relativePath,
				TilePath:     tileRelativePath,
				Status:       model.PageStatusUnassigned,
				Version:      version,
				VersionLabel: versionLabel,
			}
			model.DB.Create(&page)
			newPageCount++
		}

		results = append(results, gin.H{
			"file":       fileHeader.Filename,
			"success":    true,
			"page":       pageNumber,
			"image_path": relativePath,
			"tile_path":  tileRelativePath,
		})
	}

	project.PageCount += newPageCount
	model.DB.Save(&project)

	userID := c.GetUint64("user_id")
	util.AuditLog(c, userID, "batch_upload_images", "project", projectID, map[string]interface{}{
		"files_count":   len(files),
		"success_count": len(results) - newPageCount,
		"start_page":  startPage,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "batch upload completed",
		"total":   len(files),
		"results": results,
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func UploadEmendations(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	userID := c.GetUint64("user_id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))

	var imported int

	if ext == ".json" {
		type EmendationEntry struct {
			PageNumber   int    `json:"page_number"`
			Type         string `json:"type"`
			Position     int    `json:"position"`
			OriginalText string `json:"original_text"`
			CorrectedText string `json:"corrected_text"`
			Emendation   string `json:"emendation"`
			CreatedBy    *uint64 `json:"created_by"`
		}

		var entries []EmendationEntry
		if err := json.Unmarshal(content, &entries); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON format"})
			return
		}

		for _, entry := range entries {
			var page model.Page
			if err := model.DB.Where("project_id = ? AND page_number = ?", projectID, entry.PageNumber).First(&page).Error; err != nil {
				continue
			}

			corrType := model.CorrectionTypeWrong
			switch entry.Type {
			case "missing":
				corrType = model.CorrectionTypeMissing
			case "extra":
				corrType = model.CorrectionTypeExtra
			case "reversed":
				corrType = model.CorrectionTypeReversed
			case "variant":
				corrType = model.CorrectionTypeVariant
			}

			createdByVal := userID
			if entry.CreatedBy != nil {
				createdByVal = *entry.CreatedBy
			}

			correction := model.Correction{
				PageID:        page.ID,
				Type:          corrType,
				StartPosition: entry.Position,
				EndPosition:   entry.Position + utf8.RuneCountInString(entry.OriginalText),
				OriginalText:  entry.OriginalText,
				CorrectedText: entry.CorrectedText,
				Emendation:    entry.Emendation,
				CreatedBy:     createdByVal,
			}
			model.DB.Create(&correction)
			imported++
		}
	} else if ext == ".txt" {
		lines := strings.Split(string(content), "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "\t", 6)
			if len(parts) < 5 {
				parts = strings.SplitN(line, "|", 6)
			}
			if len(parts) < 5 {
				continue
			}

			pageNum, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
			corrType := strings.TrimSpace(parts[1])
			position, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
			original := strings.TrimSpace(parts[3])
			corrected := strings.TrimSpace(parts[4])
			emendation := ""
			if len(parts) >= 6 {
				emendation = strings.TrimSpace(parts[5])
			}

			var page model.Page
			if err := model.DB.Where("project_id = ? AND page_number = ?", projectID, pageNum).First(&page).Error; err != nil {
				continue
			}

			cType := model.CorrectionTypeWrong
			switch corrType {
			case "漏", "missing":
				cType = model.CorrectionTypeMissing
			case "衍", "extra":
				cType = model.CorrectionTypeExtra
			case "倒", "reversed":
				cType = model.CorrectionTypeReversed
			case "异", "variant":
				cType = model.CorrectionTypeVariant
			}

			correction := model.Correction{
				PageID:        page.ID,
				Type:          cType,
				StartPosition: position,
				EndPosition:   position + utf8.RuneCountInString(original),
				OriginalText:  original,
				CorrectedText: corrected,
				Emendation:    emendation,
				CreatedBy:     userID,
			}
			model.DB.Create(&correction)
			imported++
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file format, use .json or .txt"})
		return
	}

	util.AuditLog(c, userID, "emendations_import", "project", projectID, map[string]interface{}{
		"filename": header.Filename,
		"imported": imported,
	})

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("成功导入 %d 条校勘记", imported), "imported": imported})
}

type LinePosition struct {
	LineIndex  int     `json:"line_index"`
	Text       string  `json:"text"`
	StartChar  int     `json:"start_char"`
	EndChar    int     `json:"end_char"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Confidence float64 `json:"confidence"`
}

func analyzeLinePositions(text string, imageWidth, imageHeight int) []LinePosition {
	var positions []LinePosition

	if text == "" {
		return positions
	}

	runes := []rune(text)
	totalRunes := len(runes)

	if totalRunes == 0 {
		return positions
	}

	charsPerLine := 25
	margin := 80
	lineHeight := 40
	estimatedLines := (totalRunes + charsPerLine - 1) / charsPerLine
	totalTextHeight := estimatedLines * lineHeight
	startY := (imageHeight - totalTextHeight) / 2

	if startY < margin {
		startY = margin
	}

	for i := 0; i < estimatedLines; i++ {
		startChar := i * charsPerLine
		endChar := startChar + charsPerLine
		if endChar > totalRunes {
			endChar = totalRunes
		}

		lineText := string(runes[startChar:endChar])
		lineRuneCount := utf8.RuneCountInString(lineText)
		lineWidth := lineRuneCount * 28

		positions = append(positions, LinePosition{
			LineIndex:  i,
			Text:       lineText,
			StartChar:  startChar,
			EndChar:    endChar,
			X:          (imageWidth - lineWidth) / 2,
			Y:          startY + i*lineHeight,
			Width:      lineWidth,
			Height:     lineHeight - 4,
			Confidence: 0.85,
		})
	}

	return positions
}
