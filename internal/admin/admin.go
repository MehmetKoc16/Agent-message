// Package admin, `agent-relay admin` alt komutlarını (doğrudan veritabanı) uygular.
package admin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/flagx"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

const usage = `kullanım: agent-relay admin --config <json> <komut>
  add-agent <ad> --rooms r1,r2 [--kind ajan|insan]   ajan ekler, token'ı BİR KEZ basar
  revoke <ad>                                         ajanı iptal eder
  list                                                ajanları listeler (token basmaz)
  reopen <oda> <thread>                               kilitli konuşmayı açar (+max_turns)
  purge                                               saklama süresi dolan mesajları siler`

// NewToken 32 rastgele bayttan base64url token üretir.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Run bir admin komutunu çalıştırır ve çıkış kodunu döndürür.
// Token yalnız add-agent'ta ve yalnız stdout'a, tek satır olarak yazılır.
func Run(ctx context.Context, cfg config.Server, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 1
	}
	st, err := store.Open(cfg.DB, cfg.MaxTurns)
	if err != nil {
		fmt.Fprintln(stderr, "hata:", err)
		return 1
	}
	defer st.Close()
	if err := run(ctx, cfg, st, args[0], args[1:], stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "hata:", err)
		return 1
	}
	return 0
}

func run(ctx context.Context, cfg config.Server, st *store.Store, cmd string, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	switch cmd {
	case "add-agent":
		rooms := fs.String("rooms", "", "virgülle ayrılmış odalar")
		kind := fs.String("kind", "ajan", "ajan | insan")
		pos, err := flagx.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("kullanım: add-agent <ad> --rooms r1,r2 [--kind ajan|insan]")
		}
		name := pos[0]
		if !config.NameRe.MatchString(name) {
			return fmt.Errorf("ajan adı geçersiz (desen: %s)", config.NameRe)
		}
		if *kind != "ajan" && *kind != "insan" {
			return errors.New("--kind ajan ya da insan olmalı")
		}
		var list []string
		for _, r := range strings.Split(*rooms, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if !config.NameRe.MatchString(r) {
				return fmt.Errorf("oda adı geçersiz: %q", r)
			}
			list = append(list, r)
		}
		if len(list) == 0 {
			return errors.New("--rooms zorunlu")
		}
		token, err := NewToken()
		if err != nil {
			return err
		}
		if err := st.AddAgent(ctx, name, *kind, store.HashToken(token), list); err != nil {
			if errors.Is(err, store.ErrExists) {
				return fmt.Errorf("ajan zaten var: %s", name)
			}
			return err
		}
		fmt.Fprintf(stderr, "ajan eklendi: %s (%s). Token yalnız bir kez gösterilir, hemen saklayın:\n", name, *kind)
		fmt.Fprintln(stdout, token)
		return nil

	case "revoke":
		pos, err := flagx.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("kullanım: revoke <ad>")
		}
		if err := st.RevokeAgent(ctx, pos[0]); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("ajan yok: %s", pos[0])
			}
			return err
		}
		fmt.Fprintf(stdout, "iptal edildi: %s\n", pos[0])
		return nil

	case "list":
		rows, err := st.ListAgents(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "AD\tTÜR\tODALAR\tOLUŞTURMA\tSON_GÖRÜLME\tDURUM")
		for _, a := range rows {
			state := "aktif"
			if a.RevokedAt != "" {
				state = "iptal " + a.RevokedAt
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", a.Name, a.Kind, strings.Join(a.Rooms, ","),
				a.CreatedAt, dash(a.LastSeen), state)
		}
		return tw.Flush()

	case "reopen":
		pos, err := flagx.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 2 {
			return errors.New("kullanım: reopen <oda> <thread>")
		}
		if err := st.ReopenThread(ctx, pos[0], pos[1], cfg.MaxTurns); err != nil {
			if errors.Is(err, store.ErrThreadNotFound) {
				return fmt.Errorf("konuşma yok: %s/%s", pos[0], pos[1])
			}
			return err
		}
		fmt.Fprintf(stdout, "açıldı: %s/%s (tur sınırı +%d)\n", pos[0], pos[1], cfg.MaxTurns)
		return nil

	case "purge":
		if _, err := flagx.Parse(fs, args); err != nil {
			return err
		}
		n, err := st.Purge(ctx, time.Now().Add(-time.Duration(cfg.RetentionDays)*24*time.Hour))
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "silinen mesaj: %d\n", n)
		return nil
	}
	fmt.Fprintln(stderr, usage)
	return fmt.Errorf("bilinmeyen komut: %s", cmd)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
