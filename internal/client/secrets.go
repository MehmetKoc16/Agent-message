package client

import "regexp"

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

// Atlatma bayrağı bilerek yoktur. Ad deseni büyük harfli .env geleneğine bakar
// (TOKEN, PASSWORD, SECRET, API_KEY içeren AD=değer satırları).
var secretPatterns = []secretPattern{
	{"PRIVATE KEY", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"sk-", regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`)},
	{"ghp_", regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`)},
	{"github_pat_", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)},
	{"AKIA", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"xox", regexp.MustCompile(`xox[bpas]-[A-Za-z0-9-]{10,}`)},
	{"AIza", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
	{"ENV-SATIRI (PASSWORD/SECRET/TOKEN/API_KEY)",
		regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?[A-Z0-9_]*(?:PASSWORD|SECRET|TOKEN|API_KEY)[A-Z0-9_]*[ \t]*=[ \t]*\S+`)},
}

// ScanSecrets ilk eşleşen desenin ADINI döndürür (değeri asla döndürmez).
func ScanSecrets(text string) (string, bool) {
	for _, p := range secretPatterns {
		if p.re.MatchString(text) {
			return p.name, true
		}
	}
	return "", false
}
