package bridge

import (
	"syscall"
	"unicode/utf16"
)

// Windows CreateProcess sınırı UTF-16 birimleriyle ölçülür. Go'nun kullandığı
// EscapeArg ile exe, prompt, model ve bütün diğer seçeneklerin gerçek payını say.
func agyCommandFits(exe string, args []string) bool {
	units := func(s string) int {
		n := 0
		for _, r := range s {
			n += utf16.RuneLen(r)
		}
		return n
	}
	n := units(syscall.EscapeArg(exe))
	for _, arg := range args {
		n += 1 + units(syscall.EscapeArg(arg))
	}
	return n <= 32000
}
