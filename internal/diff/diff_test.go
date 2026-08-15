package diff

import "testing"

func TestCompareVersionsSimilarity(t *testing.T) {
	text := "学而时习之，不亦说乎？有朋自远方来，不亦乐乎？"

	cases := []struct {
		name     string
		a, b     string
		min, max float64
	}{
		{"identical", text, text, 1, 1},
		{"both_empty", "", "", 1, 1},
		{"one_empty", text, "", 0, 0},
		{"single_char_diff", "学而时习之", "学而时习之乎", 0.8, 1},
		{"completely_different", "天地玄黄", "宇宙洪荒", 0, 0.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompareVersions(tc.a, tc.b, [2]string{"甲", "乙"}).Similarity
			if got < tc.min || got > tc.max {
				t.Errorf("similarity = %v, want in [%v, %v]", got, tc.min, tc.max)
			}
		})
	}
}
