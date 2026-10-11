"# Yardımcı ajanlar: Codex CLI ve Gemini (agy) — kullanım kılavuzu
Bu makinede iki yardımcı ajan var. Claude tasarlar, görev metnini yazar, testleri çalıştırır,
diff'i inceler ve commit'ler. Kodu ve araştırmayı yardımcılar yapar (Claude kotası dar).

- Karmaşık kodlama, çok dosyalı değişiklik, kod incelemesi → Codex
- Küçük işler, metin üretme, ikinci görüş, (izinli sitelerden) web okuma → Gemini

1. Codex CLI (ChatGPT aboneliğiyle giriş yapılmış, API anahtarı yok)
   Yol (PATH'te değil; uygulama güncellenince klasör adı değişebilir):
   /c/Users/pc/AppData/Local/OpenAI/Codex/bin/cdef5aaf3e41ab53/codex.exe
   Bulamazsan: find /c/Users/pc/AppData/Local/OpenAI -iname codex.exe
   Çağrı (Bash aracında, run_in_background: true ile):
   cd <proje> && codex.exe exec -m gpt-6-sol --sandbox workspace-write --skip-git-repo-check \
    "$(cat <scratchpad>/gorev.txt)" < /dev/null > <scratchpad>/codex.log 2>&1; echo cikis=$?

- -m gpt-6-sol: model kimliği tam olarak bu. "sol-6" ve "gpt-sol-6" reddediliyor.
- --sandbox read-only: yalnız inceleme ve rapor. workspace-write: kod değiştirecekse.
- < /dev/null şart; stdin açık kalırsa takılır.
- Cevap: logda "tokens used" satırından sonraki blok.
  awk '/^tokens used/{f=1;next} f' codex.log | tail -n +2
- Codex kendi sandbox'ında projenin .venv'ini çoğu zaman BAŞLATAMIYOR ("Python312 bulunamadı").
  Testleri her zaman Claude kendisi çalıştırır. Görev metnine "başlamazsa bildir, BAŞKA PYTHON
  KULLANMA" yaz; yazmazsan Anaconda gibi başka bir Python'a geçebiliyor.
- read-only modda proje dışındaki dosyaları (ör. Claude'un scratchpad'i) okuyamıyor.
  Girdi dosyalarını proje içinde .gitignore'daki bir klasöre kopyala (ör. cache/inceleme/).
- İki Codex görevi paralel çalışabilir, ama yalnız farklı dosyalara dokunuyorlarsa.
  Görevlerde "şu dosyalara DOKUNMA" listesini açıkça ver.

2. Gemini — Antigravity CLI (agy, Google AI Pro girişi)
   Yol: /c/Users/pc/AppData/Local/agy/bin/agy.exe
   Modeller: agy models (ör. gemini-3.1-pro-high, gemini-3.8-flash-medium)
   Çağrı:
   agy.exe -p "$(cat prompt.txt)" --model gemini-3.1-pro-high --print-timeout 900s \
    < /dev/null > gemini.out 2>&1

- Başsız (-p) modda araç izinleri sorulamaz, varsayılan olarak REDDEDİLİR.
  Hata: "jetski: no output produced — ... permission ... auto-denied".
- Dosya okutma güvenilmez (read_file izin kuralları tutmadı). Doğru yöntem: dosya içeriğini
  PROMPT'UN İÇİNE göm ve "hiçbir araç kullanma" de. Windows argüman sınırı ~32.000 karakter;
  prompt'u bir dosyada kur ve karakter sayısını ölç.
- Web okuma yalnız izin verilen alan adlarında çalışır:
  ~/.gemini/antigravity-cli/settings.json → permissions.allow
  ör. "read_url(www.kap.org.tr)", "read_url(kap.org.tr)". www'li ve www'siz ayrı yazılmalı.
  Bu dosyayı Claude DEĞİŞTİREMEZ (güvenlik denetimi engeller, doğrusu da bu): kullanıcıdan iste.
- JavaScript ile yüklenen sayfalarda read_url yalnız iskelet döndürür; veri gelmez.
- ASLA --dangerously-skip-permissions kullanma: komut çalıştırabilen ajan ortam
  değişkenlerinden API anahtarlarını okuyabilir.

3. Her görev metninde bulunması gerekenler (şablon)
   GÖREV: tek cümle. Kullanıcı onayı <tarih>.
   GÜVENLİK KURALLARI (uymazsan görev geçersiz):

- Ortam değişkeni OKUMA, YAZDIRMA yok. API anahtarı arama yok. .env arama yok.
- Ağ isteği YOK (gerekmiyorsa). Veritabanı/üretim verisine YAZMA; testler tmp_path kullanır.
- Değiştirilebilir dosyalar: <liste>. Şunlara DOKUNMA: <liste>.
- Testler: <komut> (şu an N geçiyor). Başlamazsa bildir; başka Python kullanma.
  NEDEN: gerçek olay ve sayılarla, neden gerekli.
  YAPILACAK: numaralı, somut adımlar; isimler, sabitler, hata durumları.
  TESTLER: hangi senaryolar (Türkçe isim, NEDEN docstring'i).
  KABUL: tüm testler geçsin. Bitince özetle: <tam olarak neyi söylemesini istiyorsun>.

4. Çalışma düzeni ve tuzaklar
1. Görev metnini Write aracıyla scratchpad'e yaz, ajanı run_in_background ile başlat.
   Bildirim gelene kadar başka işe geç; yoklama yapma. Bash'te & KULLANMA: harness takibi kopar.
1. Ajan bitince: özeti oku → git diff'i KENDİN incele → tam test takımını KENDİN çalıştır.
   Ajan "testler geçti" ya da "çalıştıramadım" diyebilir; ikisine de güvenme.
1. Gemini'nin iddialarını kanıtla doğrulamadan kullanıcıya aktarma. Sık görülen hatalar:
   yanlış yönlü yorum, olayları kaçırmak, "veri tamam" sanmak.
1. Ters bölü içeren içerik (Windows yolu, .bat) için Bash heredoc ya da sed KULLANMA:
   \b gibi diziler bozulur. Write aracıyla bir Python betiği yaz, onu çalıştır.
1. Yaygın hata: anahtar kelime eşlemede alt dize eşleşmesi ("Dönemsel" içinde "sel").
   Ajanın yazdığı sınıflandırıcıları gerçek veride say ve örnekle kontrol et.
1. Commit'ten önce sır kontrolü yap: diff'te API anahtarı deseni, .env, veritabanı dosyası,
   1 MB'den büyük dosya olmamalı. Force push yapma.
   "
