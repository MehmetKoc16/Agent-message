//go:build !windows

package bridge

import "unicode/utf16"

// Diğer platformlarda önceki 28000 birimlik prompt sınırını koru.
func agyCommandFits(_ string, args []string) bool {
	if len(args) < 2 {
		return false
	}
	n := 0
	for _, r := range args[1] {
		n += utf16.RuneLen(r)
	}
	return n <= 28000
}
