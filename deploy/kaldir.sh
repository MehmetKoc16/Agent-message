#!/usr/bin/env bash
set -euo pipefail
burada=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=ortak.sh
source "$burada/ortak.sh"
kok= uygula=0 veri_sil=0
while (($#)); do
    case $1 in
        --uygula) uygula=1; shift ;;
        --veri-sil) veri_sil=1; shift ;;
        --help|-h)
            printf '%s\n' 'sudo bash deploy/kaldir.sh [--uygula] [--veri-sil] (varsayılan kuru çalışma)'; exit 0 ;;
        *) hata "bilinmeyen argüman: $1" ;;
    esac
done
kok_kontrol
sistem_kontrol
printf '%s\n' 'Plan: stop/disable agent-relay.socket ve agent-relay.service; birimler/drop-in kaldır; daemon-reload; /opt/agent-relay kaldır.'
if (( veri_sil )); then
    printf '%s\n' 'UYARI: --veri-sil /var/lib/agent-relay ve /etc/agent-relay içeriğini kalıcı siler.'
else
    printf '%s\n' 'Veri ve config korunacak. agentrelay hesabı korunacak.'
fi
printf '%s\n' 'Caddy bloğu elle bildirilir: caddy-ekle.sh --geri-al <yedek-dosyası> önce kuru, sonra --uygula.' \
    'Yedek sonrası başka Caddy değişikliği varsa tüm dosyayı geri almayın; sunucu sahibi yalnız relay bloğunu çıkarmalı.'
(( uygula )) || { printf '%s\n' 'KURU ÇALIŞMA: hiçbir şey değiştirilmedi.'; exit 0; }
kilit_al
# Sabit hedeflerde bile sembolik bağlantı veya bağlama noktası varsa otomatik silme yapma.
hedefler=(/opt/agent-relay /etc/systemd/system/agent-relay.service.d)
if (( veri_sil )); then hedefler+=(/var/lib/agent-relay /etc/agent-relay); fi
komut_iste mountpoint
for hedef in "${hedefler[@]}" /etc/systemd/system/agent-relay.service /etc/systemd/system/agent-relay.socket; do
    [[ ! -L $hedef ]] || hata 'kaldırma hedefi sembolik bağlantı; elle inceleyin'
    if mountpoint -q -- "$hedef"; then hata 'kaldırma hedefi bağlama noktası; elle inceleyin'; fi
done
if systemctl cat agent-relay.socket >/dev/null 2>&1; then
    systemctl stop agent-relay.socket
    systemctl disable agent-relay.socket
fi
if systemctl cat agent-relay.service >/dev/null 2>&1; then
    systemctl stop agent-relay
    systemctl disable agent-relay
fi
rm -f -- /etc/systemd/system/agent-relay.service /etc/systemd/system/agent-relay.socket
rm -rf --one-file-system -- /etc/systemd/system/agent-relay.service.d
systemctl daemon-reload
rm -rf --one-file-system -- /opt/agent-relay
if (( veri_sil )); then
    rm -rf --one-file-system -- /var/lib/agent-relay /etc/agent-relay
fi
printf '%s\n' 'Relay kaldırıldı. Caddy ve DNS geri alma adımını sunucu sahibiyle tamamlayın.'
