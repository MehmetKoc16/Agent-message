package client

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

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

// stripFormat görünmez biçim (Cf) karakterlerini çıkarır. Yalnız tarama kopyasında
// kullanılır: araya U+200B konmuş bir sır desenden kaçamasın; gönderilen metin değişmez.
func stripFormat(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
}

// ScanSecrets ilk eşleşen desenin ADINI döndürür (değeri asla döndürmez).
func ScanSecrets(text string) (string, bool) {
	text = stripFormat(text)
	for _, p := range secretPatterns {
		if p.re.MatchString(text) {
			return p.name, true
		}
	}
	return "", false
}

// The configured credential has no recognizable provider prefix. Compare the
// known value as well as generic patterns, without ever returning the value.
// Aynı dizindeki diğer relay ayarlarının token'ları da (siblingTokens) karşılaştırılır.
func (r *runner) scanSecrets(text string) (string, bool) {
	clean := stripFormat(text)
	if r.cfg != nil && r.cfg.Token != "" && strings.Contains(clean, r.cfg.Token) {
		return "relay kimlik bilgisi", true
	}
	for _, tok := range r.tokens {
		if strings.Contains(clean, tok) {
			return "relay kimlik bilgisi", true
		}
	}
	return ScanSecrets(clean)
}

const (
	minSiblingToken  = 16
	maxSiblingConfig = 1 << 20
)

// siblingTokens aktif ayarın dizinindeki *.json dosyalarından üst düzey "token"
// alanlarını toplar. Aynı makinedeki başka relay hesaplarının token'ları bilinen bir
// önek taşımadığı için desenle yakalanamaz. Okunamayan, büyük ya da bozuk dosya
// sessizce atlanır; değerler yalnız bellekte tutulur ve hiçbir yere yazılmaz.
func siblingTokens(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(f, maxSiblingConfig+1))
		f.Close()
		if err != nil || len(b) > maxSiblingConfig {
			continue
		}
		var v struct {
			Token json.RawMessage `json:"token"`
		}
		if json.Unmarshal(b, &v) != nil || len(v.Token) == 0 {
			continue
		}
		var tok string
		if json.Unmarshal(v.Token, &tok) != nil || utf8.RuneCountInString(tok) < minSiblingToken {
			continue
		}
		out = append(out, tok)
	}
	return out
}
