package history

// levenshteinRatio returns the normalised edit distance in [0.0, 1.0].
// Returns 0.0 when both strings are empty.
// Returns 1.0 when one is empty and the other is not.
// The normalised distance is editDistance(a, b) / max(runeLen(a), runeLen(b)).
// Strings are decoded to rune slices so that multi-byte UTF-8 characters
// are treated as single units rather than multiple bytes.
func levenshteinRatio(a, b string) float64 {
	aRunes, bRunes := []rune(a), []rune(b)
	la, lb := len(aRunes), len(bRunes)

	if la == 0 && lb == 0 {
		return 0.0
	}
	if la == 0 || lb == 0 {
		return 1.0
	}

	// Allocate DP rows.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := range lb + 1 {
		prev[j] = j
	}

	for i := range la {
		curr[0] = i + 1
		for j := range lb {
			cost := 1
			if aRunes[i] == bRunes[j] {
				cost = 0
			}
			del := prev[j+1] + 1
			ins := curr[j] + 1
			sub := prev[j] + cost
			curr[j+1] = min(del, min(ins, sub))
		}
		prev, curr = curr, prev
	}

	dist := prev[lb]
	return float64(dist) / float64(max(la, lb))
}
