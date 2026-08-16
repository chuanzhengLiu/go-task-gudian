package api

import (
	"ancient-texts-backend/internal/model"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupReviewTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Institution{}, &model.User{}, &model.Project{}, &model.Page{}, &model.ReviewRound{}, &model.AuditLog{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	model.DB = db
	t.Cleanup(func() { model.DB = nil })
}

func seedReviewPage(t *testing.T, reviewRequired int) (model.Project, model.Page) {
	t.Helper()
	project := model.Project{
		InstitutionID:  1,
		Title:          "test project",
		CreatedBy:      1,
		ReviewRequired: reviewRequired,
	}
	if err := model.DB.Create(&project).Error; err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	page := model.Page{
		ProjectID:  project.ID,
		PageNumber: 1,
		Status:     model.PageStatusReviewing,
	}
	if err := model.DB.Create(&page).Error; err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	return project, page
}

func submitReview(t *testing.T, pageID uint64, reviewerID uint64, status model.ReviewStatus) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(ReviewRequest{Status: status})
	req := httptest.NewRequest(http.MethodPost, "/api/pages/"+strconv.FormatUint(pageID, 10)+"/review", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(pageID, 10)}}
	c.Set("user_id", reviewerID)
	c.Set("user_role", string(model.RoleProofreader2))

	SubmitReview(c)
	return w
}

func countRounds(t *testing.T, pageID uint64, status model.ReviewStatus) int64 {
	t.Helper()
	var count int64
	model.DB.Model(&model.ReviewRound{}).Where("page_id = ? AND status = ?", pageID, status).Count(&count)
	return count
}

func reloadPage(t *testing.T, pageID uint64) model.Page {
	t.Helper()
	var page model.Page
	if err := model.DB.First(&page, pageID).Error; err != nil {
		t.Fatalf("failed to reload page: %v", err)
	}
	return page
}

func TestSubmitReviewCompletesPageWhenRequiredApprovalsReached(t *testing.T) {
	setupReviewTestDB(t)
	_, page := seedReviewPage(t, 2)

	if w := submitReview(t, page.ID, 101, model.ReviewStatusApproved); w.Code != http.StatusOK {
		t.Fatalf("first review: got status %d, body %s", w.Code, w.Body.String())
	}
	if got := reloadPage(t, page.ID).Status; got != model.PageStatusReviewing {
		t.Fatalf("after 1 approval: page status = %q, want %q", got, model.PageStatusReviewing)
	}
	if got := countRounds(t, page.ID, model.ReviewStatusPending); got != 1 {
		t.Fatalf("after 1 approval: pending rounds = %d, want 1", got)
	}

	if w := submitReview(t, page.ID, 102, model.ReviewStatusApproved); w.Code != http.StatusOK {
		t.Fatalf("second review: got status %d, body %s", w.Code, w.Body.String())
	}
	if got := reloadPage(t, page.ID).Status; got != model.PageStatusCompleted {
		t.Fatalf("after 2 approvals: page status = %q, want %q", got, model.PageStatusCompleted)
	}
	if got := countRounds(t, page.ID, model.ReviewStatusApproved); got != 2 {
		t.Fatalf("after 2 approvals: approved rounds = %d, want 2", got)
	}
	if got := countRounds(t, page.ID, model.ReviewStatusPending); got != 0 {
		t.Fatalf("after 2 approvals: pending rounds = %d, want 0 (no extra round opened)", got)
	}
}

func TestSubmitReviewCompletesWithSingleRequiredApproval(t *testing.T) {
	setupReviewTestDB(t)
	_, page := seedReviewPage(t, 1)

	if w := submitReview(t, page.ID, 101, model.ReviewStatusApproved); w.Code != http.StatusOK {
		t.Fatalf("review: got status %d, body %s", w.Code, w.Body.String())
	}
	if got := reloadPage(t, page.ID).Status; got != model.PageStatusCompleted {
		t.Fatalf("page status = %q, want %q", got, model.PageStatusCompleted)
	}
	if got := countRounds(t, page.ID, model.ReviewStatusPending); got != 0 {
		t.Fatalf("pending rounds = %d, want 0", got)
	}
}

func TestSubmitReviewRejectsPage(t *testing.T) {
	setupReviewTestDB(t)
	_, page := seedReviewPage(t, 2)

	if w := submitReview(t, page.ID, 101, model.ReviewStatusRejected); w.Code != http.StatusOK {
		t.Fatalf("review: got status %d, body %s", w.Code, w.Body.String())
	}
	if got := reloadPage(t, page.ID).Status; got != model.PageStatusRejected {
		t.Fatalf("page status = %q, want %q", got, model.PageStatusRejected)
	}
}
