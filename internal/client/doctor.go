package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/flagx"
)

const doctorUsage = "kullanım: relay [--config <json>] doctor [--timeout 10s]"
const doctorMaxBytes = 1 << 20

// Çıktılar yalnız sabit metinlerden oluşur. URL, token, dosya yolu, hesap/oda
// adları, sunucu gövdesi ve alttaki hata metni teşhis raporuna taşınmaz.
func doctorLine(env Env, status, check, message string) {
	fmt.Fprintf(env.Stdout, "[%s] %s: %s\n", status, check, message)
}

type doctorTrace struct {
	mu                sync.Mutex
	tlsStarted, tlsOK bool
}

func (d *doctorTrace) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		TLSHandshakeStart: func() {
			d.mu.Lock()
			d.tlsStarted = true
			d.mu.Unlock()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			d.mu.Lock()
			d.tlsOK = err == nil
			d.mu.Unlock()
		},
	}
}

func (d *doctorTrace) networkError(ctx context.Context, env Env, err error) int {
	if errors.Is(ctx.Err(), context.Canceled) {
		doctorLine(env, "HATA", "İPTAL", "Teşhis iptal edildi.")
		return ExitCanceled
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		doctorLine(env, "HATA", "DNS", "Sunucu adı çözümlenemedi; alan adını ve DNS bağlantısını kontrol et.")
		return ExitUnreachable
	}
	var timed net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.As(err, &timed) && timed.Timeout() {
		doctorLine(env, "HATA", "ZAMAN AŞIMI", "Teşhis süresi doldu; bağlantıyı kontrol et veya --timeout süresini artır.")
		return ExitUnreachable
	}
	d.mu.Lock()
	tlsFailed := d.tlsStarted && !d.tlsOK
	d.mu.Unlock()
	var cert *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var record tls.RecordHeaderError
	if tlsFailed || errors.As(err, &cert) || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &invalid) || errors.As(err, &record) {
		doctorLine(env, "HATA", "TLS", "TLS bağlantısı doğrulanamadı; sertifikayı, HTTPS adresini ve sistem saatini kontrol et. Sertifika denetimi kapatılmadı.")
		return ExitUnreachable
	}
	doctorLine(env, "HATA", "BAĞLANTI", "Sunucuya ulaşılamadı veya bağlantı kesildi; adresi, portu ve sunucu hizmetini kontrol et.")
	return ExitUnreachable
}

// doctorGet yalnız sabit GET yollarını çağırır. Sağlık isteğine token eklenmez;
// HTTP istemcisi yönlendirmeleri izlemez. Yanıt/URL/hata hiçbir zaman basılmaz.
func doctorGet(ctx context.Context, hc *http.Client, cfg *Config, path string, auth bool, out any) (int, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL+path, nil)
	if err != nil {
		return 0, err
	}
	if auth {
		r.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	r.Header.Set("Cache-Control", "no-store")
	resp, err := hc.Do(r)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, doctorMaxBytes+1))
	if err != nil {
		return 0, err
	}
	if len(data) > doctorMaxBytes || json.Unmarshal(data, out) != nil {
		return resp.StatusCode, errors.New("geçersiz teşhis yanıtı")
	}
	return resp.StatusCode, nil
}

func doctorHTTP(env Env, check string, status int) int {
	switch {
	case status >= 300 && status < 400:
		doctorLine(env, "HATA", check, "Sunucu yönlendirme döndürüyor; ayardaki URL'yi doğrudan relay adresiyle düzelt. Yönlendirme izlenmedi.")
	case check == "SAĞLIK" && (status == 401 || status == 403):
		doctorLine(env, "HATA", check, "Sağlık yoluna erişim reddedildi; URL'yi ve ters proxy erişim kurallarını kontrol et. Token henüz sınanmadı.")
	case status == 401:
		doctorLine(env, "HATA", check, "Token geçersiz veya hesap iptal edilmiş; yöneticinle görüşüp relay setup ile yeniden bağlan.")
	case status == 403:
		doctorLine(env, "HATA", check, "Hesabın bu odaya veya işleme erişimi yok; yöneticinden üyeliği kontrol etmesini iste.")
	case status == 404:
		doctorLine(env, "HATA", check, "Relay API yolu bulunamadı; URL'yi, ters proxy yolunu ve sunucu sürümünü kontrol et.")
	case status == 429:
		doctorLine(env, "HATA", check, "İstek veya eşzamanlı bağlantı sınırına ulaşıldı; biraz bekleyip yeniden dene.")
		return ExitUnreachable
	case status >= 500:
		doctorLine(env, "HATA", check, "Sunucu veya ters proxy geçici hata döndürdü; hizmetin sağlığını kontrol et.")
		return ExitUnreachable
	default:
		doctorLine(env, "HATA", check, "Beklenen relay yanıtı alınamadı; URL'yi ve sunucu sürümünü kontrol et.")
	}
	return ExitError
}

func runDoctor(parent context.Context, args []string, env Env, path string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	timeout := fs.Duration("timeout", 10*time.Second, "tüm ağ kontrolleri için süre")
	pos, err := flagx.Parse(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(env.Stdout, doctorUsage)
		return ExitOK
	}
	if err != nil || len(pos) != 0 || *timeout <= 0 || *timeout > 2*time.Minute {
		fmt.Fprintln(env.Stderr, doctorUsage+" (süre 0'dan büyük, en çok 2m)")
		return ExitError
	}
	if parent.Err() != nil {
		doctorLine(env, "HATA", "İPTAL", "Teşhis iptal edildi.")
		return ExitCanceled
	}
	if path == "" {
		path, err = DefaultConfigPath()
		if err != nil {
			doctorLine(env, "HATA", "AYAR", "Varsayılan ayar konumu bulunamadı; --config ile dosya seç.")
			return ExitError
		}
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		doctorLine(env, "HATA", "AYAR", "Ayar dosyası yok; önce relay setup ile bağlan veya --config ile dosya seç.")
		return ExitError
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > doctorMaxBytes {
		doctorLine(env, "HATA", "AYAR", "Ayar dosyası okunamıyor, normal dosya değil veya boyutu sınırı aşıyor.")
		return ExitError
	}
	cfg, err := LoadConfig(path)
	if err != nil || !setupTokenRe.MatchString(cfg.Token) {
		doctorLine(env, "HATA", "AYAR", "JSON ve url/room/agent/token alanlarını kontrol et; HTTPS gerekir, HTTP yalnız yerel adreslerde geçerlidir. Gerekirse relay setup kullan.")
		return ExitError
	}
	doctorLine(env, "OK", "AYAR", "Ayar biçimi ve zorunlu alanlar geçerli.")
	localOK := true
	// doctor dosya taşımaz; henüz taşınmamış eski adlı imleç de raporlanır.
	statePath := cfg.statePath
	if _, err := os.Lstat(statePath); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Lstat(cfg.legacyStatePath); err == nil {
			statePath = cfg.legacyStatePath
		}
	}
	state, stateErr := os.Stat(statePath)
	if errors.Is(stateErr, os.ErrNotExist) {
		doctorLine(env, "BİLGİ", "İMLEÇ", "Henüz okuma imleci yok; bu durum yeni kurulumda normaldir.")
	} else {
		if stateErr == nil && state.Mode().IsRegular() && state.Size() <= doctorMaxBytes {
			_, stateErr = (cursor{path: statePath}).Read()
		} else {
			stateErr = errors.New("imleç okunamıyor")
		}
		localOK = stateErr == nil
		if localOK {
			doctorLine(env, "OK", "İMLEÇ", "Okuma imleci geçerli; değiştirilmedi.")
		} else {
			doctorLine(env, "HATA", "İMLEÇ", "Okuma imleci bozuk veya okunamıyor; yedekleyip incele. Sıfırlanırsa eski mesajlar yeniden gelir.")
		}
	}
	ctx, cancel := context.WithTimeout(parent, *timeout)
	defer cancel()
	a := newAPI(cfg)
	defer a.hc.CloseIdleConnections()
	trace := &doctorTrace{}
	var health struct {
		OK bool `json:"ok"`
	}
	status, err := doctorGet(httptrace.WithClientTrace(ctx, trace.trace()), a.hc, cfg, "/v1/health", false, &health)
	if status == 0 {
		return trace.networkError(ctx, env, err)
	}
	u, _ := url.Parse(cfg.URL)
	if net.ParseIP(u.Hostname()) != nil {
		doctorLine(env, "OK", "DNS", "Sayısal adres kullanılıyor; DNS çözümlemesi gerekmiyor.")
	} else {
		doctorLine(env, "OK", "DNS", "Sunucu adı çözümlendi.")
	}
	doctorLine(env, "OK", "BAĞLANTI", "Sunucuya bağlantı kuruldu.")
	if u.Scheme == "https" {
		doctorLine(env, "OK", "TLS", "Sertifika ve sunucu adı doğrulandı.")
	} else {
		doctorLine(env, "BİLGİ", "TLS", "Yerel HTTP bağlantısı; TLS kullanılmıyor.")
	}
	if status != http.StatusOK {
		return doctorHTTP(env, "SAĞLIK", status)
	}
	if err != nil || !health.OK {
		doctorLine(env, "HATA", "SAĞLIK", "Geçerli relay sağlık yanıtı alınamadı; API adresini ve ters proxy ayarını kontrol et.")
		return ExitError
	}
	doctorLine(env, "OK", "SAĞLIK", "Relay hizmeti yanıt veriyor.")
	var me struct {
		Name, Kind string
		Rooms      []string
	}
	status, err = doctorGet(ctx, a.hc, cfg, "/v1/me", true, &me)
	if status == 0 {
		return trace.networkError(ctx, env, err)
	}
	if status != http.StatusOK {
		return doctorHTTP(env, "KİMLİK", status)
	}
	if err != nil || me.Name == "" || (me.Kind != "ajan" && me.Kind != "insan") {
		doctorLine(env, "HATA", "KİMLİK", "Kimlik yanıtı geçersiz; sunucu sürümünü kontrol et.")
		return ExitError
	}
	if me.Name != cfg.Agent {
		doctorLine(env, "HATA", "KİMLİK", "Token başka bir hesaba ait; relay setup sırasında doğru hesap adı ve token'ı birlikte kullan.")
		return ExitError
	}
	doctorLine(env, "OK", "KİMLİK", "Token doğrulandı ve ayardaki hesapla eşleşti.")
	member := false
	for _, room := range me.Rooms {
		member = member || room == cfg.Room
	}
	if !member {
		doctorLine(env, "HATA", "ODA", "Hesap ayardaki odaya üye değil; oda ayarını veya yönetici tarafından verilen üyeliği kontrol et.")
		return ExitError
	}
	var agents struct {
		Agents []agentView `json:"agents"`
	}
	status, err = doctorGet(ctx, a.hc, cfg, "/v1/rooms/"+url.PathEscape(cfg.Room)+"/agents", true, &agents)
	if status == 0 {
		return trace.networkError(ctx, env, err)
	}
	if status != http.StatusOK {
		return doctorHTTP(env, "ODA", status)
	}
	if err != nil || agents.Agents == nil {
		doctorLine(env, "HATA", "ODA", "Katılımcı yanıtı geçersiz; sunucu sürümünü kontrol et.")
		return ExitError
	}
	doctorLine(env, "OK", "ODA", "Oda üyeliği ve katılımcı erişimi doğrulandı.")
	if !localOK {
		return ExitError
	}
	doctorLine(env, "OK", "SONUÇ", "Bağlantı kontrolleri geçti. Mesajlar, ayarlar ve okuma imleci değiştirilmedi.")
	return ExitOK
}
