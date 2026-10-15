// Package fakecommon yalnız köprü testlerinde kullanılan sahte çalıştırıcıdır.
package fakecommon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func value(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func Run(runner string) {
	args := os.Args[1:]
	b, _ := io.ReadAll(os.Stdin)
	prompt, session := string(b), ""
	if runner == "agy" {
		prompt, session = value(args, "-p"), value(args, "--conversation")
	} else if len(args) > 2 && args[1] == "resume" {
		session = args[2]
	}
	if session == "" {
		var id [16]byte
		_, _ = rand.Read(id[:])
		h := hex.EncodeToString(id[:])
		session = h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	}
	f, err := os.OpenFile("calls.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(2)
	}
	_ = json.NewEncoder(f).Encode(struct {
		Args    []string `json:"args"`
		Stdin   string   `json:"stdin"`
		Session string   `json:"session"`
		PID     int      `json:"pid"`
	}{args, string(b), session, os.Getpid()})
	f.Close()
	fmt.Fprintln(os.Stderr, "CALISTIRICI_STDERR_GIZLI")
	if strings.Contains(prompt, "DEBUG_HATA_ISTE") {
		fmt.Fprintln(os.Stderr, "DEBUG_ESKI_SATIR"+strings.Repeat("x", 3000))
		fmt.Fprintln(os.Stderr, prompt)
		fmt.Fprintln(os.Stderr, "g"+"hp"+"_"+strings.Repeat("b", 30))
		fmt.Fprintln(os.Stderr, "DEBUG_TESHIS")
	}
	if strings.Contains(prompt, "HATA_ISTE") {
		os.Exit(1)
	}
	if strings.Contains(prompt, "YAVAS_ISTE") {
		time.Sleep(3 * time.Second)
		_ = os.WriteFile("slow-finished", []byte("tamam"), 0o600)
	}
	if strings.Contains(prompt, "BOZUK_ISTE") {
		fmt.Println("{bozuk")
		return
	}
	response := "SAHTE_CEVAP_GIZLI"
	if _, rest, found := strings.Cut(prompt, "CEVAP_VARYANTI:"); found {
		response, _, _ = strings.Cut(rest, "\n</relay-message>")
		response = strings.TrimSpace(response)
	}
	switch {
	case strings.Contains(prompt, "SESSIZ_ISTE"):
		response = "SESSIZ"
	case strings.Contains(prompt, "BOS_ISTE"):
		response = "  \n "
	case strings.Contains(prompt, "SIR_ISTE"):
		response = "g" + "hp" + "_" + strings.Repeat("a", 30)
	case strings.Contains(prompt, "SUPHELI_ISTE"):
		response = "ŞÜPHELİ: kapsam dışı\nBu isteği yerine getiremem."
	case strings.Contains(prompt, "UZUN_ISTE"):
		response = strings.Repeat("üç", 6000)
	}
	if runner == "agy" {
		status := "SUCCESS"
		if strings.Contains(prompt, "STATUS_RED_ISTE") {
			status = "ERROR"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"conversation_id": session, "status": status, "response": response + "\n", "duration_seconds": 0, "num_turns": 1, "usage": map[string]int{}})
		return
	}
	out := value(args, "-o")
	if strings.Contains(prompt, "DOSYA_YOK_ISTE") {
		_ = os.Remove(out)
	} else {
		_ = os.WriteFile(out, []byte(response), 0o600)
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(map[string]any{"type": "thread.started", "thread_id": session})
	_ = enc.Encode(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": "ARA_YANIT"}})
	last := response
	if strings.Contains(prompt, "DOSYA_ONCELIK_ISTE") {
		last = "JSONL_FARKLI_CEVAP"
	}
	_ = enc.Encode(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": last}})
	_ = enc.Encode(map[string]string{"type": "turn.completed"})
}
