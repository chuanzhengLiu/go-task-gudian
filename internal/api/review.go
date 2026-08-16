package api

import (
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"
)

type ReviewRequest struct {
	Status  model.ReviewStatus `json:"status" binding:"required"`
	Comment string              `json:"comment"`
}

func ListReviewTasks(c *gin.Context) {
	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")
	userInstID, _ := c.Get("institution_id")

	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	query := model.DB.Table("review_rounds").
		Joins("JOIN pages ON pages.id = review_rounds.page_id").
		Joins("JOIN projects ON projects.id = pages.project_id")

	if userRole != string(model.RoleAdmin) {
		if userRole == string(model.RoleProofreader2) {
			if userInstID != nil {
				query = query.Where("projects.institution_id = ?", *userInstID.(*uint64))
			}
		} else if userRole == string(model.RoleInstAdmin) {
			if userInstID != nil {
				query = query.Where("projects.institution_id = ?", *userInstID.(*uint64))
			}
		} else {
			query = query.Where("pages.assigned_to = ?", userID)
		}
	}

	if status != "" {
		query = query.Where("review_rounds.status = ?", status)
	}

	var total int64
	query.Count(&total)

	var results []map[string]interface{}
	query.Select("review_rounds.*, pages.page_number, pages.status as page_status, projects.title as project_title").
		Order("review_rounds.created_at desc").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&results)

	c.JSON(http.StatusOK, gin.H{
		"items":     results,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func SubmitReview(c *gin.Context) {
	pageID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page id"})
		return
	}

	userID := c.GetUint64("user_id")
	userRole := c.GetString("user_role")

	var page model.Page
	if err := model.DB.First(&page, pageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) && userRole != string(model.RoleProofreader2) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var project model.Project
	model.DB.First(&project, page.ProjectID)

	var currentRound model.ReviewRound
	model.DB.Where("page_id = ? AND status = ?", pageID, model.ReviewStatusPending).
		Order("round_num desc").First(&currentRound)

	if currentRound.ID == 0 {
		var maxRound int
		model.DB.Model(&model.ReviewRound{}).Where("page_id = ?", pageID).
			Select("COALESCE(MAX(round_num), 0)").Scan(&maxRound)
		currentRound = model.ReviewRound{
			PageID:     pageID,
			ReviewerID: userID,
			RoundNum:   maxRound + 1,
		}
	} else {
		currentRound.ReviewerID = userID
	}

	currentRound.Status = req.Status
	currentRound.Comment = req.Comment
	model.DB.Save(&currentRound)

	var approvedCount int64
	model.DB.Model(&model.ReviewRound{}).
		Where("page_id = ? AND status = ?", pageID, model.ReviewStatusApproved).
		Count(&approvedCount)

	if req.Status == model.ReviewStatusApproved {
		if int(approvedCount) >= project.ReviewRequired {
			page.Status = model.PageStatusCompleted
		} else {
			nextRound := model.ReviewRound{
				PageID:     pageID,
				ReviewerID: 0,
				RoundNum:   currentRound.RoundNum + 1,
				Status:     model.ReviewStatusPending,
			}
			model.DB.Create(&nextRound)
		}
	} else if req.Status == model.ReviewStatusRejected {
		page.Status = model.PageStatusRejected
	}

	model.DB.Save(&page)

	util.AuditLog(c, userID, "submit_review", "page", pageID, req)

	c.JSON(http.StatusOK, gin.H{
		"message":       "review submitted",
		"review_round":  currentRound,
		"page_status":   page.Status,
		"approved_count": approvedCount,
		"required":      project.ReviewRequired,
	})
}

func UpdateReview(c *gin.Context) {
	reviewID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userID := c.GetUint64("user_id")

	var review model.ReviewRound
	if err := model.DB.First(&review, reviewID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "review not found"})
		return
	}

	if review.ReviewerID != userID {
		userRole := c.GetString("user_role")
		if userRole != string(model.RoleAdmin) && userRole != string(model.RoleInstAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	var req ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	review.Status = req.Status
	review.Comment = req.Comment
	model.DB.Save(&review)

	util.AuditLog(c, userID, "update_review", "review_round", reviewID, req)

	c.JSON(http.StatusOK, review)
}

func GetPageReviews(c *gin.Context) {
	pageID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page id"})
		return
	}

	var reviews []model.ReviewRound
	model.DB.Where("page_id = ?", pageID).
		Preload("Reviewer").
		Order("round_num desc, created_at desc").
		Find(&reviews)

	c.JSON(http.StatusOK, reviews)
}

func ExportText(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var project model.Project
	if err := model.DB.First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	var pages []model.Page
	model.DB.Where("project_id = ? AND status = ?", projectID, model.PageStatusCompleted).
		Order("page_number asc").Find(&pages)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%s\n", project.Title))
	if project.Author != "" {
		buf.WriteString(fmt.Sprintf("作者：%s\n", project.Author))
	}
	if project.VersionInfo != "" {
		buf.WriteString(fmt.Sprintf("版本：%s\n", project.VersionInfo))
	}
	buf.WriteString(fmt.Sprintf("\n%s\n\n", string(bytes.Repeat([]byte("="), 50))))

	footnoteCount := 1
	for _, page := range pages {
		buf.WriteString(fmt.Sprintf("--- 第 %d 页 ---\n\n", page.PageNumber))
		buf.WriteString(page.CorrectedText)
		buf.WriteString("\n\n")

		var corrections []model.Correction
		model.DB.Where("page_id = ?", page.ID).Order("start_position asc").Find(&corrections)

		for _, corr := range corrections {
			if corr.Note != "" || corr.Emendation != "" {
				emendation := corr.Emendation
				if emendation == "" {
					emendation = corr.Note
				}
				typeName := getCorrectionTypeName(corr.Type)
				buf.WriteString(fmt.Sprintf("[%d] 第%d页，%s：原「%s」→ 今「%s」。%s\n",
					footnoteCount, page.PageNumber, typeName,
					corr.OriginalText, corr.CorrectedText, emendation))
				footnoteCount++
			}
		}
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.txt\"", project.Title))
	c.String(http.StatusOK, buf.String())
}

func getCorrectionTypeName(t model.CorrectionType) string {
	switch t {
	case model.CorrectionTypeWrong:
		return "讹字"
	case model.CorrectionTypeMissing:
		return "脱字"
	case model.CorrectionTypeExtra:
		return "衍文"
	case model.CorrectionTypeReversed:
		return "倒文"
	case model.CorrectionTypeVariant:
		return "异体字"
	default:
		return "修改"
	}
}

type TEIApp struct {
	XMLName xml.Name `xml:"app"`
	From    string   `xml:"from,attr,omitempty"`
	To      string   `xml:"to,attr,omitempty"`
	Lem     TEILem   `xml:"lem"`
	Rdg     TEIRdg   `xml:"rdg"`
}

type TEILem struct {
	XMLName xml.Name `xml:"lem"`
	Source  string   `xml:"source,attr,omitempty"`
	Text    string   `xml:",chardata"`
}

type TEIRdg struct {
	XMLName xml.Name `xml:"rdg"`
	Source  string   `xml:"source,attr,omitempty"`
	Text    string   `xml:",chardata"`
}

func ExportTEI(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var project model.Project
	if err := model.DB.First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	var pages []model.Page
	model.DB.Where("project_id = ?", projectID).Order("page_number asc").Find(&pages)

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<?xml-model href="http://www.tei-c.org/release/xml/tei/custom/schema/relaxng/tei_all.rng" type="application/xml" schematypens="http://relaxng.org/ns/structure/1.0"?>
<TEI xmlns="http://www.tei-c.org/ns/1.0">
  <teiHeader>
    <fileDesc>
      <titleStmt>
        <title>` + xmlEscape(project.Title) + `</title>`)
	if project.Author != "" {
		buf.WriteString(`
        <author>` + xmlEscape(project.Author) + `</author>`)
	}
	buf.WriteString(`
      </titleStmt>
      <publicationStmt>
        <publisher>古籍数字化校对协作平台</publisher>
        <date when="` + time.Now().Format("2006-01-02") + `"/>
      </publicationStmt>
      <sourceDesc>
        <p>` + xmlEscape(project.VersionInfo) + `</p>
      </sourceDesc>
    </fileDesc>
  </teiHeader>
  <text>
    <body>
`)

	for _, page := range pages {
		buf.WriteString(fmt.Sprintf(`      <pb n="%d"/>`+"\n", page.PageNumber))

		text := page.CorrectedText
		var corrections []model.Correction
		model.DB.Where("page_id = ?", page.ID).Order("start_position desc").Find(&corrections)

		sort.Slice(corrections, func(i, j int) bool {
			return corrections[i].StartPosition > corrections[j].StartPosition
		})

		for _, corr := range corrections {
			app := TEIApp{
				From: strconv.Itoa(corr.StartPosition),
				To:   strconv.Itoa(corr.EndPosition),
				Lem: TEILem{
					Source: "orig",
					Text:   corr.OriginalText,
				},
				Rdg: TEIRdg{
					Source: "corr",
					Text:   corr.CorrectedText,
				},
			}
			appXML, _ := xml.MarshalIndent(app, "        ", "  ")
			appStr := string(appXML)
			appStr = appStr + "\n"

			text = text[:corr.StartPosition] + appStr + text[corr.EndPosition:]
		}

		buf.WriteString(fmt.Sprintf(`      <p>%s</p>`+"\n", xmlEscape(text)))
	}

	buf.WriteString(`    </body>
  </text>
</TEI>`)

	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.xml\"", project.Title))
	c.String(http.StatusOK, buf.String())
}

func xmlEscape(s string) string {
	buf := new(bytes.Buffer)
	xml.Escape(buf, []byte(s))
	return buf.String()
}

func ExportExcel(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var project model.Project
	if err := model.DB.First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	summarySheet := "项目概览"
	f.SetSheetName("Sheet1", summarySheet)

	f.SetCellValue(summarySheet, "A1", "项目名称")
	f.SetCellValue(summarySheet, "B1", project.Title)
	f.SetCellValue(summarySheet, "A2", "作者")
	f.SetCellValue(summarySheet, "B2", project.Author)
	f.SetCellValue(summarySheet, "A3", "版本信息")
	f.SetCellValue(summarySheet, "B3", project.VersionInfo)
	f.SetCellValue(summarySheet, "A4", "总页数")
	f.SetCellValue(summarySheet, "B4", project.PageCount)
	f.SetCellValue(summarySheet, "A5", "导出时间")
	f.SetCellValue(summarySheet, "B5", time.Now().Format("2006-01-02 15:04:05"))

	pagesSheet := "逐页统计"
	f.NewSheet(pagesSheet)

	headers := []string{"页码", "状态", "校对员", "校对标记数", "异体字数", "完成时间"}
	for i, h := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(pagesSheet, cell, h)
	}

	var pages []model.Page
	model.DB.Where("project_id = ?", projectID).Order("page_number asc").Find(&pages)

	for row, page := range pages {
		rowNum := row + 2
		f.SetCellValue(pagesSheet, fmt.Sprintf("A%d", rowNum), page.PageNumber)
		f.SetCellValue(pagesSheet, fmt.Sprintf("B%d", rowNum), getStatusName(page.Status))

		var userName string
		if page.AssignedTo != nil {
			var user model.User
			model.DB.Select("name").First(&user, *page.AssignedTo)
			userName = user.Name
		}
		f.SetCellValue(pagesSheet, fmt.Sprintf("C%d", rowNum), userName)

		var corrCount, variantCount int64
		model.DB.Model(&model.Correction{}).Where("page_id = ?", page.ID).Count(&corrCount)
		model.DB.Model(&model.Correction{}).Where("page_id = ? AND type = ?", page.ID, model.CorrectionTypeVariant).Count(&variantCount)
		f.SetCellValue(pagesSheet, fmt.Sprintf("D%d", rowNum), corrCount)
		f.SetCellValue(pagesSheet, fmt.Sprintf("E%d", rowNum), variantCount)
		f.SetCellValue(pagesSheet, fmt.Sprintf("F%d", rowNum), page.UpdatedAt.Format("2006-01-02"))
	}

	correctionsSheet := "校对标记明细"
	f.NewSheet(correctionsSheet)

	corrHeaders := []string{"页码", "类型", "原文", "改后", "位置", "校勘记", "创建人", "创建时间"}
	for i, h := range corrHeaders {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(correctionsSheet, cell, h)
	}

	var allCorrections []model.Correction
	model.DB.Joins("JOIN pages ON pages.id = corrections.page_id").
		Where("pages.project_id = ?", projectID).
		Order("pages.page_number, corrections.start_position").
		Find(&allCorrections)

	for row, corr := range allCorrections {
		rowNum := row + 2
		var page model.Page
		model.DB.Select("page_number").First(&page, corr.PageID)

		f.SetCellValue(correctionsSheet, fmt.Sprintf("A%d", rowNum), page.PageNumber)
		f.SetCellValue(correctionsSheet, fmt.Sprintf("B%d", rowNum), getCorrectionTypeName(corr.Type))
		f.SetCellValue(correctionsSheet, fmt.Sprintf("C%d", rowNum), corr.OriginalText)
		f.SetCellValue(correctionsSheet, fmt.Sprintf("D%d", rowNum), corr.CorrectedText)
		f.SetCellValue(correctionsSheet, fmt.Sprintf("E%d", rowNum), fmt.Sprintf("%d-%d", corr.StartPosition, corr.EndPosition))
		f.SetCellValue(correctionsSheet, fmt.Sprintf("F%d", rowNum), corr.Emendation)

		var user model.User
		model.DB.Select("name").First(&user, corr.CreatedBy)
		f.SetCellValue(correctionsSheet, fmt.Sprintf("G%d", rowNum), user.Name)
		f.SetCellValue(correctionsSheet, fmt.Sprintf("H%d", rowNum), corr.CreatedAt.Format("2006-01-02"))
	}

	buf, _ := f.WriteToBuffer()

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.xlsx\"", project.Title))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

func ExportPDF(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var project model.Project
	if err := model.DB.First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	var pages []model.Page
	model.DB.Where("project_id = ?", projectID).Order("page_number asc").Find(&pages)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)

	pdf.AddFont("NotoSansSC", "", "/usr/share/fonts/NotoSansSC-Regular.json")

	pdf.AddPage()
	pdf.SetFont("NotoSansSC", "", 18)
	pdf.CellFormat(0, 20, project.Title, "", 1, "C", false, 0, "")
	pdf.SetFont("NotoSansSC", "", 12)
	if project.Author != "" {
		pdf.CellFormat(0, 10, fmt.Sprintf("作者：%s", project.Author), "", 1, "C", false, 0, "")
	}
	if project.VersionInfo != "" {
		pdf.CellFormat(0, 10, fmt.Sprintf("版本：%s", project.VersionInfo), "", 1, "C", false, 0, "")
	}
	pdf.CellFormat(0, 10, fmt.Sprintf("导出时间：%s", time.Now().Format("2006年01月02日 15:04:05")), "", 1, "C", false, 0, "")
	pdf.Ln(10)

	for _, page := range pages {
		pdf.AddPage()

		pdf.SetFont("NotoSansSC", "B", 14)
		pdf.CellFormat(0, 12, fmt.Sprintf("第 %d 页", page.PageNumber), "", 1, "L", false, 0, "")

		uploadDir := os.Getenv("UPLOAD_DIR")
		if uploadDir == "" {
			uploadDir = "./uploads"
		}
		imagePath := filepath.Join(uploadDir, page.ImagePath)
		if _, err := os.Stat(imagePath); err == nil {
			imgWidth := 170.0
			imgHeight := 120.0
			pdf.ImageOptions(imagePath, 20, pdf.GetY(), imgWidth, imgHeight, false, gofpdf.ImageOptions{ImageType: "", ReadDpi: true}, 0, "")
			pdf.Ln(imgHeight + 8)
		}

		pdf.SetFont("NotoSansSC", "B", 12)
		pdf.CellFormat(0, 10, "校对文本：", "", 1, "L", false, 0, "")
		pdf.SetFont("NotoSansSC", "", 11)
		pdf.SetLeftMargin(20)
		pdf.SetRightMargin(20)
		pdf.MultiCell(0, 7, page.CorrectedText, "", "L", false)
		pdf.Ln(5)

		var corrections []model.Correction
		model.DB.Where("page_id = ?", page.ID).Order("start_position asc").Find(&corrections)

		if len(corrections) > 0 {
			pdf.SetFont("NotoSansSC", "B", 12)
			pdf.CellFormat(0, 10, "校勘记：", "", 1, "L", false, 0, "")
			pdf.SetFont("NotoSansSC", "", 10)

			for i, corr := range corrections {
				pdf.SetX(25)
				pdf.CellFormat(8, 6, fmt.Sprintf("[%d]", i+1), "", 0, "L", false, 0, "")
				pdf.CellFormat(25, 6, getCorrectionTypeName(corr.Type), "", 0, "L", false, 0, "")
				pdf.CellFormat(30, 6, fmt.Sprintf("原：%s", corr.OriginalText), "", 0, "L", false, 0, "")
				pdf.CellFormat(30, 6, fmt.Sprintf("改：%s", corr.CorrectedText), "", 0, "L", false, 0, "")
				pdf.Ln(6)
				if corr.Emendation != "" {
					pdf.SetX(33)
					pdf.MultiCell(140, 5, fmt.Sprintf("说明：%s", corr.Emendation), "", "L", false)
				}
				pdf.Ln(2)
			}
		}

		pdf.SetLeftMargin(10)
		pdf.SetRightMargin(10)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate PDF"})
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.pdf\"", project.Title))
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}

func getStatusName(s model.PageStatus) string {
	switch s {
	case model.PageStatusUnassigned:
		return "待分配"
	case model.PageStatusAssigned:
		return "已分配"
	case model.PageStatusProofing:
		return "校对中"
	case model.PageStatusReviewing:
		return "审校中"
	case model.PageStatusCompleted:
		return "已完成"
	case model.PageStatusRejected:
		return "已驳回"
	default:
		return string(s)
	}
}

func ExportStats(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var project model.Project
	if err := model.DB.First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	var stats struct {
		TotalPages      int64            `json:"total_pages"`
		StatusBreakdown map[string]int64 `json:"status_breakdown"`
		TotalCorrections int64           `json:"total_corrections"`
		CorrectionTypes map[string]int64 `json:"correction_types"`
		VariantCount    int64            `json:"variant_count"`
		TopVariants     []map[string]interface{} `json:"top_variants"`
		Contributors    []map[string]interface{} `json:"contributors"`
	}

	model.DB.Model(&model.Page{}).Where("project_id = ?", projectID).Count(&stats.TotalPages)

	statuses := []model.PageStatus{
		model.PageStatusUnassigned, model.PageStatusAssigned,
		model.PageStatusProofing, model.PageStatusReviewing,
		model.PageStatusCompleted, model.PageStatusRejected,
	}
	stats.StatusBreakdown = make(map[string]int64)
	for _, s := range statuses {
		var count int64
		model.DB.Model(&model.Page{}).Where("project_id = ? AND status = ?", projectID, s).Count(&count)
		stats.StatusBreakdown[string(s)] = count
	}

	model.DB.Table("corrections").
		Joins("JOIN pages ON pages.id = corrections.page_id").
		Where("pages.project_id = ?", projectID).
		Count(&stats.TotalCorrections)

	corrTypes := []model.CorrectionType{
		model.CorrectionTypeWrong, model.CorrectionTypeMissing,
		model.CorrectionTypeExtra, model.CorrectionTypeReversed,
		model.CorrectionTypeVariant,
	}
	stats.CorrectionTypes = make(map[string]int64)
	for _, t := range corrTypes {
		var count int64
		model.DB.Table("corrections").
			Joins("JOIN pages ON pages.id = corrections.page_id").
			Where("pages.project_id = ? AND corrections.type = ?", projectID, t).
			Count(&count)
		stats.CorrectionTypes[string(t)] = count
	}

	model.DB.Table("corrections").
		Joins("JOIN pages ON pages.id = corrections.page_id").
		Where("pages.project_id = ? AND corrections.type = ?", projectID, model.CorrectionTypeVariant).
		Count(&stats.VariantCount)

	variantRows, _ := model.DB.Table("corrections").
		Select("corrections.original_text as variant_char, corrections.corrected_text as standard_char, COUNT(*) as count").
		Joins("JOIN pages ON pages.id = corrections.page_id").
		Where("pages.project_id = ? AND corrections.type = ?", projectID, model.CorrectionTypeVariant).
		Group("corrections.original_text, corrections.corrected_text").
		Order("count desc").
		Limit(10).
		Rows()
	defer variantRows.Close()

	for variantRows.Next() {
		var vc, sc string
		var count int
		variantRows.Scan(&vc, &sc, &count)
		stats.TopVariants = append(stats.TopVariants, map[string]interface{}{
			"variant_char":  vc,
			"standard_char": sc,
			"count":         count,
		})
	}

	contribRows, _ := model.DB.Table("corrections").
		Select("corrections.created_by as user_id, COUNT(*) as count").
		Joins("JOIN pages ON pages.id = corrections.page_id").
		Where("pages.project_id = ?", projectID).
		Group("corrections.created_by").
		Order("count desc").
		Rows()
	defer contribRows.Close()

	for contribRows.Next() {
		var userID uint64
		var count int
		contribRows.Scan(&userID, &count)
		var user model.User
		model.DB.Select("name, email, role").First(&user, userID)
		stats.Contributors = append(stats.Contributors, map[string]interface{}{
			"user_id": userID,
			"name":    user.Name,
			"email":   user.Email,
			"role":    user.Role,
			"count":   count,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"project": project,
		"stats":   stats,
	})
}
