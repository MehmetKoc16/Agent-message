#!/usr/bin/env bash
set -euo pipefail
burada=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=ortak.sh
source "$burada/ortak.sh"
blok= alan= mevcut_saglik= geri= kok= uygula=0
kullanim() {
    printf '%s\n' 'sudo bash deploy/caddy-ekle.sh --blok <Caddyfile.relay> --alan relay.example.com --mevcut-saglik https://api.example.com/saglik [--uygula]' \
        'sudo bash deploy/caddy-ekle.sh --geri-al <yedek-dosyası> [--uygula]' \
        '--kok <dizin>: yalnız test; --uygula ile kullanılamaz.'
}
while (($#)); do
    case $1 in
        --blok) deger_iste "$@"; blok=$2; shift 2 ;;
        --alan) deger_iste "$@"; alan=$2; shift 2 ;;
        --mevcut-saglik) deger_iste "$@"; mevcut_saglik=$2; shift 2 ;;
        --geri-al) deger_iste "$@"; geri=$2; shift 2 ;;
        --kok) deger_iste "$@"; kok=$2; shift 2 ;;
        --uygula) uygula=1; shift ;;
        --help|-h) kullanim; exit 0 ;;
        *) hata "bilinmeyen argüman: $1" ;;
    esac
done
if [[ -n $geri ]]; then
    [[ -z $blok && -z $alan && -z $mevcut_saglik ]] || hata '--geri-al diğer mod argümanlarıyla kullanılamaz'
else
    [[ -n $blok && -n $alan && -n $mevcut_saglik ]] || hata '--blok, --alan ve --mevcut-saglik zorunlu'
    # Çözümlemeden önce alanı doğrula; seçenek/metin DNS aracına taşınmasın.
    etiket='[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?'
    dns="^$etiket(\\.$etiket)+$"
    [[ ${#alan} -le 253 && $alan =~ $dns ]] || hata 'geçerli küçük harfli DNS alanı gerekli'
fi
dns_kontrol() {
    # DNS sorgusu kuru çalışmada da yapılır; hiçbir dosya ya da servis değişmez.
    komut_iste getent
    komut_iste ip
    local adresler arayuzler
    adresler=$(getent ahosts "$alan" | awk '{print $1}' | sort -u) || adresler=
    arayuzler=$(ip -o addr | awk '{sub(/\/.*/, "", $4); print $4}') || arayuzler=
    if ! awk 'NR==FNR {a[$0]=1; next} NF && a[$0] {ok=1} END {exit !ok}' \
        <(printf '%s\n' "$adresler") <(printf '%s\n' "$arayuzler"); then
        if (( uygula )); then hata 'DNS kaydı henüz bu sunucuyu göstermiyor'; fi
        printf '%s\n' 'UYARI: DNS kaydı henüz bu sunucuyu göstermiyor; uygulamadan önce düzeltin.' >&2
    fi
}
if [[ -z $geri ]]; then dns_kontrol; fi
kok_kontrol
sistem_kontrol
komut_iste diff
caddyfile="$kok/etc/caddy/Caddyfile"
[[ -f $caddyfile && ! -L $caddyfile ]] || hata 'normal /etc/caddy/Caddyfile gerekli'
if [[ -n $geri ]]; then
    [[ $geri == "$caddyfile".yedek-* && -f $geri && ! -L $geri ]] || hata 'yalnız Caddyfile.yedek-* normal yedeği kabul edilir'
else
    [[ -f $blok && -r $blok ]] || hata 'blok okunamıyor'
    # Kabuk veya Caddy sözdizimi enjekte edebilen alan/URL değerlerini reddet.
    [[ $mevcut_saglik == https://* && $mevcut_saglik != *[[:space:]@\?#]* ]] || hata 'sağlık URL HTTPS olmalı; kimlik bilgisi/sorgu içeremez'
    saglik_host=${mevcut_saglik#https://}
    saglik_host=${saglik_host%%/*}
    saglik_host=${saglik_host%:443}
    [[ ${#saglik_host} -le 253 && $saglik_host =~ $dns ]] || hata 'sağlık URL için DNS alanı ve standart HTTPS portu gerekli'
    # Tek site bloğunu denetle; asıl sözdizimini uygulamada Caddy doğrular.
    awk -v alan="$alan" '
        {
            gsub(/"([^"\\]|\\.)*"/, "\"\"")
            sub(/#.*/, "")
            if ($0 ~ /^[[:space:]]*$/) next
            if (!basladi) {
                satir=$0; sub(/^[[:space:]]*/, "", satir)
                if (substr(satir,1,length(alan)) != alan) exit 1
                kalan=substr(satir,length(alan)+1)
                if (kalan !~ /^[[:space:]]+\{[[:space:]]*$/) exit 1
                basladi=1; $0="{"
            }
            for (i=1; i<=length($0); i++) {
                c=substr($0,i,1)
                if (bitti && c !~ /[[:space:]]/) exit 1
                if (c=="{") derinlik++
                if (c=="}") { derinlik--; if(derinlik==0) bitti=1 }
                if (derinlik<0) exit 1
            }
        }
        END { if (!basladi || !bitti || derinlik!=0) exit 1 }
    ' "$blok" || hata 'blok yalnız --alan için dengeli tek site bloğu içermeli'
fi
tekrar_kontrol() {
    # Alan, Caddyfile'da site adresi olarak geçiyorsa dur: tek başına, virgüllü/çok satırlı
    # liste içinde, http(s):// önekiyle ya da :port ekiyle. Yorumlar ve başka bir alanın
    # alt dizesi (xrelay.example.com) eşleşmez: serbest metin araması değil belirteç karşılaştırması.
    local sonuc=0
    awk -v alan="$alan" '
        BEGIN { alan=tolower(alan) }
        {
            satir=$0
            gsub(/"([^"\\]|\\.)*"/, "\"\"", satir)
            sub(/(^|[[:space:]])#.*/, "", satir)
            if (derinlik==0) {
                n=split(satir, t, /[[:space:],{}]+/)
                for (i=1; i<=n; i++) {
                    a=tolower(t[i])
                    sub(/^https?:\/\//, "", a)
                    sub(/:[0-9]+$/, "", a)
                    if (a!="" && a==alan) bulundu=1
                }
            }
            tmp=satir
            derinlik+=gsub(/\{/, "", tmp)
            derinlik-=gsub(/\}/, "", tmp)
            if (derinlik<0) derinlik=0
        }
        END { exit bulundu ? 0 : 1 }
    ' "$caddyfile" || sonuc=$?
    (( sonuc != 0 )) || hata 'alan zaten mevcut Caddyfile içinde; ekleme yapılmadı'
    (( sonuc == 1 )) || hata 'Caddyfile alan taraması başarısız'
}
aday() {
    if [[ -n $geri ]]; then cat -- "$geri"; else
        cat -- "$caddyfile"
        printf '\n'
        cat -- "$blok"
        printf '\n'
    fi
}
if [[ -z $geri ]]; then tekrar_kontrol; fi
printf '%s\n' 'Önerilen Caddyfile farkı:'
fark_kodu=0
diff -u --label /etc/caddy/Caddyfile --label /etc/caddy/Caddyfile.onerilen \
    "$caddyfile" <(aday) || fark_kodu=$?
(( fark_kodu <= 1 )) || hata 'diff başarısız'
if (( ! uygula )); then
    printf '%s\n' 'KURU ÇALIŞMA: dosya/yedek yazılmadı; DNS dışında ağ isteği veya reload yapılmadı.' \
        'Uygulama: zaman damgalı yedek → ekle/geri koy → validate → reload; restart yok.'
    exit 0
fi
for arac in caddy curl cp date flock; do komut_iste "$arac"; done
kilit_al
if [[ -z $geri ]]; then
    tekrar_kontrol
    saglik "$mevcut_saglik" || hata 'mevcut uygulama ön sağlık kontrolü 200 değil; dosya değiştirilmedi'
else
    # Geri alma kaynağı da gerçek Caddy ile, değiştirmeden önce doğrulanır.
    caddy validate --config "$geri" --adapter caddyfile
fi
# Mevcut yapılandırmayı da doğrula; bozuk bir başlangıcı yedekleyip yayınlama.
caddy validate --config "$caddyfile" --adapter caddyfile
yedek="$caddyfile.yedek-$(date -u +%Y%m%dT%H%M%S-%N)"
[[ ! -e $yedek ]] || hata 'yedek adı çakıştı'
cp -p -- "$caddyfile" "$yedek"
printf 'Yedek: %s\n' "$yedek"
geri_don() {
    trap - ERR INT TERM
    printf '%s\n' 'hata: Caddy değişikliği geri alınıyor.' >&2
    if cp -p -- "$yedek" "$caddyfile" && \
        caddy validate --config "$caddyfile" --adapter caddyfile && systemctl reload caddy; then
        printf '%s\n' 'Önceki Caddyfile geri kondu ve reload edildi.' >&2
    else
        printf 'ACİL: otomatik geri alma tamamlanamadı; sunucu sahibi incelemeli. Yedek: %s\n' "$yedek" >&2
    fi
    exit 1
}
trap geri_don ERR INT TERM
if [[ -n $geri ]]; then
    cp -p -- "$geri" "$caddyfile"
else
    # Yalnız sona ekle; mevcut baytlar değişmez.
    printf '\n' >> "$caddyfile"
    cat -- "$blok" >> "$caddyfile"
    printf '\n' >> "$caddyfile"
fi
caddy validate --config "$caddyfile" --adapter caddyfile
systemctl reload caddy
if [[ -z $geri ]]; then
    saglik_bekle 15 "$mevcut_saglik"
    # İlk ACME sertifikası zaman alabilir; relay gecikmesi canlı siteyi geri aldırmaz.
    if ! saglik_bekle 90 "https://$alan/v1/health"; then
        trap - ERR INT TERM
        printf '%s\n' 'UYARI: kısmi başarı; mevcut uygulama sağlıklı, relay HTTPS 90 sn içinde 200 olmadı. GERİ ALINMADI.' \
            'DNS kaydını, ACME/sertifika loglarını (journalctl -u caddy) ve /run/agent-relay/relay.sock izinlerini inceleyin.' >&2
        exit 3
    fi
fi
trap - ERR INT TERM
printf '%s\n' 'Caddy işlemi tamamlandı; restart kullanılmadı.'
if [[ -n $geri ]]; then
    printf '%s\n' 'Geri alma sonrası mevcut uygulamanın sağlığını ayrıca doğrulayın.'
fi
