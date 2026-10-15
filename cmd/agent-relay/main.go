// agent-relay: relay sunucusu ve yönetici komutları.
//
//	agent-relay serve --config <json>
//	agent-relay admin --config <json> <komut>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/MehmetKoc16/Agent-message/internal/admin"
	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/server"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 || (args[0] != "serve" && args[0] != "admin") {
		fmt.Fprintln(os.Stderr, "kullanım: agent-relay serve|admin --config <json> [komut]")
		return 1
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	cfgPath := fs.String("config", "", "ayar dosyası (JSON)")
	// Yalnız baştaki bayraklar ana komuta aittir; komut adından sonrası alt komutundur.
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	pos := fs.Args()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "hata: --config zorunlu")
		return 1
	}
	cfg, err := config.LoadServer(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hata:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if args[0] == "admin" {
		return admin.Run(ctx, cfg, pos, os.Stdout, os.Stderr)
	}
	if len(pos) > 0 {
		fmt.Fprintln(os.Stderr, "hata: serve fazladan argüman almaz")
		return 1
	}
	return serve(ctx, cfg)
}

func serve(ctx context.Context, cfg config.Server) int {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Aktivasyon hatası varsa veritabanını açmadan açık hata ile dur.
	ln, err := server.Listen(cfg)
	if err != nil {
		log.Error("dinleyici açılamadı", "err", err.Error())
		return 1
	}
	defer ln.Close()
	st, err := store.Open(cfg.DB, cfg.MaxTurns)
	if err != nil {
		log.Error("veritabanı açılamadı", "err", err.Error())
		return 1
	}
	defer st.Close()
	srv := server.New(cfg, st, log)
	if err := srv.Serve(ctx, ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("sunucu durdu", "err", err.Error())
		return 1
	}
	return 0
}
