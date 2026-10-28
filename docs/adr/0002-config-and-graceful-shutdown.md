# ADR 0002: Konfigürasyon ve graceful shutdown

- **Durum:** Accepted
- **Tarih:** 2026-10-28
- **Ticket:** FTSM-002

## Context

Timeout'lar FTSM-001'de koda sabit yazılmıştı. Ortamlar arasında bu değerlerin ayarlanabilmesi gerekiyor. Hatalı bir değer servis başlarken yakalanmalı. Deploy ve restart sırasında devam eden request'ler kaybedilmemeli.

## Decision

**Config kaynağı:** Değerler sadece ortam değişkenlerinden okunuyor (12-factor) ve stdlib kullanılıyor. Yedi değişken için viper veya envconfig gibi bir bağımlılık eklemek, getireceği faydadan daha pahalı.

**Boş ve tanımsız değer:** Boş string set edilmiş bir değişken, hiç tanımlanmamış gibi default değeri alıyor. Docker Compose ve CI'da `VAR=` yazılan satırlar sık görülür. Bunların servisi düşürmesi yerine default'a düşmesini tercih ediyoruz.

**Validation:** Config yüklenirken bütün hatalar `errors.Join` ile toplanıyor ve tek seferde raporlanıyor. Servis bu durumda port açmadan exit code `1` ile çıkıyor. Bir değer parse edilemediğinde, o değeri içeren cross-field kuralı (header timeout ≤ read timeout) çalıştırılmıyor. Aksi hâlde kullanıcının hiç girmediği default değerle karşılaştırma yapan yanıltıcı bir hata üretilirdi.

**Logger:** Logger config'ten önce JSON formatında kuruluyor. Log seviyesi config yüklendikten sonra `slog.LevelVar` ile uygulanıyor. Böylece config hataları da JSON formatında log'lanıyor.

**Shutdown:**
- `main`, ilk `SIGINT`/`SIGTERM` sinyalinde context'i sinyal adını içeren bir cause ile iptal ediyor. Sonraki sinyallerde varsayılan davranış geçerli, yani ikinci Ctrl+C process'i hemen öldürür.
- `run`, context'i parametre olarak alıyor. Bu sayede testler gerçek bir OS sinyali göndermeden kapanışı tetikleyebiliyor.
- Shutdown context'i `context.WithoutCancel` ile türetiliyor. Böylece zaten iptal edilmiş olan sinyal context'inin iptali shutdown'a geçmiyor.
- `SHUTDOWN_TIMEOUT` aşılırsa kalan bağlantılar `Close` ile kapatılıyor ve process `1` ile çıkıyor.

**`SHUTDOWN_TIMEOUT` = 15s:** Kubernetes'in varsayılan `terminationGracePeriodSeconds` değeri 30s. 15s hem LB'nin trafiği kesmesine hem de in-flight request'lerin bitmesine yetecek pay bırakıyor. Değer orkestratörün bekleme süresinden her zaman kısa tutulmalı. Varsayılan `docker stop` sadece 10s bekler; bu yüzden README'de `docker stop -t 20` önerildi.

**Paket yapısı:** ADR 0001 `internal/`'a geçişi FTSM-005'e bırakmıştı. Config ve server artık ayrı sorumluluklar olduğu için geçiş bu ticket'ta yapıldı. `internal/config` env okuma ve validation'ı, `internal/server` handler ve `http.Server` kurulumunu içeriyor. `cmd/api` sadece wiring ve process yaşam döngüsünü yönetiyor.

## Consequences

- Config hatası olan bir deploy hiç ayağa kalkmaz ve hatanın nedeni log'da tek satırda görünür.
- Shutdown davranışı Windows dahil her ortamda otomatik testlerle doğrulanıyor. Sadece sinyali context'e çeviren küçük fonksiyon (`signalContext`) manuel olarak test ediliyor.
- Readiness'ı fail edip trafiği önce boşaltma (drain) adımı henüz yok. FTSM-030'da eklenecek.
