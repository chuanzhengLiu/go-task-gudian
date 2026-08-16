package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"ancient-texts-backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB points the package-level model.DB at a per-test sqlite file
// database and migrates the VariantChar table, returning a teardown func.
// A file (rather than shared in-memory) DB avoids cross-test leakage.
func setupTestDB(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.VariantChar{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	prev := model.DB
	model.DB = db
	return func() {
		model.DB = prev
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
		_ = os.RemoveAll(dir)
	}
}

// seedVariants inserts n variant rows whose frequency decreases with the
// insertion index (row 0 has the highest frequency). Because ListVariants
// orders by frequency desc then variant_char asc, the first page is the
// first n-by-pageSize inserted rows.
func seedVariants(t *testing.T, n int) {
	t.Helper()
	rows := make([]model.VariantChar, 0, n)
	for i := 0; i < n; i++ {
		// Frequency decreases so the natural (insertion) order matches the
		// ORDER BY frequency desc ordering used by ListVariants.
		rows = append(rows, model.VariantChar{
			VariantChar:  "v" + strconv.Itoa(i),
			StandardChar: "s" + strconv.Itoa(i),
			Frequency:    n - i,
		})
	}
	if err := model.DB.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func callListVariants(t *testing.T, page, pageSize int) map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/variants", ListVariants)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/variants?page="+strconv.Itoa(page)+"&page_size="+strconv.Itoa(pageSize), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body
}

func variantChars(body map[string]interface{}) []string {
	items := body["items"].([]interface{})
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.(map[string]interface{})["variant_char"].(string))
	}
	return out
}

// TestListVariantsPagination reproduces the reported bug: with 73 total rows
// and page_size=50, page 1 must return the first 50 rows (not 23), and
// page 2 must return the remaining 23 (not be empty).
func TestListVariantsPagination(t *testing.T) {
	defer setupTestDB(t)()
	seedVariants(t, 73)

	page1 := callListVariants(t, 1, 50)
	if got := int(page1["total"].(float64)); got != 73 {
		t.Fatalf("total: want 73, got %d", got)
	}
	p1Chars := variantChars(page1)
	if len(p1Chars) != 50 {
		t.Fatalf("page 1 item count: want 50, got %d", len(p1Chars))
	}
	// The first page must be the first 50 rows by frequency-desc order.
	for i, vc := range p1Chars {
		want := "v" + strconv.Itoa(i)
		if vc != want {
			t.Fatalf("page 1 item %d: want %q, got %q", i, want, vc)
		}
	}

	page2 := callListVariants(t, 2, 50)
	p2Chars := variantChars(page2)
	if len(p2Chars) != 23 {
		t.Fatalf("page 2 item count: want 23, got %d", len(p2Chars))
	}
	for i, vc := range p2Chars {
		want := "v" + strconv.Itoa(50 + i)
		if vc != want {
			t.Fatalf("page 2 item %d: want %q, got %q", i, want, vc)
		}
	}

	// page 3 should be empty (73 rows, 50 per page -> 2 pages).
	page3 := callListVariants(t, 3, 50)
	if len(page3["items"].([]interface{})) != 0 {
		t.Fatalf("page 3 should be empty, got %d items",
			len(page3["items"].([]interface{})))
	}
}

// TestListVariantsPaginationDefaultParams covers the default page=1 path so a
// future off-by-one regression on the default offset is also caught.
func TestListVariantsPaginationDefaultParams(t *testing.T) {
	defer setupTestDB(t)()
	seedVariants(t, 5)

	body := callListVariants(t, 1, 50)
	if got := int(body["total"].(float64)); got != 5 {
		t.Fatalf("total: want 5, got %d", got)
	}
	if len(body["items"].([]interface{})) != 5 {
		t.Fatalf("default page item count: want 5, got %d",
			len(body["items"].([]interface{})))
	}
}

// TestListVariantsPaginationSearchAndVerified ensures filters still compose
// with the corrected offset.
func TestListVariantsPaginationSearchAndVerified(t *testing.T) {
	defer setupTestDB(t)()

	rows := []model.VariantChar{
		{VariantChar: "v0", StandardChar: "s0", Frequency: 10, Verified: true},
		{VariantChar: "v1", StandardChar: "s1", Frequency: 9, Verified: false},
		{VariantChar: "v0dup", StandardChar: "s0", Frequency: 8, Verified: true},
	}
	if err := model.DB.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/variants", ListVariants)

	// verified=true -> 2 rows, page_size=1 -> page 1 returns v0 (highest freq).
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/variants?verified=true&page=1&page_size=1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if int(body["total"].(float64)) != 2 {
		t.Fatalf("verified total: want 2, got %v", body["total"])
	}
	items := body["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("verified page 1 count: want 1, got %d", len(items))
	}
	if got := items[0].(map[string]interface{})["variant_char"].(string); got != "v0" {
		t.Fatalf("verified page 1 item: want v0, got %s", got)
	}
}
