#!/usr/bin/env bash
# Yalnız kardeş betikler kaynak alır; ortam değişkeni okumaz/yazdırmaz.

hata() { printf 'hata: %s\n' "$*" >&2; exit 1; }
deger_iste() { [[ $# -ge 2 && -n $2 && $2 != --* ]] || hata "$1 için değer gerekli"; }
komut_iste() { command -v "$1" >/dev/null 2>&1 || hata "gerekli komut yok: $1"; }

kok_kontrol() {
    # Test kökü yalnız okuma içindir; sistem komutları ve --uygula kapalıdır.
    if [[ -n $kok ]]; then
        [[ $kok == /* && -d $kok ]] || hata '--kok var olan mutlak bir dizin olmalı'
        kok=$(cd -- "$kok" && pwd -P)
        [[ $kok != / ]] || hata '--kok / olamaz'
        (( uygula == 0 )) || hata '--kok ile --uygula yasak'
    else
        [[ $(id -u) == 0 ]] || hata 'root gerekli (sudo ile çalıştırın)'
    fi
}

sistem_kontrol() {
    local surum="$kok/etc/os-release" pid="$kok/proc/1/comm"
    [[ -f $surum ]] || hata 'Ubuntu sürüm dosyası yok'
    # os-release kabukta kaynak alınmaz; yalnız sabit alanları karşılaştır.
    grep -Eq '^ID="?ubuntu"?$' "$surum" || hata 'yalnız Ubuntu desteklenir'
    grep -Eq '^VERSION_ID="?24\.04"?$' "$surum" || hata 'Ubuntu 24.04 gerekli'
    [[ -d $kok/run/systemd/system && -f $pid ]] || hata 'systemd çalışmıyor'
    [[ $(cat -- "$pid") == systemd ]] || hata 'PID 1 systemd değil'
    if [[ -z $kok ]]; then
        komut_iste systemctl
    fi
}

port_kontrol() {
    local dinleyen
    if [[ -n $kok ]]; then
        [[ -f $kok/test/ss-ltn ]] || hata 'test ss çıktısı yok'
        dinleyen=$(cat -- "$kok/test/ss-ltn")
    else
        komut_iste ss
        dinleyen=$(ss -H -ltn)
    fi
    # Wildcard ve IPv6 dinleyicileri de çakışabilir; tüm adreslerde aynı portu reddet.
    if awk -v p="$port" '$4 ~ (":" p "$") {found=1} END {exit !found}' <<< "$dinleyen"; then
        hata "port $port kullanımda; mevcut servisi inceleme olmadan durdurmayın"
    fi
}

bellek_kontrol() {
    local kb
    [[ -f $kok/proc/meminfo ]] || hata 'MemAvailable okunamadı'
    kb=$(awk '$1=="MemAvailable:" {print $2}' "$kok/proc/meminfo")
    [[ $kb =~ ^[0-9]+$ ]] || hata 'MemAvailable geçersiz'
    (( kb >= 300 * 1024 )) || hata 'MemAvailable en az 300 MiB olmalı'
}

saglik() {
    # Proxy kullanma, yönlendirme izleme, TLS doğrulamasını kapatma.
    local kod
    kod=$(curl --noproxy '*' --proxy '' --connect-timeout 1 --max-time 1 \
        --silent --output /dev/null --write-out '%{http_code}' "$1") || return 1
    [[ $kod == 200 ]]
}

saglik_bekle() {
    local sure=$1 son url tamam
    shift
    son=$((SECONDS + sure))
    while (( SECONDS < son )); do
        tamam=1
        for url in "$@"; do
            (( SECONDS < son )) || return 1
            if ! saglik "$url"; then tamam=0; fi
        done
        if (( tamam && SECONDS <= son )); then return 0; fi
        (( SECONDS < son )) && sleep 1
    done
    return 1
}

kilit_al() {
    komut_iste flock
    # Bütün değişiklik betikleri aynı kilidi tutar; kuru çalışma dosya açmaz.
    exec 9>/run/lock/agent-relay-deploy.lock
    flock -n 9 || hata 'başka bir relay değişikliği devam ediyor'
}
