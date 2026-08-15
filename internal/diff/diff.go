package diff

import (
	"sort"
	"unicode"
)

type DiffOperation string

const (
	OpEqual   DiffOperation = "equal"
	OpInsert  DiffOperation = "insert"
	OpDelete  DiffOperation = "delete"
	OpReplace DiffOperation = "replace"
)

type DiffChunk struct {
	Operation DiffOperation `json:"operation"`
	Text      string        `json:"text"`
	Position  int           `json:"position"`
	Length    int           `json:"length"`
}

type DiffResult struct {
	Chunks      []DiffChunk `json:"chunks"`
	Insertions  int         `json:"insertions"`
	Deletions   int         `json:"deletions"`
	Equal       int         `json:"equal"`
	Similarity  float64     `json:"similarity"`
}

type Point struct {
	X, Y int
}

type Snake struct {
	Start Point
	End   Point
	Diag  int
}

func isFunctionWord(ch rune) bool {
	functionWords := map[rune]bool{
		'之': true, '乎': true, '者': true, '也': true, '矣': true,
		'焉': true, '哉': true, '而': true, '以': true, '于': true,
		'其': true, '所': true, '与': true, '及': true,
		'则': true, '乃': true, '即': true, '遂': true, '且': true,
		'然': true, '故': true, '虽': true, '若': true, '如': true,
	}
	return functionWords[ch]
}

func MyersDiff(a, b string) []DiffChunk {
	runesA := []rune(a)
	runesB := []rune(b)

	n := len(runesA)
	m := len(runesB)
	max := n + m
	if max == 0 {
		return []DiffChunk{}
	}

	v := make(map[int]int)
	v[1] = 0
	var trace []map[int]int

	for d := 0; d <= max; d++ {
		vCopy := make(map[int]int)
		for k, val := range v {
			vCopy[k] = val
		}
		trace = append(trace, vCopy)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[k-1] < v[k+1]) {
				x = v[k+1]
			} else {
				x = v[k-1] + 1
			}
			y := x - k

			for x < n && y < m && runesA[x] == runesB[y] {
				x++
				y++
			}

			v[k] = x

			if x >= n && y >= m {
				return backtrack(trace, runesA, runesB)
			}
		}
	}

	return []DiffChunk{}
}

func backtrack(trace []map[int]int, a, b []rune) []DiffChunk {
	var chunks []DiffChunk

	x := len(a)
	y := len(b)

	for d := len(trace) - 1; d > 0; d-- {
		v := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && v[k-1] < v[k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := v[prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
		}

		if d > 0 {
			if prevX == x {
				chunks = append(chunks, DiffChunk{
					Operation: OpInsert,
					Text:      string(b[prevY:y]),
					Position:  prevX,
					Length:    y - prevY,
				})
			} else if prevY == y {
				chunks = append(chunks, DiffChunk{
					Operation: OpDelete,
					Text:      string(a[prevX:x]),
					Position:  prevX,
					Length:    x - prevX,
				})
			} else {
				chunks = append(chunks, DiffChunk{
					Operation: OpReplace,
					Text:      string(a[prevX:x]),
					Position:  prevX,
					Length:    x - prevX,
				})
				chunks = append(chunks, DiffChunk{
					Operation: OpInsert,
					Text:      string(b[prevY:y]),
					Position:  prevX,
					Length:    y - prevY,
				})
			}
		}

		x = prevX
		y = prevY
	}

	for i, j := 0, len(chunks)-1; i < j; i, j = i+1, j-1 {
		chunks[i], chunks[j] = chunks[j], chunks[i]
	}

	return mergeAdjacentChunks(chunks)
}

func mergeAdjacentChunks(chunks []DiffChunk) []DiffChunk {
	if len(chunks) < 2 {
		return chunks
	}

	var merged []DiffChunk
	current := chunks[0]

	for i := 1; i < len(chunks); i++ {
		next := chunks[i]
		if current.Operation == next.Operation && current.Position+current.Length == next.Position {
			current.Text += next.Text
			current.Length += next.Length
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	return merged
}

func CompareVersions(textA, textB string, labels [2]string) *DiffResult {
	runesA := []rune(textA)
	runesB := []rune(textB)

	diffChunks := MyersDiff(textA, textB)

	var chunksWithEqual []DiffChunk
	posA := 0

	for _, chunk := range diffChunks {
		if chunk.Position > posA {
			chunksWithEqual = append(chunksWithEqual, DiffChunk{
				Operation: OpEqual,
				Text:      string(runesA[posA:chunk.Position]),
				Position:  posA,
				Length:    chunk.Position - posA,
			})
		}
		chunksWithEqual = append(chunksWithEqual, chunk)
		// insert 不消耗 A 的字符，但 equal 段不能越过插入点重复统计。
		if chunk.Position > posA {
			posA = chunk.Position
		}
		if chunk.Operation == OpEqual || chunk.Operation == OpDelete || chunk.Operation == OpReplace {
			posA = chunk.Position + chunk.Length
		}
	}

	if posA < len(runesA) {
		chunksWithEqual = append(chunksWithEqual, DiffChunk{
			Operation: OpEqual,
			Text:      string(runesA[posA:]),
			Position:  posA,
			Length:    len(runesA) - posA,
		})
	}

	result := &DiffResult{
		Chunks: chunksWithEqual,
	}

	for _, chunk := range chunksWithEqual {
		switch chunk.Operation {
		case OpEqual:
			result.Equal += chunk.Length
		case OpInsert:
			result.Insertions += chunk.Length
		case OpDelete, OpReplace:
			result.Deletions += chunk.Length
		}
	}

	// Dice 系数：相等字符在两份文本中各出现一次，故分子为 2*Equal，
	// 保证完全相同的文本相似度为 1。
	total := len(runesA) + len(runesB)
	if total > 0 {
		result.Similarity = float64(2*result.Equal) / float64(total)
	} else {
		result.Similarity = 1
	}

	return result
}

func GenerateEmendation(diffResult *DiffResult, labels [2]string) []map[string]interface{} {
	var emendations []map[string]interface{}

	for _, chunk := range diffResult.Chunks {
		if chunk.Operation == OpEqual {
			continue
		}

		emendation := map[string]interface{}{
			"position": chunk.Position,
		}

		switch chunk.Operation {
		case OpDelete:
			emendation["type"] = "delete"
			emendation["note"] = labels[0] + "本有「" + chunk.Text + "」，" + labels[1] + "本无"
		case OpInsert:
			emendation["type"] = "insert"
			emendation["note"] = labels[0] + "本无，" + labels[1] + "本作「" + chunk.Text + "」"
		case OpReplace:
			emendation["type"] = "replace"
			emendation["note"] = labels[0] + "本作「" + chunk.Text + "」，" + labels[1] + "本..."
		}

		emendations = append(emendations, emendation)
	}

	return emendations
}

func FindVariantChars(text string, variantMap map[string]string) []map[string]interface{} {
	runes := []rune(text)
	var results []map[string]interface{}

	for i, r := range runes {
		str := string(r)
		if standard, ok := variantMap[str]; ok {
			results = append(results, map[string]interface{}{
				"position":      i,
				"variant_char":  str,
				"standard_char": standard,
				"context":       getContext(runes, i),
			})
		}
	}

	return results
}

func getContext(runes []rune, pos int) string {
	start := max(0, pos-5)
	end := min(len(runes), pos+6)
	return string(runes[start:end])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func SplitByPunctuation(text string) []string {
	var segments []string
	var current []rune

	for _, r := range []rune(text) {
		current = append(current, r)
		if unicode.IsPunct(r) || r == '\n' || r == '。' || r == '，' || r == '；' || r == '：' {
			if len(current) > 0 {
				segments = append(segments, string(current))
				current = nil
			}
		}
	}

	if len(current) > 0 {
		segments = append(segments, string(current))
	}

	return segments
}

func ThreeWayDiff(base, a, b string) map[string]interface{} {
	diffAB := CompareVersions(base, a, [2]string{"base", "a"})
	diffAC := CompareVersions(base, b, [2]string{"base", "b"})

	conflicts := findConflicts(diffAB.Chunks, diffAC.Chunks)

	return map[string]interface{}{
		"diff_a":    diffAB,
		"diff_b":    diffAC,
		"conflicts": conflicts,
		"has_conflict": len(conflicts) > 0,
	}
}

func findConflicts(chunksA, chunksB []DiffChunk) []map[string]interface{} {
	var conflicts []map[string]interface{}

	for _, chunkA := range chunksA {
		if chunkA.Operation == OpEqual {
			continue
		}

		for _, chunkB := range chunksB {
			if chunkB.Operation == OpEqual {
				continue
			}

			aStart := chunkA.Position
			aEnd := chunkA.Position + chunkA.Length
			bStart := chunkB.Position
			bEnd := chunkB.Position + chunkB.Length

			if aStart < bEnd && aEnd > bStart {
				conflicts = append(conflicts, map[string]interface{}{
					"position": max(aStart, bStart),
					"length":   min(aEnd, bEnd) - max(aStart, bStart),
					"version_a": chunkA,
					"version_b": chunkB,
				})
			}
		}
	}

	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i]["position"].(int) < conflicts[j]["position"].(int)
	})

	return conflicts
}
