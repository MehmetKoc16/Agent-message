// olcum: sunucunun bellek kullanımını ölçer.
//
// agent-relay'i AYRI SÜREÇ olarak başlatır (geçici dizin, 127.0.0.1'de boş port), 4 ajanı
// long-poll'da bekletir, başka bir ajanla 200 mesaj gönderir ve süreç belleğini
// Windows'ta `tasklist /FI "PID eq <pid>" /FO CSV` ile (Linux'ta `ps -o rss=`) ölçer.
// Ölçülen değer "Mem Usage" yani çalışma kümesidir (working set).
//
// Kullanım: go run ./tools/olcum -bin bin/agent-relay.exe
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	room     = "olcum"
	nWaiters = 4
	nMsgs    = 200
)

func main() {
	bin := flag.String("bin", "", "agent-relay ikilisi")
	flag.Parse()
	if *bin == "" {
		fmt.Fprintln(os.Stderr, "kullanım: olcum -bin <agent-relay ikilisi>")
		os.Exit(1)
	}
	if err := run(*bin); err != nil {
		fmt.Fprintln(os.Stderr, "HATA:", err)
		os.Exit(1)
	}
}

func run(bin string) error {
	dir, err := os.MkdirTemp("", "olcum-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	port, err := freePort()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "server.json")
	cfg := map[string]any{
		"host": "127.0.0.1", "port": port, "db": filepath.Join(dir, "relay.db"),
		"rate_per_min": 5000, "rate_per_day": 50000, "max_conns_per_agent": 8,
	}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		return err
	}

	// Ajanlar (admin alt komutuyla; token stdout'tan okunur).
	names := []string{"gonderen"}
	for i := 1; i <= nWaiters; i++ {
		names = append(names, fmt.Sprintf("bekleyen%d", i))
	}
	tokens := map[string]string{}
	for _, n := range names {
		out, err := exec.Command(bin, "admin", "--config", cfgPath, "add-agent", n, "--rooms", room).Output()
		if err != nil {
			return fmt.Errorf("add-agent %s: %w", n, err)
		}
		tokens[n] = strings.TrimSpace(string(out))
	}

	// Sunucuyu ayrı süreç olarak başlat; HER YOLDA kapatılır.
	var logBuf bytes.Buffer
	srv := exec.Command(bin, "serve", "--config", cfgPath)
	srv.Stderr = &logBuf
	if err := srv.Start(); err != nil {
		return err
	}
	pid := srv.Process.Pid
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			_ = srv.Process.Kill()
			_ = srv.Wait()
		}
	}
	defer stop()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitHealthy(base); err != nil {
		return fmt.Errorf("%w\nsunucu logu:\n%s", err, logBuf.String())
	}
	time.Sleep(300 * time.Millisecond)
	idle, err := rss(pid)
	if err != nil {
		return err
	}

	// 4 bekleyen: long-poll döngüsü, 200 mesaj alana dek.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var delivered atomic.Int64
	var wg sync.WaitGroup
	for i := 1; i <= nWaiters; i++ {
		name := fmt.Sprintf("bekleyen%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			var after int64
			got := 0
			for got < nMsgs && ctx.Err() == nil {
				msgs, err := poll(ctx, base, tokens[name], after)
				if err != nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				for _, m := range msgs {
					after = max(after, m)
				}
				got += len(msgs)
				delivered.Add(int64(len(msgs)))
			}
		}()
	}
	time.Sleep(500 * time.Millisecond) // bekleyenler bağlansın
	waiting, err := rss(pid)
	if err != nil {
		return err
	}

	// Gönderim sırasında bellek örneklenir (tepe değer).
	peak := waiting
	sampleDone := make(chan struct{})
	var sampWG sync.WaitGroup
	sampWG.Add(1)
	go func() {
		defer sampWG.Done()
		for {
			select {
			case <-sampleDone:
				return
			case <-time.After(40 * time.Millisecond):
				if v, err := rss(pid); err == nil {
					peak = max(peak, v)
				}
			}
		}
	}()

	start := time.Now()
	body := strings.Repeat("ölçüm mesajı ", 15) // ~200 bayt
	for i := 0; i < nMsgs; i++ {
		if err := post(ctx, base, tokens["gonderen"], body); err != nil {
			close(sampleDone)
			sampWG.Wait()
			return fmt.Errorf("gönderim %d: %w", i, err)
		}
	}
	wg.Wait()
	elapsed := time.Since(start)
	close(sampleDone)
	sampWG.Wait()
	time.Sleep(300 * time.Millisecond)
	after, err := rss(pid)
	if err != nil {
		return err
	}

	fmt.Printf("sunucu PID: %d (ölçüm bitince kapatılır)\n", pid)
	fmt.Printf("bekleyen ajan: %d, gönderilen mesaj: %d, teslim edilen: %d (beklenen %d), süre: %v\n",
		nWaiters, nMsgs, delivered.Load(), nWaiters*nMsgs, elapsed.Round(time.Millisecond))
	fmt.Printf("bellek (çalışma kümesi):\n")
	fmt.Printf("  boşta (başlangıç):           %6.1f MB\n", mb(idle))
	fmt.Printf("  4 ajan wait'te:              %6.1f MB\n", mb(waiting))
	fmt.Printf("  200 mesaj sırasında tepe:    %6.1f MB\n", mb(peak))
	fmt.Printf("  200 mesaj sonrası:           %6.1f MB\n", mb(after))
	stop()
	if delivered.Load() != int64(nWaiters*nMsgs) {
		return fmt.Errorf("teslimat eksik: %d/%d", delivered.Load(), nWaiters*nMsgs)
	}
	return nil
}

func mb(kb int) float64 { return float64(kb) / 1024 }

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitHealthy(base string) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/v1/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("sunucu 15 sn içinde hazır olmadı")
}

func poll(ctx context.Context, base, token string, after int64) ([]int64, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/v1/rooms/%s/messages?after=%d&wait=20", base, room, after), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Messages []struct {
			ID int64 `json:"id"`
		} `json:"messages"`
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	ids := make([]int64, len(r.Messages))
	for i, m := range r.Messages {
		ids[i] = m.ID
	}
	return ids, nil
}

func post(ctx context.Context, base, token, body string) error {
	payload, _ := json.Marshal(map[string]string{"type": "bilgi", "body": body})
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/v1/rooms/"+room+"/messages", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 201 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// rss sürecin bellek kullanımını KB olarak verir.
func rss(pid int) (int, error) {
	if runtime.GOOS != "windows" {
		out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(strings.TrimSpace(string(out)))
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0, err
	}
	rec, err := csv.NewReader(bytes.NewReader(out)).Read()
	if err != nil || len(rec) < 5 {
		return 0, fmt.Errorf("tasklist çıktısı çözülemedi: %q", out)
	}
	// "Mem Usage" son sütun: "12.345 K" ya da "12,345 K" (yerel ayara göre).
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, rec[len(rec)-1])
	return strconv.Atoi(digits)
}
