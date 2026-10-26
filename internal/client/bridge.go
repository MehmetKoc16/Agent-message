package client

import (
	"context"
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/bridge"
	"github.com/MehmetKoc16/Agent-message/internal/flagx"
)

// cmdBridge yalnız mevcut istemci yardımcılarını köprünün bağımsız döngüsüne bağlar.
func (r *runner) cmdBridge(ctx context.Context, fs *flag.FlagSet, args []string) int {
	o := bridge.Options{Room: r.cfg.Room, Agent: r.cfg.Agent}
	fs.StringVar(&o.Runner, "runner", "", "codex|agy")
	fs.StringVar(&o.Exe, "exe", "", "çalıştırıcı yolu (boş: standart kurulum)")
	fs.StringVar(&o.Workdir, "workdir", ".", "çalışma dizini")
	fs.StringVar(&o.DebugLog, "debug-log", "", "başarısız çalıştırıcıların sır taramalı stderr kaydı (isteğe bağlı)")
	fs.StringVar(&o.Model, "model", "", "model")
	fs.StringVar(&o.Effort, "effort", "medium", "Codex düşünme eforu")
	fs.StringVar(&o.Sandbox, "sandbox", "read-only", "Codex: read-only|workspace-write")
	fs.IntVar(&o.MaxRuns, "max-runs-per-hour", 20, "kayan saatte çalıştırma sınırı")
	seconds := fs.Int("runner-timeout", 900, "çalıştırıcı zaman aşımı (sn)")
	fs.BoolVar(&o.Once, "once", false, "bekleyen mesajları bir kez işle")
	allowFrom := fs.String("allow-from", "", "yalnız bu hesaplardan gelen mesajlar ajanı tetikler (ad1,ad2; boş: herkes)")
	fs.BoolVar(&o.IncludePublic, "include-public", false, "herkese açık (alıcısız) mesajları da işle; varsayılan yalnız köprüye yönlü mesajlar")
	pos, err := flagx.Parse(fs, args)
	if err != nil {
		return ExitError
	}
	if len(pos) != 0 || *seconds <= 0 || int64(*seconds) > int64((time.Duration(1<<63-1))/time.Second) {
		return r.fail(ExitError, "hata: bridge argümanları ya da runner-timeout geçersiz")
	}
	if *allowFrom != "" {
		for _, name := range strings.Split(*allowFrom, ",") {
			o.AllowFrom = append(o.AllowFrom, strings.TrimSpace(name))
		}
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "sandbox" {
			o.SandboxExplicit = true
		}
	})
	o.Timeout = time.Duration(*seconds) * time.Second
	var legacy string
	o.StatePath, legacy = stateFileNames(filepath.Dir(r.cur.path), "bridge", r.cfg.Room, r.cfg.Agent, o.Runner)
	// Bilinmeyen runner adı dosya yoluna girmeden önce bridge.Run içinde reddedilir.
	if o.Runner == "codex" || o.Runner == "agy" {
		if err := migrateLegacy(legacy, o.StatePath); err != nil {
			return r.fail(ExitError, "hata: %v", err)
		}
	}
	d := bridge.Dependencies{
		Cursor: r.cur, Frame: Render, Scan: r.scanSecrets,
		Fetch: r.api.fetch, Transient: isTransient,
		Send: func(ctx context.Context, in bridge.Outgoing) error {
			_, err := r.api.send(ctx, sendInput{To: in.To, Type: in.Type, Thread: in.Thread, ReplyTo: &in.ReplyTo, Body: in.Body, RequestID: in.RequestID})
			return err
		},
		Conflict: func(err error) bool {
			var ae *APIError
			return errors.As(err, &ae) && ae.Status == 409 && ae.Code != "istek_cakismasi"
		},
		RateLimited: func(err error) bool {
			var ae *APIError
			return errors.As(err, &ae) && ae.Status == 429
		},
		Lock:    func(path string) (func(), error) { return (cursor{path: path}).lock() },
		Replace: renameRetry, Stderr: r.env.Stderr,
		PollSeconds: pollSeconds, RetryDelay: retryDelay,
	}
	if err := bridge.Run(ctx, o, d); err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return r.fail(ExitCanceled, "iptal edildi")
		}
		return r.fetchFail(err)
	}
	return ExitOK
}
