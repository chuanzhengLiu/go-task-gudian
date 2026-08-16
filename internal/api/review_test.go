package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ancient-texts-backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTestDB swaps model.DB for an in-memory SQLite database and runs the
// migrations SubmitReview relies on. The original DB is restored on cleanup.
func setupTestDB(t *testing.T) func() {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Project{}, &model.Page{}, &model.ReviewRound{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := model.DB
	model.DB = db
	return func() { model.DB = orig }
}

// submitReview calls the SubmitReview handler as a proofreader2, recording an
// approval/rejection for the given page.
func submitReview(t *testing.T, pageID, userID uint64, status model.ReviewStatus) (int, map[string]interface{}) {
	t.Helper()
	body, _ := json.Marshal(ReviewRequest{Status: status, Comment: "ok"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uintToStr(pageID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", userID)
	c.Set("user_role", string(model.RoleProofreader2))

	SubmitReview(c)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func uintToStr(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TestSubmitReviewCompletesAtRequiredCount is the regression test for the
// off-by-one in the approval threshold. With ReviewRequired=2 the page must
// transition to completed exactly on the second approval, and no extra pending
// round may be spawned. The previous ">" comparison left the page stuck in
// "reviewing" and opened a new pending round on every approval.
func TestSubmitReviewCompletesAtRequiredCount(t *testing.T) {
	restore := setupTestDB(t)
	defer restore()

	project := model.Project{Title: "p", Status: model.ProjectStatusActive, ReviewRequired: 2, CreatedBy: 1}
	if err := model.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	page := model.Page{ProjectID: project.ID, PageNumber: 1, Status: model.PageStatusReviewing}
	if err := model.DB.Create(&page).Error; err != nil {
		t.Fatal(err)
	}

	// Seed a pending round 1 so the handler follows its normal "claim existing
	// pending round" path rather than synthesising one.
	model.DB.Create(&model.ReviewRound{PageID: page.ID, ReviewerID: 0, RoundNum: 1, Status: model.ReviewStatusPending})

	// First approval: one of two required -> still reviewing, next pending round created.
	if code, _ := submitReview(t, page.ID, 10, model.ReviewStatusApproved); code != http.StatusOK {
		t.Fatalf("first approval: want 200, got %d", code)
	}
	var page1 model.Page
	model.DB.First(&page1, page.ID)
	if page1.Status != model.PageStatusReviewing {
		t.Fatalf("after 1st approval: want reviewing, got %s", page1.Status)
	}

	// Second approval: reaches the required count -> completed, no extra round.
	if code, _ := submitReview(t, page.ID, 20, model.ReviewStatusApproved); code != http.StatusOK {
		t.Fatalf("second approval: want 200, got %d", code)
	}
	var page2 model.Page
	model.DB.First(&page2, page.ID)
	if page2.Status != model.PageStatusCompleted {
		t.Fatalf("after 2nd approval: want completed, got %s", page2.Status)
	}

	var pendingCount int64
	model.DB.Model(&model.ReviewRound{}).
		Where("page_id = ? AND status = ?", page.ID, model.ReviewStatusPending).
		Count(&pendingCount)
	if pendingCount != 0 {
		t.Fatalf("want 0 pending rounds after completion, got %d", pendingCount)
	}

	var approvedCount int64
	model.DB.Model(&model.ReviewRound{}).
		Where("page_id = ? AND status = ?", page.ID, model.ReviewStatusApproved).
		Count(&approvedCount)
	if approvedCount != 2 {
		t.Fatalf("want 2 approved rounds, got %d", approvedCount)
	}
}

// TestSubmitReviewRequiredOneCompletesOnFirstApproval guards the same threshold
// for ReviewRequired=1: a single approval must complete the page. Under the old
// ">" check this case also never completed.
func TestSubmitReviewRequiredOneCompletesOnFirstApproval(t *testing.T) {
	restore := setupTestDB(t)
	defer restore()

	project := model.Project{Title: "p1", Status: model.ProjectStatusActive, ReviewRequired: 1, CreatedBy: 1}
	model.DB.Create(&project)
	page := model.Page{ProjectID: project.ID, PageNumber: 1, Status: model.PageStatusReviewing}
	model.DB.Create(&page)

	if code, _ := submitReview(t, page.ID, 10, model.ReviewStatusApproved); code != http.StatusOK {
		t.Fatalf("approval: want 200, got %d", code)
	}
	var got model.Page
	model.DB.First(&got, page.ID)
	if got.Status != model.PageStatusCompleted {
		t.Fatalf("want completed, got %s", got.Status)
	}

	var pendingCount int64
	model.DB.Model(&model.ReviewRound{}).
		Where("page_id = ? AND status = ?", page.ID, model.ReviewStatusPending).
		Count(&pendingCount)
	if pendingCount != 0 {
		t.Fatalf("want 0 pending rounds, got %d", pendingCount)
	}
}

// TestSubmitReviewRejectionStillMarksRejected ensures the unrelated rejection
// path was not disturbed by the threshold fix.
func TestSubmitReviewRejectionStillMarksRejected(t *testing.T) {
	restore := setupTestDB(t)
	defer restore()

	project := model.Project{Title: "pr", Status: model.ProjectStatusActive, ReviewRequired: 2, CreatedBy: 1}
	model.DB.Create(&project)
	page := model.Page{ProjectID: project.ID, PageNumber: 1, Status: model.PageStatusReviewing}
	model.DB.Create(&page)
	model.DB.Create(&model.ReviewRound{PageID: page.ID, ReviewerID: 0, RoundNum: 1, Status: model.ReviewStatusPending})

	if code, _ := submitReview(t, page.ID, 10, model.ReviewStatusRejected); code != http.StatusOK {
		t.Fatalf("rejection: want 200, got %d", code)
	}
	var got model.Page
	model.DB.First(&got, page.ID)
	if got.Status != model.PageStatusRejected {
		t.Fatalf("want rejected, got %s", got.Status)
	}
}
