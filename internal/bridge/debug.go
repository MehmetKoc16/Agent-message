package bridge

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

const maxDebugStderr = 2 << 10

// Yalnız son 2 KB tutulur. Kesilmiş ilk satır, başındaki sır deseni kaybolmuş
// olabileceği için yazılmaz. Başarılı turlarda bu tampon tamamen atılır.
type stderrTail struct {
	data    []byte
	partial bool
}

func (b *stderrTail) Write(p []byte) (int, error) {
	n := len(p)
	b.data = append(b.data, p...)
	if drop := len(b.data) - maxDebugStderr; drop > 0 {
		b.partial = b.data[drop-1] != '\n'
		copy(b.data, b.data[drop:])
		b.data = b.data[:maxDebugStderr]
	}
	return n, nil
}

func (b *stderrTail) complete() string {
	data := b.data
	if b.partial {
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			return ""
		}
	}
	return strings.ToValidUTF8(string(data), "�")
}

// debug yalnız açıkça seçilen dosyaya gider. Sır içeren satır bütünüyle gizlenir;
// çalıştırıcının prompt'u stderr'e yansıtması halinde o satırlar da çıkarılır.
func (e *engine) debug(m proto.Message, result runResult, prompt string, pending []proto.Message) {
	if e.o.DebugLog == "" {
		return
	}
	private := strings.Split(prompt, "\n")
	private = append(private, strings.Split(m.Body, "\n")...)
	for _, old := range pending {
		private = append(private, strings.Split(old.Body, "\n")...)
	}
	var b strings.Builder
	lines := strings.Split(strings.TrimRight(result.Stderr, "\r\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if name, found := e.d.Scan(line); found {
			line = "[sır deseni gizlendi: " + name + "]"
		} else {
			for _, text := range private {
				text = strings.TrimSpace(text)
				if text != "" && (strings.Contains(line, text) || (strings.TrimSpace(line) != "" && strings.Contains(text, strings.TrimSpace(line)))) {
					line = "[prompt metni gizlendi]"
					break
				}
			}
		}
		// %q kontrol karakterlerinin sahte kayıt satırı oluşturmasını da önler.
		fmt.Fprintf(&b, "%s konuşma=%q mesaj=%d çıkış=%d stderr=%q\n", time.Now().Format(time.RFC3339), m.Thread, m.ID, result.ExitCode, line)
	}
	f, err := os.OpenFile(e.o.DebugLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		err = f.Chmod(0o600)
		if err == nil {
			_, err = f.WriteString(b.String())
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		e.log(m, "hata", 0, " debug_log=yazılamadı")
	}
}
