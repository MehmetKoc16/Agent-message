#!/usr/bin/env bash
# Yalnız kardeş betikler kaynak alır; ortam değişkeni okumaz/yazdırmaz.

hata() { printf 'hata: %s\n' "$*" >&2; exit 1; }
deger_iste() { [[ $# -ge 2 && -n $2 && $2 != --* ]] || hata "$1 için değer gerekli"; }
komut_iste() { command -v "$1" >/dev/null 2>&1 || hata "gerekli komut yok: $1"; }

# Örnek blokla birlikte sabit tutulur; farklı log hedefleri otomatik hazırlanmaz.
caddy_log_yolu=/var/log/caddy/relay.log
caddy_log_kontrol() {
    # Salt okunur: test kökü ve sayısal kimliklerle de sınanabilir.
    local hedef=$1 uid=$2 gid=$3 dizin parca bilgi
    dizin=${hedef%/*}
    parca=$dizin
    while [[ $parca != / && -n $parca ]]; do
        [[ ! -L $parca ]] || hata 'Caddy log yolunda sembolik bağlantı var'
        [[ $parca == */* ]] || break
        parca=${parca%/*}
    done
    [[ -d $dizin ]] || hata 'Caddy log dizini yok'
    [[ $(stat -c '%u' -- "$dizin") == "$uid" ]] || hata 'Caddy log dizini sahibi caddy değil'
    [[ ! -L $hedef ]] || hata 'Caddy log dosyası sembolik bağlantı'
    if [[ -e $hedef ]]; then
        [[ -f $hedef ]] || hata 'Caddy log hedefi normal dosya değil'
        bilgi=$(stat -c '%u:%g:%a:%h' -- "$hedef") || hata 'Caddy log bilgisi okunamadı'
        [[ $bilgi == "$uid:$gid:600:1" ]] || hata 'Caddy log dosyası caddy:caddy 0600 ve tek bağlantılı olmalı; dosyaya dokunulmadı'
    fi
}

caddy_log_hazirla() (
    # Yalnız canlı --uygula çağırır. Dizin fd'si hedefi sabitler; link mevcut
    # adı atomik olarak reddeder. Root hiçbir zaman mevcut loga yazmaz.
    local hedef=$1 uid=$2 gid=$3
    caddy_log_kontrol "$hedef" "$uid" "$gid"
    # openat/O_NOFOLLOW: her dizin bileşeni sabitlenir. Hazırlama aynı dosya
    # sistemindedir; /tmp ayrı bir mount olsa da atomik link çalışır.
    python3 - "$hedef" "$uid" "$gid" <<'PY'
import os, secrets, stat, subprocess, sys

target, uid, gid = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW
directory = os.open('/', flags)
private = None
name = None
verified = False
try:
    for part in target.split('/')[1:-1]:
        child = os.open(part, flags, dir_fd=directory)
        os.close(directory)
        directory = child
    if os.fstat(directory).st_uid != uid:
        raise RuntimeError('Caddy log dizini sahibi caddy değil')
    leaf = target.rsplit('/', 1)[1]
    try:
        info = os.stat(leaf, dir_fd=directory, follow_symlinks=False)
    except FileNotFoundError:
        info = None
    if info is not None:
        if not (stat.S_ISREG(info.st_mode) and info.st_uid == uid and
                info.st_gid == gid and stat.S_IMODE(info.st_mode) == 0o600 and info.st_nlink == 1):
            raise RuntimeError('Caddy log hedefi değişti/geçersiz; dosyaya dokunulmadı')
    else:
        name = '.agent-relay-log-' + secrets.token_hex(16)
        os.mkdir(name, 0o700, dir_fd=directory)
        private = os.open(name, flags, dir_fd=directory)
        info = os.fstat(private)
        if info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o700 or os.listdir(private):
            raise RuntimeError('özel log hazırlama dizini değişti')
        verified = True
        subprocess.run(['install', '-o', 'caddy', '-g', 'caddy', '-m', '0600',
                        '/dev/null', f'/proc/self/fd/{private}/relay.log'],
                       pass_fds=(private,), check=True)
        # EEXIST: yarışta beliren dosya/symlink ASLA ezilmez veya izlenmez.
        os.link('relay.log', leaf, src_dir_fd=private, dst_dir_fd=directory,
                follow_symlinks=False)
except (OSError, RuntimeError, subprocess.CalledProcessError) as exc:
    print(f'hata: Caddy log hazırlığı başarısız; mevcut hedefe yazılmadı: {exc}', file=sys.stderr)
    sys.exit(1)
finally:
    if private is not None:
        # Yalnız doğrulanmış root dizininin içeriğini temizle.
        owned = os.fstat(private)
        if verified:
            try:
                os.unlink('relay.log', dir_fd=private)
            except FileNotFoundError:
                pass
            try:
                entry = os.stat(name, dir_fd=directory, follow_symlinks=False)
                if (entry.st_dev, entry.st_ino) == (owned.st_dev, owned.st_ino):
                    os.rmdir(name, dir_fd=directory)
            except FileNotFoundError:
                pass
        os.close(private)
    os.close(directory)
PY
)

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

soket_kontrol() {
    komut_iste getent
    getent group caddy >/dev/null || hata 'caddy grubu yok; önce Caddy kurulumunu doğrulayın'
    local hedef="$kok/run/agent-relay/relay.sock"
    [[ ! -L $kok/run/agent-relay && ! -L $hedef ]] || hata 'soket hedefinde sembolik bağlantı var'
    if [[ -e $hedef && ! -S $hedef ]]; then hata 'soket hedefi var ve soket değil'; fi
}

soket_saglik() {
    if command -v curl >/dev/null 2>&1; then
        local kod
        kod=$(curl --noproxy '*' --proxy '' --unix-socket /run/agent-relay/relay.sock \
            --max-time 1 --silent --output /dev/null --write-out '%{http_code}' http://localhost/v1/health) || return 1
        [[ $kod == 200 ]]
    else
        python3 - <<'PY'
import socket, http.client
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.settimeout(1)
try:
    s.connect('/run/agent-relay/relay.sock')
    s.sendall(b'GET /v1/health HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
    response = http.client.HTTPResponse(s)
    response.begin()
    raise SystemExit(0 if response.status == 200 else 1)
finally:
    s.close()
PY
    fi
}

soket_saglik_bekle() {
    local son=$((SECONDS + 10))
    while (( SECONDS < son )); do
        if soket_saglik; then return 0; fi
        sleep 1
    done
    return 1
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
