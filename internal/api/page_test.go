package api

import (
	"ancient-texts-backend/internal/model"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupPageTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Institution{}, &model.User{}, &model.Project{}, &model.Page{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	// Each test gets a clean shared-cache DB; ensure no rows leak between tests.
	db.Exec("DELETE FROM pages")
	db.Exec("DELETE FROM projects")
	model.DB = db
	t.Cleanup(func() { model.DB = nil })
}

func seedPage(t *testing.T) model.Page {
	t.Helper()
	project := model.Project{
		InstitutionID: 1,
		Title:         "test project",
		CreatedBy:     1,
	}
	if err := model.DB.Create(&project).Error; err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	page := model.Page{
		ProjectID:  project.ID,
		PageNumber: 1,
		Status:     model.PageStatusUnassigned,
	}
	if err := model.DB.Create(&page).Error; err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	return page
}

func callLockPage(t *testing.T, pageID uint64, userID uint64) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pages/"+strconv.FormatUint(pageID, 10)+"/lock", nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(pageID, 10)}}
	c.Set("user_id", userID)

	LockPage(c)
	return w
}

func reloadPage(t *testing.T, pageID uint64) model.Page {
	t.Helper()
	var page model.Page
	if err := model.DB.First(&page, pageID).Error; err != nil {
		t.Fatalf("failed to reload page: %v", err)
	}
	return page
}

// TestLockPageBlocksRecentLockByAnotherUser: a lock taken by someone else within
// the 5-minute window must be reported as occupied (the bug reported the reverse).
func TestLockPageBlocksRecentLockByAnotherUser(t *testing.T) {
	setupPageTestDB(t)
	page := seedPage(t)

	otherUser := uint64(101)
	recent := time.Now().Add(-30 * time.Second)
	page.LockBy = &otherUser
	page.LockAt = &recent
	if err := model.DB.Save(&page).Error; err != nil {
		t.Fatalf("failed to seed lock: %v", err)
	}

	w := callLockPage(t, page.ID, 202)
	if w.Code != http.StatusConflict {
		t.Fatalf("recent lock by another user: got status %d, want %d (body %s)", w.Code, http.StatusConflict, w.Body.String())
	}

	// The page must remain locked by the original holder.
	got := reloadPage(t, page.ID)
	if got.LockBy == nil || *got.LockBy != otherUser {
		t.Fatalf("page lock_by changed: got %v, want %d", got.LockBy, otherUser)
	}
}

// TestLockPageAllowsTakeoverAfterExpiry: once a lock is older than 5 minutes it is
// considered expired and anyone may take it over (the bug reported the reverse).
func TestLockPageAllowsTakeoverAfterExpiry(t *testing.T) {
	setupPageTestDB(t)
	page := seedPage(t)

	otherUser := uint64(101)
	expired := time.Now().Add(-6 * time.Minute)
	page.LockBy = &otherUser
	page.LockAt = &expired
	if err := model.DB.Save(&page).Error; err != nil {
		t.Fatalf("failed to seed lock: %v", err)
	}

	w := callLockPage(t, page.ID, 202)
	if w.Code != http.StatusOK {
		t.Fatalf("expired lock: got status %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}

	got := reloadPage(t, page.ID)
	if got.LockBy == nil || *got.LockBy != 202 {
		t.Fatalf("expired lock not taken over: lock_by = %v, want 202", got.LockBy)
	}
	if got.LockAt == nil || got.LockAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("expired lock not refreshed: lock_at = %v", got.LockAt)
	}
}

// TestLockPageAllowsRelockBySameUser: the current lock holder can refresh their
// own lock regardless of age.
func TestLockPageAllowsRelockBySameUser(t *testing.T) {
	setupPageTestDB(t)
	page := seedPage(t)

	holder := uint64(101)
	stale := time.Now().Add(-10 * time.Minute)
	page.LockBy = &holder
	page.LockAt = &stale
	if err := model.DB.Save(&page).Error; err != nil {
		t.Fatalf("failed to seed lock: %v", err)
	}

	w := callLockPage(t, page.ID, holder)
	if w.Code != http.StatusOK {
		t.Fatalf("same-user relock: got status %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}

	got := reloadPage(t, page.ID)
	if got.LockBy == nil || *got.LockBy != holder {
		t.Fatalf("relock changed holder: lock_by = %v, want %d", got.LockBy, holder)
	}
	if got.LockAt == nil || !got.LockAt.After(stale) {
		t.Fatalf("relock did not refresh lock_at: got %v, want after %v", got.LockAt, stale)
	}
}

// TestLockPageFreshPage: a page with no lock can be locked by anyone.
func TestLockPageFreshPage(t *testing.T) {
	setupPageTestDB(t)
	page := seedPage(t)

	w := callLockPage(t, page.ID, 303)
	if w.Code != http.StatusOK {
		t.Fatalf("fresh page lock: got status %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}

	got := reloadPage(t, page.ID)
	if got.LockBy == nil || *got.LockBy != 303 {
		t.Fatalf("fresh page not locked: lock_by = %v, want 303", got.LockBy)
	}
}
