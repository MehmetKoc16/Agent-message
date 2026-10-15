package bridge

import (
	"strings"
	"unicode"
)

// Normalleştirme yalnız işaretleri tanımak içindir; gönderilen cevabı değiştirmez.
func responseKind(body string) (silent, suspicious bool) {
	normalize := func(s string) string {
		s = strings.Map(func(r rune) rune {
			if strings.ContainsRune("*_`#", r) {
				return -1
			}
			return r
		}, s)
		return strings.ToUpperSpecial(unicode.TurkishCase, strings.TrimSpace(s))
	}
	clean := normalize(body)
	whole := strings.TrimRightFunc(clean, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSpace(r) })
	first, _, _ := strings.Cut(clean, "\n")
	first = strings.TrimSpace(first)
	silent = strings.TrimSpace(body) == "" || whole == "SESSIZ" || whole == "SESSİZ" || strings.HasPrefix(first, "SESSIZ:") || strings.HasPrefix(first, "SESSİZ:")
	suspicious = strings.HasPrefix(first, "ŞÜPHELİ:") || strings.HasPrefix(first, "ŞÜPHELI:")
	return
}
