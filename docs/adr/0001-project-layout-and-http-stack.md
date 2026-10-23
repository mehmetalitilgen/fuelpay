# ADR 0001: Proje yapısı ve HTTP stack

- **Durum:** Accepted
- **Tarih:** 2026-10-23
- **Ticket:** FTSM-001

## Context

FuelPay TSM'in ilk sürümü tek bir liveness endpoint'i (`GET /healthz`) sunuyor. Sonraki ticket'larda station, terminal ve payment API'leri eklenecek. Bu yüzden başlangıç yapısı basit kalmalı, ama sonradan büyütülebilmeli.

## Decision

**Layout:** Tek binary için `cmd/api/` kullanıyoruz. Şimdilik tüm kod `package main` içinde, çünkü tek bir endpoint için ayrı paket açmak premature abstraction olur. İlk domain handler'ı geldiğinde (FTSM-005, station API) HTTP katmanı ve domain kodu `internal/` altına taşınacak. `main` sadece wiring yapacak.

**HTTP stack:** Stdlib `net/http` ve Go 1.22+ `ServeMux` kullanıyoruz. Bunlar method matching, path wildcard'ları ve otomatik 405 + `Allow` desteği sağlıyor ve hiçbir bağımlılık eklemiyor. Middleware gruplama veya subrouter ihtiyacı gerçek bir bakım maliyeti oluşturursa chi'ye geçişi değerlendireceğiz. chi `http.Handler` uyumlu olduğu için geçiş maliyeti düşük. Gin kullanmıyoruz, çünkü stdlib handler imzasından ayrılıyor.

**Timeout'lar:**

| Alan | Değer | Gerekçe |
|---|---|---|
| `ReadHeaderTimeout` | 5s | Header'lar küçüktür. Kısa limit slowloris tipi saldırılara karşı korur. Body okumasından bağımsız ve daha kısa olmalı. |
| `ReadTimeout` | 10s | Header + body. Şu an body'li bir endpoint yok, ödeme payload'ları da küçük olacak. |
| `WriteTimeout` | 10s | Handler'larımız kısa süreli. Streaming ihtiyacı doğarsa ilgili endpoint'te `http.ResponseController` ile ayrıca ele alınacak. |
| `IdleTimeout` | 120s | Önündeki LB/proxy'nin idle timeout'undan (örn. AWS ALB varsayılanı 60s) **uzun** olmalı. Aksi hâlde LB, backend'in kapattığı bir bağlantıyı yeniden kullanır ve istemci aralıklı 502 alır. |
| `MaxHeaderBytes` | 1 MB | Stdlib varsayılanı. Bilinçli olarak açıkça yazıldı. |

**Logging:** `log/slog` JSON handler kullanıyoruz ve log'lar stdout'a yazılıyor. Log anahtarları küçük harf olacak (`addr`, `version`, `err`).

**Sürüm:** `main.version` değişkenine `-ldflags "-X main.version=..."` ile build zamanında yazılıyor. Handler'a parametre olarak veriliyor, böylece global state'e bağlı değil ve test edilebiliyor.

## Consequences

- Hiç dış bağımlılık yok, build basit.
- `package main` içindeki kod başka paketlerden import edilemiyor. FTSM-005'te `internal/`'a taşıma zorunlu olacak.
- Timeout değerleri sabit. FTSM-002'de config ile override edilebilir hâle gelecek.
