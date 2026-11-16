#!/usr/bin/env bash
set -euo pipefail
burada=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
deploy=$(cd -- "$burada/.." && pwd)
gecici=$(mktemp -d "$burada/.kuru-test.XXXXXX")
temizle() {
    # Silme hedefini önce test dizinine bağla; yalnız oluşturduğumuz alt dizini kaldır.
    cd -- "$burada"
    case $gecici in "$burada"/.kuru-test.*) rm -rf -- "$gecici" ;; *) exit 1 ;; esac
}
trap temizle EXIT
kok="$gecici/kok"
cd -- "$gecici"
mkdir -p kok/etc/caddy kok/run/systemd/system kok/proc/1 kok/test girdi
mkdir -p stub kok/run/agent-relay
# NEDEN: DNS/grup ön kontrolleri gerçek ağa veya sunucu hesaplarına erişmemeli.
cat > "$gecici/stub/getent" <<'SH'
#!/usr/bin/env bash
burada=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
case "$1 $2" in
    'group caddy') [[ ! -e $burada/grup-yok ]] || exit 2; printf 'caddy:x:999:\n' ;;
    'ahosts relay.example.com') cat "$burada/dns" ;;
    *) exit 2 ;;
esac
SH
cat > "$gecici/stub/ip" <<'SH'
#!/usr/bin/env bash
printf '1: lo inet6 ::1/128 scope host\n'
SH
printf '::1 STREAM relay.example.com\n' > "$gecici/stub/dns"
chmod +x "$gecici/stub/getent" "$gecici/stub/ip"
# Sabit PATH: test ortamının mevcut değişken değerini okumaz.
export PATH="$gecici/stub:/usr/bin:/bin"
printf 'ID=ubuntu\nVERSION_ID="24.04"\n' > "$kok/etc/os-release"
printf 'systemd\n' > "$kok/proc/1/comm"
printf 'MemAvailable: 400000 kB\n' > "$kok/proc/meminfo"
: > "$kok/test/ss-ltn"
printf 'api.example.com {\n    respond 200\n}\n' > "$kok/etc/caddy/Caddyfile"
printf 'sahte-ikili-test-icin-calistirilmaz\n' > "$gecici/girdi/agent-relay-linux-amd64"
(cd -- "$gecici/girdi" && sha256sum agent-relay-linux-amd64) > "$gecici/girdi/SHA256SUMS"
cp -- "$deploy/Caddyfile.relay.ornek" "$gecici/girdi/Caddyfile.relay"
cp -- "$kok/etc/caddy/Caddyfile" "$kok/etc/caddy/Caddyfile.yedek-test"
kur=(bash "$deploy/kurulum.sh" --kok "$kok" --ikili "$gecici/girdi/agent-relay-linux-amd64" --ozet "$gecici/girdi/SHA256SUMS")
cad=(bash "$deploy/caddy-ekle.sh" --kok "$kok" --blok "$gecici/girdi/Caddyfile.relay" --alan relay.example.com --mevcut-saglik https://api.example.com/saglik)
anlik() {
    find "$gecici" -printf '%y %P %m %s %T@\n' | sort
    find "$gecici" -type f -print0 | sort -z | xargs -0 sha256sum
}
say=0
bekle() {
    local baslik=$1 sonuc=$2 desen=$3 once sonra cikti kod=0
    shift 3
    once=$(anlik)
    cikti=$("$@" 2>&1) || kod=$?
    sonra=$(anlik)
    [[ $once == "$sonra" ]] || { printf 'BAŞARISIZ: %s dosya değiştirdi\n' "$baslik"; exit 1; }
    if [[ $sonuc == hata ]]; then
        (( kod != 0 )) || { printf 'BAŞARISIZ: %s hata vermedi\n' "$baslik"; exit 1; }
    else
        (( kod == 0 )) || { printf 'BAŞARISIZ: %s (%s)\n%s\n' "$baslik" "$kod" "$cikti"; exit 1; }
    fi
    [[ $cikti == *"$desen"* ]] || { printf 'BAŞARISIZ: %s beklenen çıktı yok\n%s\n' "$baslik" "$cikti"; exit 1; }
    say=$((say + 1))
    printf 'OK %02d: %s; dosya değişmedi\n' "$say" "$baslik"
}
basarisiz() { printf 'BAŞARISIZ: %s\n' "$*"; exit 1; }
# Gerçek /run'daki kilit dizinine test hiçbir koşulda dokunmamalı.
gercek_kilit=/run/agent-relay-deploy
gercek_kilit_once=yok
if [[ -e $gercek_kilit || -L $gercek_kilit ]]; then gercek_kilit_once=$(find "$gercek_kilit" -printf '%y %P %m %s %T@\n' | sort); fi
bekle 'kurulum eksik argüman' hata 'zorunlu' bash "$deploy/kurulum.sh"
bekle 'kurulum eksik değer' hata 'değer gerekli' bash "$deploy/kurulum.sh" --ikili
bekle 'kurulum bilinmeyen bayrak' hata 'bilinmeyen' "${kur[@]}" --yanlis
bekle 'geçersiz port kaldırıldı' hata '--port kaldırıldı' "${kur[@]}" --port 0
bekle 'test kökünde uygulama yasak' hata '--kok ile --uygula yasak' "${kur[@]}" --uygula
bekle 'kurulum kuru ve eksik drop-in uyarısı' tamam 'KURU ÇALIŞMA' "${kur[@]}"
bekle 'farklı port artık hata' hata '--port kaldırıldı' "${kur[@]}" --port 8791
printf '%064d  agent-relay-linux-amd64\n' 0 > "$gecici/girdi/YANLIS"
bekle 'SHA-256 uyuşmazlığı' hata 'SHA-256 uyuşmazlığı' "${kur[@]}" --ozet "$gecici/girdi/YANLIS"
printf '%064d  baska-dosya\n' 0 > "$gecici/girdi/YANLIS"
bekle 'manifest dosya adı uyuşmazlığı' hata 'tek geçerli satır' "${kur[@]}" --ozet "$gecici/girdi/YANLIS"
cat "$gecici/girdi/SHA256SUMS" "$gecici/girdi/SHA256SUMS" > "$gecici/girdi/YANLIS"
bekle 'çift manifest satırı' hata 'tek geçerli satır' "${kur[@]}" --ozet "$gecici/girdi/YANLIS"
printf 'MemAvailable: 200000 kB\n' > "$kok/proc/meminfo"
bekle 'düşük RAM' hata 'en az 300 MiB' "${kur[@]}"
printf 'MemAvailable: 400000 kB\n' > "$kok/proc/meminfo"
printf 'koru\n' > "$kok/run/agent-relay/relay.sock"
bekle 'soket olmayan hedef' hata 'soket değil' "${kur[@]}"
rm -- "$kok/run/agent-relay/relay.sock"
: > "$gecici/stub/grup-yok"
bekle 'caddy grubu yoksa durur' hata 'caddy grubu yok' "${kur[@]}"
rm -- "$gecici/stub/grup-yok"
printf 'ID=ubuntu\nVERSION_ID="22.04"\n' > "$kok/etc/os-release"
bekle 'yanlış Ubuntu' hata 'Ubuntu 24.04 gerekli' "${kur[@]}"
printf 'ID=ubuntu\nVERSION_ID="24.04"\n' > "$kok/etc/os-release"
printf 'init\n' > "$kok/proc/1/comm"
bekle 'systemd olmayan PID 1' hata 'systemd değil' "${kur[@]}"
printf 'systemd\n' > "$kok/proc/1/comm"
bekle 'Caddy eksik argüman' hata 'zorunlu' bash "$deploy/caddy-ekle.sh"
bekle 'Caddy eksik değer' hata 'değer gerekli' bash "$deploy/caddy-ekle.sh" --blok
bekle 'Caddy bilinmeyen bayrak' hata 'bilinmeyen' "${cad[@]}" --yanlis
bekle 'Caddy test kökünde uygulama yasak' hata '--kok ile --uygula yasak' "${cad[@]}" --uygula
bekle 'Caddy kuru diff' tamam '+relay.example.com {' "${cad[@]}"
(cd -- "$kok" && mkdir -p var/log/caddy)
bekle 'Caddy kuru çalışma eksik log dosyasını oluşturmaz' tamam 'KURU ÇALIŞMA' "${cad[@]}"
[[ ! -e $kok/var/log/caddy/relay.log && ! -L $kok/var/log/caddy/relay.log ]] || exit 1
log_kontrol=(bash -c 'source "$1/ortak.sh"; caddy_log_kontrol "$2/var/log/caddy/relay.log" "$3" "$4"; printf "LOG KONTROL TAMAM\n"' log-test "$deploy" "$kok")
test_uid=$(stat -c '%u' -- "$kok/var/log/caddy")
test_gid=$(stat -c '%g' -- "$kok/var/log/caddy")
bekle 'log kontrolü eksik dosyaya yazmaz' tamam 'LOG KONTROL TAMAM' "${log_kontrol[@]}" "$test_uid" "$test_gid"
mkdir -- "$kok/var/log/caddy/relay.log"
bekle 'log dizin hedefini dokunmadan reddeder' hata 'normal dosya değil' "${log_kontrol[@]}" "$test_uid" "$test_gid"
rmdir -- "$kok/var/log/caddy/relay.log"
# Git Bash ln -s bazı Windows kurulumlarında kopya üretir; gerçek bağlantı
# oluşmadıysa bunu başarı saymayız, sınanamayan kontrolü açıkça bildiririz.
printf 'korunacak log hedefi\n' > "$kok/var/log/caddy/hedef"
ln -s -- hedef "$kok/var/log/caddy/relay.log"
if [[ -L $kok/var/log/caddy/relay.log ]]; then
    bekle 'log symlink hedefini dokunmadan reddeder' hata 'sembolik bağlantı' "${log_kontrol[@]}" "$test_uid" "$test_gid"
else
    printf '%s\n' 'SINANAMADI: Windows gerçek symlink oluşturmadı; symlink reddi Linux üzerinde doğrulanmalı.'
fi
rm -- "$kok/var/log/caddy/relay.log"
bekle 'log dizini yanlış sahibini dokunmadan reddeder' hata 'dizini sahibi caddy değil' "${log_kontrol[@]}" 4294967294 "$test_gid"
printf 'mevcut log korunur\n' > "$kok/var/log/caddy/relay.log"
# Sahiplik değiştirmeden stat çıktısını taklit et; Linux chown/0600 testi değildir.
bekle 'log dosyası yanlış sahiplik bilgisi reddedilir (stat taklidi)' hata 'dosyaya dokunulmadı' bash -c '
    source "$1/ortak.sh"
    stat() {
        if [[ $2 == "%u:%g:%a:%h" ]]; then printf "4294967294:4294967294:600:1\n"; else command stat "$@"; fi
    }
    caddy_log_kontrol "$2/var/log/caddy/relay.log" "$3" "$4"
' log-test "$deploy" "$kok" "$test_uid" "$test_gid"
rm -- "$kok/var/log/caddy/relay.log"
printf '%s\n' 'SINANAMADI: gerçek caddy sahipliği/0600, ayrıcalıklı install ve runuser/validate/reload Windows testinin kapsamında değil.'
# NEDEN: DNS hatasında --uygula kilit/yedek dahil hiçbir şey yazmamalı.
: > "$gecici/stub/dns"
bekle 'DNS çözülemiyor uygulama yazmaz' hata 'DNS kaydı henüz bu sunucuyu göstermiyor' "${cad[@]}" --uygula
bekle 'DNS çözülemiyor kuru uyarır' tamam 'UYARI: DNS kaydı' "${cad[@]}"
printf '127.0.0.1 STREAM relay.example.com\n' > "$gecici/stub/dns"
bekle 'DNS başka adres uygulama yazmaz' hata 'DNS kaydı henüz bu sunucuyu göstermiyor' "${cad[@]}" --uygula
printf '::1 STREAM relay.example.com\n' > "$gecici/stub/dns"
bekle 'Caddy güvensiz sağlık URL' hata 'HTTPS olmalı' "${cad[@]}" --mevcut-saglik http://api.example.com/saglik
bekle 'Caddy hatalı alan' hata 'DNS alanı' "${cad[@]}" --alan 'relay.example.com;komut'
printf '\nrelay.example.com {\n    respond 200\n}\n' >> "$kok/etc/caddy/Caddyfile"
bekle 'Caddy aynı alanı tekrar eklemez' hata 'alan zaten' "${cad[@]}"
sifirla() { cp -- "$kok/etc/caddy/Caddyfile.yedek-test" "$kok/etc/caddy/Caddyfile"; }
sifirla
printf '\nhttps://relay.example.com {\n    respond 200\n}\n' >> "$kok/etc/caddy/Caddyfile"
bekle 'Caddy https:// önekli aynı alanı reddeder' hata 'alan zaten' "${cad[@]}"
sifirla
printf '\nfoo.example.com, relay.example.com {\n    respond 200\n}\n' >> "$kok/etc/caddy/Caddyfile"
bekle 'Caddy virgüllü listedeki alanı reddeder' hata 'alan zaten' "${cad[@]}"
sifirla
printf '\nfoo.example.com,\nRelay.Example.com:443 {\n    respond 200\n}\n' >> "$kok/etc/caddy/Caddyfile"
bekle 'Caddy çok satırlı liste, büyük harf ve portu reddeder' hata 'alan zaten' "${cad[@]}"
sifirla
printf '\n# relay.example.com {\n#     respond 200\n# }\nfoo.example.com { # relay.example.com\n    respond 200\n}\n' >> "$kok/etc/caddy/Caddyfile"
bekle 'Caddy yorumdaki alan yanlış pozitif vermez' tamam '+relay.example.com {' "${cad[@]}"
sifirla
printf '\nxrelay.example.com {\n    respond 200\n}\nrelay.example.com.evil.test {\n    respond 200\n}\n' >> "$kok/etc/caddy/Caddyfile"
bekle 'Caddy alt dize alan yanlış pozitif vermez' tamam '+relay.example.com {' "${cad[@]}"
sifirla
printf '\nfoo.example.com {\n    redir https://relay.example.com{uri}\n    respond "relay.example.com"\n}\n' >> "$kok/etc/caddy/Caddyfile"
bekle 'Caddy blok gövdesindeki ad yanlış pozitif vermez' tamam '+relay.example.com {' "${cad[@]}"
sifirla
bekle 'Caddy geri alma kuru' tamam 'KURU ÇALIŞMA' bash "$deploy/caddy-ekle.sh" --kok "$kok" --geri-al "$kok/etc/caddy/Caddyfile.yedek-test"
bekle 'Caddy geri alma mod çakışması' hata 'diğer mod' "${cad[@]}" --geri-al "$kok/etc/caddy/Caddyfile.yedek-test"
printf '\napi.example.com {\n    respond 200\n}\n' >> "$gecici/girdi/Caddyfile.relay"
bekle 'Caddy birden fazla siteyi reddeder' hata 'tek site bloğu' "${cad[@]}"
# --- Dağıtım kilidi: özel dizin, sembolik bağlantı reddi, kesmeden açma ---
# kilit_al test dizini ve test kullanıcısının kimliğiyle çağrılır; gerçek /run kullanılmaz.
kilit_test="$gecici/kilit-test"
mkdir -- "$kilit_test"
kilit=(bash -c 'source "$1/ortak.sh"; kilit_al "$2" "$3"; printf "KİLİT ALINDI\n"' kilit-test "$deploy")
ben=$(id -u)
mkdir -m 0700 -- "$kilit_test/izin"
if command -v flock >/dev/null 2>&1 && [[ $(stat -c '%a' -- "$kilit_test/izin") == 700 ]]; then
    printf 'korunacak kilit hedefi\n' > "$kilit_test/hedef"
    mkdir -- "$kilit_test/hedef-dizin"
    ln -s -- hedef-dizin "$kilit_test/bagli-dizin"
    if [[ -L $kilit_test/bagli-dizin ]]; then
        bekle 'kilit dizini symlink ise reddedilir' hata 'kilit dizini sembolik bağlantı' "${kilit[@]}" "$kilit_test/bagli-dizin" "$ben"
    else
        printf '%s\n' 'SINANAMADI: gerçek symlink oluşmadı; kilit dizini symlink reddi Linux üzerinde doğrulanmalı.'
    fi
    bekle 'kilit dizini normal dosya ise reddedilir' hata 'dizin değil' "${kilit[@]}" "$kilit_test/hedef" "$ben"
    mkdir -m 0755 -- "$kilit_test/acik"
    bekle 'kilit dizini 0700 değilse reddedilir' hata 'root:0700 değil' "${kilit[@]}" "$kilit_test/acik" "$ben"
    bekle 'kilit dizini yanlış sahipliyse reddedilir' hata 'root:0700 değil' "${kilit[@]}" "$kilit_test/izin" 4294967294
    ln -s -- ../hedef "$kilit_test/izin/kilit"
    if [[ -L $kilit_test/izin/kilit ]]; then
        # bekle içerik özetini de karşılaştırır: bağlantı hedefi sıfırlanmamış olmalı.
        bekle 'kilit dosyası symlink ise hedef kesilmeden reddedilir' hata 'kilit dosyası sembolik bağlantı' "${kilit[@]}" "$kilit_test/izin" "$ben"
    else
        printf '%s\n' 'SINANAMADI: gerçek symlink oluşmadı; kilit dosyası symlink reddi Linux üzerinde doğrulanmalı.'
    fi
    rm -- "$kilit_test/izin/kilit"
    mkdir -- "$kilit_test/izin/kilit"
    bekle 'kilit dosyası dizin ise reddedilir' hata 'normal dosya değil' "${kilit[@]}" "$kilit_test/izin" "$ben"
    rmdir -- "$kilit_test/izin/kilit"
    printf 'mevcut kilit icerigi\n' > "$kilit_test/izin/kilit"
    bekle 'mevcut kilit dosyası kesilmeden alınır' tamam 'KİLİT ALINDI' "${kilit[@]}" "$kilit_test/izin" "$ben"
    # Eksik dizin 0700 oluşturulur (bu çağrı bilerek dosya yazar; bekle kullanılmaz).
    cikti=$("${kilit[@]}" "$kilit_test/yeni" "$ben" 2>&1) || basarisiz "kilit dizini oluşturulamadı: $cikti"
    [[ $cikti == *'KİLİT ALINDI'* && -d $kilit_test/yeni && ! -L $kilit_test/yeni && -f $kilit_test/yeni/kilit ]] || basarisiz 'kilit dizini/dosyası beklenen biçimde oluşmadı'
    [[ $(stat -c '%u:%a' -- "$kilit_test/yeni") == "$ben:700" ]] || basarisiz 'yeni kilit dizini 0700 değil'
    say=$((say + 1)); printf 'OK %02d: eksik kilit dizini 0700 oluşturulur\n' "$say"
    # Kilit tutulurken ikinci çağıran (üç betik aynı kilit_al yolunu kullanır) reddedilir.
    (
        exec 8>>"$kilit_test/yeni/kilit"
        flock -n 8 || exit 1
        "${kilit[@]}" "$kilit_test/yeni" "$ben" > "$kilit_test/ikinci" 2>&1 && exit 1
        grep -q 'başka bir relay değişikliği devam ediyor' "$kilit_test/ikinci"
    ) || basarisiz 'tutulan kilit ikinci çağıranı engellemedi'
    say=$((say + 1)); printf 'OK %02d: tutulan kilit ikinci çağıranı reddeder\n' "$say"
else
    printf '%s\n' 'SINANAMADI: flock yok veya dosya sistemi 0700 izinlerini saklamıyor; kilit denetimleri Linux üzerinde doğrulanmalı.'
fi
# Üç betik de paylaşılan kilit_al'ı argümansız çağırmalı; eski /run/lock yolu kalmamalı.
for betik in kurulum.sh caddy-ekle.sh kaldir.sh; do
    grep -qx 'kilit_al' "$deploy/$betik" || basarisiz "$betik paylaşılan kilit_al çağırmıyor"
done
if grep -Eq 'agent-relay-deploy\.lock|>[[:space:]]*/run/lock' "$deploy"/*.sh; then basarisiz 'betiklerde /run/lock kilidi kaldı'; fi
say=$((say + 1)); printf 'OK %02d: üç betik aynı özel kilidi paylaşır; /run/lock kullanılmaz\n' "$say"

# --- kurulum.sh geri_don: önceki sürüm sağlığı doğrulanmadan "geri alındı" denmez ---
# Fonksiyon betikten çıkarılır ve sahte systemctl/cp/rm ile gerçek ERR trap + set -e
# bağlamında çalıştırılır. PATH boş bir dizindir: sahte fonksiyon eksik kalsa bile
# gerçek rm/cp/systemctl bulunamaz.
geri_govde=$(awk '/^geri_don\(\) \{$/ {ic=1} ic {print} ic && /^}$/ {exit}' "$deploy/kurulum.sh")
[[ $geri_govde == 'geri_don() {'*soket_saglik_bekle*$'\n}' ]] || basarisiz 'kurulum.sh geri_don çıkarılamadı veya sağlık beklemesi içermiyor'
mkdir -- "$gecici/bos-yol"
geri_calistir() {
    # $1: sağlık sonucu (0/1), $2: onceki_aktif, $3: systemctl start sonucu (0/1)
    BOS_YOL="$gecici/bos-yol" bash -c '
        set -euo pipefail
        PATH=$BOS_YOL
        systemctl() { printf "systemctl %s\n" "$*"; if [[ $1 == start && $2 == agent-relay ]]; then return "$START_KOD"; fi; }
        cp() { printf "cp %s\n" "$*"; }
        rm() { printf "rm %s\n" "$*"; }
        soket_saglik_bekle() { printf "SAGLIK-BEKLENDI\n"; return "$SAGLIK_KOD"; }
        eval "$GERI_GOVDE"
        islem=/yok/inceleme birim=/yok/birim soket_birim=/yok/soket
        degisti=1 eski=1 birim_eski=1 soket_eski=1 soket_aktif=1 soket_etkin=1
        onceki_aktif=$ONCEKI_AKTIF
        trap geri_don ERR INT TERM
        false
        printf "GERI-DON-CALISMADI\n"
    '
}
geri() { SAGLIK_KOD=$1 ONCEKI_AKTIF=$2 START_KOD=$3 GERI_GOVDE=$geri_govde geri_calistir; }
bekle 'geri alma: önceki sürüm sağlıksızsa ACİL ve hata kodu' hata 'ACİL: önceki sürüm geri kondu ama' geri 1 1 0
cikti=$(geri 1 1 0 2>&1) && basarisiz 'sağlıksız geri alma sıfır döndü'
[[ $cikti == *'veritabanı göçü GERİ ALINMADI'* && $cikti == *'docs/kurulum.md'* && $cikti == *'yedekten dönülmeli'* ]] || basarisiz "ACİL iletisi eksik: $cikti"
[[ $cikti != *'kurulum geri alındı'* && $cikti != *'rm -rf'* && $cikti != *'GERI-DON-CALISMADI'* ]] || basarisiz "sağlıksız geri alma başarı bildirdi veya inceleme dizinini sildi: $cikti"
bekle 'geri alma: önceki sürüm sağlıklıysa sağlık beklenir, sonra bildirilir' hata 'kurulum geri alındı' geri 0 1 0
cikti=$(geri 0 1 0 2>&1) && basarisiz 'geri alma sıfır döndü'
[[ $cikti == *'SAGLIK-BEKLENDI'* && $cikti != *'ACİL'* ]] || basarisiz "sağlıklı geri almada sağlık beklenmedi: $cikti"
bekle 'geri alma: start başarısızsa ACİL, sağlık beklenmez' hata 'ACİL: geri alma tam değil' geri 0 1 1
cikti=$(geri 0 1 1 2>&1) && basarisiz 'start hatasında geri alma sıfır döndü'
[[ $cikti != *'SAGLIK-BEKLENDI'* && $cikti != *'kurulum geri alındı'* ]] || basarisiz "start hatası yanlış bildirildi: $cikti"
bekle 'geri alma: ilk kurulumda servis başlatılmaz' hata 'kurulum geri alındı' geri 1 0 0
cikti=$(geri 1 0 0 2>&1) && basarisiz 'ilk kurulum geri alma sıfır döndü'
[[ $cikti != *'SAGLIK-BEKLENDI'* && $cikti != *'systemctl start agent-relay'$'\n'* ]] || basarisiz "ilk kurulumda servis başlatıldı: $cikti"
gercek_kilit_sonra=yok
if [[ -e $gercek_kilit || -L $gercek_kilit ]]; then gercek_kilit_sonra=$(find "$gercek_kilit" -printf '%y %P %m %s %T@\n' | sort); fi
[[ $gercek_kilit_once == "$gercek_kilit_sonra" ]] || basarisiz 'test gerçek /run kilit dizinini değiştirdi'
say=$((say + 1)); printf 'OK %02d: gerçek /run kilit dizinine dokunulmadı\n' "$say"
printf 'BAŞARILI: %d test; root/systemd/ağ kullanılmadı; kuru çalışma dosya yazmadı.\n' "$say"
