package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ancient-texts-backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupVariantTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&model.VariantChar{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for i := 0; i < 73; i++ {
		v := model.VariantChar{
			VariantChar:  string(rune(0x4e00 + i)),
			StandardChar: string(rune(0x5000 + i)),
			Verified:     true,
		}
		if err := db.Create(&v).Error; err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
	return db
}

func listVariantsRequest(t *testing.T, url string) (int, map[string]interface{}) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/variants", ListVariants)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	r.ServeHTTP(w, req)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return w.Code, body
}

func TestListVariantsPagination(t *testing.T) {
	model.DB = setupVariantTestDB(t)

	cases := []struct {
		url       string
		wantItems int
		wantTotal int
	}{
		{"/variants?page=1&page_size=50", 50, 73},
		{"/variants?page=2&page_size=50", 23, 73},
		{"/variants?page=3&page_size=50", 0, 73},
		{"/variants", 50, 73},
	}

	for _, tc := range cases {
		code, body := listVariantsRequest(t, tc.url)
		if code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", tc.url, code)
		}
		items, _ := body["items"].([]interface{})
		if len(items) != tc.wantItems {
			t.Errorf("%s: got %d items, want %d", tc.url, len(items), tc.wantItems)
		}
		if total := int(body["total"].(float64)); total != tc.wantTotal {
			t.Errorf("%s: total = %d, want %d", tc.url, total, tc.wantTotal)
		}
	}

	// Page 1 must start with the first record (ordered by variant_char asc
	// when frequency ties), and page 2 must end with the last record.
	_, page1 := listVariantsRequest(t, "/variants?page=1&page_size=50")
	_, page2 := listVariantsRequest(t, "/variants?page=2&page_size=50")
	first := page1["items"].([]interface{})[0].(map[string]interface{})
	last := page2["items"].([]interface{})[22].(map[string]interface{})
	if first["variant_char"] != string(rune(0x4e00)) {
		t.Errorf("page 1 first variant_char = %v, want %c", first["variant_char"], rune(0x4e00))
	}
	if last["variant_char"] != string(rune(0x4e00+72)) {
		t.Errorf("page 2 last variant_char = %v, want %c", last["variant_char"], rune(0x4e00+72))
	}

	// Every record should appear exactly once across the two pages.
	seen := map[string]bool{}
	for _, body := range []map[string]interface{}{page1, page2} {
		for _, it := range body["items"].([]interface{}) {
			vc := it.(map[string]interface{})["variant_char"].(string)
			if seen[vc] {
				t.Errorf("duplicate record across pages: %s", vc)
			}
			seen[vc] = true
		}
	}
	if len(seen) != 73 {
		t.Errorf("distinct records across pages = %d, want 73", len(seen))
	}
}

func TestListVariantsSearch(t *testing.T) {
	model.DB = setupVariantTestDB(t)

	code, body := listVariantsRequest(t, fmt.Sprintf("/variants?search=%c", rune(0x4e00)))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	items, _ := body["items"].([]interface{})
	if len(items) != 1 {
		t.Errorf("search returned %d items, want 1", len(items))
	}
}
