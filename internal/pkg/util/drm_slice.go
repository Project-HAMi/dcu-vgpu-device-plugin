package util

import (
	"fmt"
	"strings"
)

// DrmSlice is a struct for sort to natural sorting, we assume that the element is a 'card%d'/'renderD%d' format.
// Note: if you want a common natural sorting, you should NOT use this one.
type DrmSlice []string

func (p DrmSlice) Len() int           { return len(p) }
func (p DrmSlice) Less(i, j int) bool { return DrmSortLess(p[i], p[j]) }
func (p DrmSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func DrmSortLess(a, b string) bool {
	cardA := strings.HasPrefix(a, DriPrefixCard)
	cardB := strings.HasPrefix(b, DriPrefixCard)
	if cardA && cardB {
		var aid, bid int
		fmt.Sscanf(a, "card%d", &aid)
		fmt.Sscanf(b, "card%d", &bid)
		return aid <= bid
	}
	renderA := strings.HasPrefix(a, DriPrefixRender)
	renderB := strings.HasPrefix(b, DriPrefixRender)
	if renderA && renderB {
		var aid, bid int
		fmt.Sscanf(a, "renderD%d", &aid)
		fmt.Sscanf(b, "renderD%d", &bid)
		return aid <= bid
	}
	return a <= b
}
