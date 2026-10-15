#!/usr/bin/env bash
set -euo pipefail
burada=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=ortak.sh
source "$burada/ortak.sh"
ikili= ozet= kok= port=8790 uygula=0 drop_onay=0
kullanim() {
    printf '%s\n' 'sudo bash deploy/kurulum.sh --ikili <agent-relay-linux-amd64> --ozet <SHA256SUMS> [--port 8790] [--uygula] [--drop-in-yok-onay]'
    printf '%s\n' '--kok <dizin>: yalnız test; --uygula ile kullanılamaz.'
}
while (($#)); do
    case $1 in
        --ikili) deger_iste "$@"; ikili=$2; shift 2 ;;
        --ozet) deger_iste "$@"; ozet=$2; shift 2 ;;
        --port) deger_iste "$@"; port=$2; shift 2 ;;
        --kok) deger_iste "$@"; kok=$2; shift 2 ;;
        --uygula) uygula=1; shift ;;
        --drop-in-yok-onay) drop_onay=1; shift ;;
        --help|-h) kullanim; exit 0 ;;
        *) hata "bilinmeyen argüman: $1" ;;
    esac
done
[[ -n $ikili && -n $ozet ]] || hata '--ikili ve --ozet zorunlu'
[[ $port =~ ^[1-9][0-9]{0,4}$ ]] && (( port <= 65535 )) || hata 'port 1-65535 arasında olmalı'
kok_kontrol
sistem_kontrol
[[ -f $ikili && -r $ikili && -f $ozet && -r $ozet ]] || hata 'ikili veya SHA256SUMS okunamıyor'
komut_iste sha256sum
ad=$(basename -- "$ikili")
# Başka dosyanın satırını veya çift satırı kabul etme; sha256sum -c tüm manifesti çalıştırmaz.
beklenen=$(awk -v ad="$ad" 'length($1)==64 && $1 !~ /[^[:xdigit:]]/ {n=$2; sub(/^\*/, "", n); if(n==ad) {print tolower($1)}}' "$ozet")
[[ $beklenen =~ ^[a-f0-9]{64}$ ]] || hata 'SHA256SUMS içinde dosya adı için tek geçerli satır gerekli'
gercek=$(sha256sum -- "$ikili")
gercek=${gercek%% *}
[[ $gercek == "$beklenen" ]] || hata 'SHA-256 uyuşmazlığı'
port_kontrol
bellek_kontrol
config="$kok/etc/agent-relay/config.json"
drop="$kok/etc/systemd/system/agent-relay.service.d/10-yerel.conf"
if [[ -f $config ]]; then
    komut_iste python3
    # Mevcut dosyayı değiştirme; sağlık kontrolünün doğru hedefe gittiğini doğrula.
    python3 - "$config" "$port" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    c = json.load(f)
if (c.get("host", "127.0.0.1") != "127.0.0.1"
        or c.get("port", 8790) != int(sys.argv[2])
        or c.get("db") != "/var/lib/agent-relay/relay.db"):
    sys.exit("hata: mevcut config host/port/db hedefle uyuşmuyor; dosya değiştirilmedi")
PY
fi
drop_eksik=0
if [[ ! -f $drop ]] || ! grep -Eq '^[[:space:]]*InaccessiblePaths[[:space:]]*=[[:space:]]*[^#[:space:]]' "$drop"; then
    drop_eksik=1
    printf '%s\n' 'UYARI: yerel 10-yerel.conf içinde InaccessiblePaths gerekli.' >&2
fi
printf '%s\n' 'Plan: SHA-256, Ubuntu/systemd, boş port ve en az 300 MiB MemAvailable doğrulandı.' \
    '1. agentrelay sistem hesabını doğrula/oluştur (kabuksuz, ev dizinsiz, ek grup yok).' \
    '2. /opt/agent-relay root:root 0755; eski ikiliyi agent-relay.onceki olarak koru.' \
    '3. config yoksa root:agentrelay 0640 oluştur; mevcut config içeriğine dokunma.' \
    '4. Birimi kur; yerel drop-in şartını kontrol et; systemd-analyze verify çalıştır.' \
    '5. daemon-reload; enable --now; 10 sn içinde HTTP 200 bekle.' \
    '6. Hata halinde servisi durdur, önceki ikili/birimi geri koy, servisi kapalı bırak.' \
    '7. systemd-analyze security agent-relay puanını göster. Caddyfile değiştirilmez.'
printf 'Seçilen port: %s\n' "$port"
sonraki() {
    printf '%s\n' 'Sonraki: DNS kaydını hazırlayın; caddy-ekle.sh kuru çalışma ve --uygula adımlarını izleyin.' \
        'Ajan: sudo -u agentrelay /opt/agent-relay/agent-relay admin --config /etc/agent-relay/config.json add-agent ornek-ajan --rooms ornek-proje'
}
if (( ! uygula )); then
    printf '%s\n' 'KURU ÇALIŞMA: hiçbir dosya yazılmadı veya servis değiştirilmedi.'
    if (( drop_eksik && ! drop_onay )); then
        printf '%s\n' 'Uygulama yerel drop-in olmadan duracaktır; istisna yalnız --drop-in-yok-onay ile.'
    fi
    sonraki
    exit 0
fi
(( ! drop_eksik || drop_onay )) || hata 'yerel drop-in eksik; hiçbir kurulum değişikliği yapılmadı'
for arac in install cp getent useradd id stat systemd-analyze curl uname python3; do komut_iste "$arac"; done
[[ $(uname -m) == x86_64 ]] || hata 'bu paket linux-amd64 hedefler; mimariyi inceleyin'
# Yanlış platformdaki bir dosyayı root olarak çalıştırmadan ELF başlığını doğrula.
python3 - "$ikili" <<'PY'
import sys
with open(sys.argv[1], 'rb') as f:
    h = f.read(20)
if len(h) != 20 or h[:6] != b'\x7fELF\x02\x01' or h[18:20] != b'\x3e\x00':
    sys.exit('hata: Linux amd64 ELF ikilisi gerekli')
PY
kilit_al
port_kontrol
bellek_kontrol
if getent passwd agentrelay >/dev/null; then
    kayit=$(getent passwd agentrelay)
    IFS=: read -r _ _ uid gid _ ev kabuk <<< "$kayit"
    (( uid > 0 && uid < 1000 )) || hata 'agentrelay sistem hesabı değil'
    [[ $kabuk == /usr/sbin/nologin && ! -d $ev ]] || hata 'agentrelay kabuksuz ve ev dizinsiz olmalı'
    [[ $(id -Gn agentrelay) == agentrelay && $(getent group agentrelay | cut -d: -f3) == "$gid" ]] || hata 'agentrelay grupları uygun değil'
else
    getent group agentrelay >/dev/null && hata 'yetim agentrelay grubu var; elle inceleyin'
    useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin agentrelay
fi
# Sembolik bağlantılarla başka uygulamaların dosyalarına yazılmasını reddet.
for hedef in /opt/agent-relay /opt/agent-relay/agent-relay /opt/agent-relay/agent-relay.onceki \
    /etc/agent-relay /etc/agent-relay/config.json /etc/systemd/system/agent-relay.service; do
    [[ ! -L $hedef ]] || hata 'kurulum hedefinde sembolik bağlantı var'
done
install -d -o root -g root -m 0755 /opt/agent-relay
eski=0 birim_eski=0 degisti=0 onceki_aktif=0
birim=/etc/systemd/system/agent-relay.service
islem=$(mktemp -d /opt/agent-relay/.kurulum.XXXXXX)
chmod 0700 "$islem"
geri_don() {
    trap - ERR INT TERM
    local sorun=0
    if (( degisti )); then
        if ! systemctl stop agent-relay; then
            if systemctl is-active --quiet agent-relay; then
                printf 'ACİL: relay durdurulamadı; dosyalara geri dönülmedi. İnceleme dizini: %s\n' "$islem" >&2
                exit 1
            fi
        fi
        if (( eski )); then
            cp -p -- /opt/agent-relay/agent-relay.onceki /opt/agent-relay/agent-relay || sorun=1
        else
            rm -f -- /opt/agent-relay/agent-relay || sorun=1
        fi
        if (( birim_eski )); then cp -p -- "$islem/birim" "$birim" || sorun=1; else
            if [[ -e $birim ]]; then systemctl disable agent-relay || sorun=1; fi
            rm -f -- "$birim" || sorun=1
        fi
        systemctl daemon-reload || sorun=1
        # Yükseltmede önceki sürüm çalışıyorduysa onu geri ayağa kaldır.
        if (( onceki_aktif && eski && birim_eski )); then systemctl start agent-relay || sorun=1; fi
    fi
    if (( sorun )); then
        printf 'ACİL: geri alma tam değil; sunucu sahibi incelemeli. İnceleme dizini: %s\n' "$islem" >&2
        exit 1
    fi
    rm -rf -- "$islem"
    printf '%s\n' 'hata: kurulum geri alındı (önceki sürüm çalışıyorsa yeniden başlatıldı, yoksa relay kapalı). Veri/config ve yerel drop-in korundu; elle inceleyin.' >&2
    exit 1
}
trap geri_don ERR INT TERM
if systemctl is-active --quiet agent-relay; then onceki_aktif=1; fi
if [[ -f $birim ]]; then cp -p -- "$birim" "$islem/birim"; birim_eski=1; fi
if [[ -f /opt/agent-relay/agent-relay ]]; then
    cp -p -- /opt/agent-relay/agent-relay /opt/agent-relay/agent-relay.onceki
    eski=1
fi
degisti=1
install -o root -g root -m 0755 -- "$ikili" /opt/agent-relay/agent-relay
if [[ ! -e $config ]]; then
    install -d -o root -g agentrelay -m 0750 /etc/agent-relay
    python3 - "$burada/config.example.json" "$port" > "$islem/config.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    c = json.load(f)
c["port"] = int(sys.argv[2])
print(json.dumps(c, indent=2))
PY
    install -o root -g agentrelay -m 0640 "$islem/config.json" "$config"
else
    [[ $(stat -c '%U:%G:%a' "$config") == root:agentrelay:640 ]] || { printf '%s\n' 'Mevcut config root:agentrelay 0640 olmalı.' >&2; false; }
fi
install -o root -g root -m 0644 "$burada/agent-relay.service" "$birim"
systemd-analyze verify "$birim"
systemctl daemon-reload
# enable --now çalışan eski sürümü yeniden başlatmaz; yükseltmede yeni ikili için restart şart (yalnız relay).
systemctl enable agent-relay
systemctl restart agent-relay
saglik_bekle 10 "http://127.0.0.1:$port/v1/health"
trap - ERR INT TERM
rm -rf -- "$islem"
printf '%s\n' 'Relay sağlık kontrolü 200; kurulum tamamlandı.'
systemd-analyze security agent-relay || true
sonraki
