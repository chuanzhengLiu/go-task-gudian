package diff

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestCompareVersions_SimilarityIdenticalText(t *testing.T) {
	text := "天何言哉，百姓何議"
	res := CompareVersions(text, text, [2]string{"A", "B"})

	if !approxEqual(res.Similarity, 1.0) {
		t.Fatalf("identical text similarity = %g, want 1.0 (frontend draws this as a progress bar)", res.Similarity)
	}
	if res.Insertions != 0 || res.Deletions != 0 {
		t.Fatalf("identical text should have no insertions/deletions, got ins=%d del=%d", res.Insertions, res.Deletions)
	}
	if res.Equal != len([]rune(text)) {
		t.Fatalf("equal rune count = %d, want %d", res.Equal, len([]rune(text)))
	}
}

func TestCompareVersions_SimilarityEmpty(t *testing.T) {
	res := CompareVersions("", "", [2]string{"A", "B"})
	if res.Similarity != 0 {
		t.Fatalf("empty/empty similarity = %g, want 0 (total==0 guard)", res.Similarity)
	}
}

func TestCompareVersions_SimilarityPartial(t *testing.T) {
	// A="abcd", B="abxd": the common subsequence is "abd" (a, b, and d all match in order),
	// only the middle c/x differs. Equal=3, len(A)+len(B)=8, similarity = 2*3/8 = 0.75.
	res := CompareVersions("abcd", "abxd", [2]string{"A", "B"})
	if !approxEqual(res.Similarity, 0.75) {
		t.Fatalf("partial similarity = %g, want 0.75", res.Similarity)
	}
}

func TestCompareVersions_SimilarityDisjoint(t *testing.T) {
	// No characters in common.
	res := CompareVersions("abcd", "wxyz", [2]string{"A", "B"})
	if res.Similarity != 0 {
		t.Fatalf("disjoint text similarity = %g, want 0", res.Similarity)
	}
	if res.Equal != 0 {
		t.Fatalf("equal should be 0, got %d", res.Equal)
	}
}
