# ADR 0004: Middleware zinciri ve request ID

- **Durum:** Accepted
- **Tarih:** 2026-11-09
- **Ticket:** FTSM-004

## Context

Bir isteği log'larda bulmanın, her isteğin sonucunu kaydetmenin ve handler'daki bir panic'i düzgün bir 500'e çevirmenin yolu yoktu. `net/http` panic olduğunda bağlantıyı yanıt göndermeden kapatır; bir POS terminali bunu ağ hatası sanıp ödemeyi tekrar deneyebilir.

## Decision

**Zincir sırası:** `requestid.Middleware → AccessLog → Recover → withJSONErrors(mux)`.

- Request ID en dışta: diğer katmanlar ID'yi context'ten okur ve header, panic olsa bile yanıta yazılmış olur.
- AccessLog, Recover'ın dışında: Recover panic'i 500'e çevirdikten sonra AccessLog o 500'ü görür. Sıra ters olsaydı panic access log'un içinden geçer ve istek hiç log'lanmazdı.
- Zincir `withJSONErrors`'ın dışında: 404 ve 405 yanıtları da request ID'li ve log'lu olur.

**Gelen ID'ye koşullu güven:** İstemcinin gönderdiği `X-Request-ID` destek ekibiyle ortak referanstır, bu yüzden geçerliyse aynen kullanılır. Ama güvenilmeyen girdidir: newline içeren bir değer sahte log satırı üretebilir, sınırsız uzunluk log'ları şişirir. Değer 1–128 karakter ve sadece `A-Z a-z 0-9 . _ -` ise kullanılır; aksi hâlde `crypto/rand` ile 16 byte (32 hex) yeni ID üretilir.

**Log'lanmayanlar:** Query string (token, kişisel veri taşıyabilir), header'lar ve gövdeler. Kart verisinin log'a düşmemesi PCI DSS gereği.

**Seviyeler:** 5xx `ERROR`, `/healthz` `DEBUG` (LB birkaç saniyede bir çağırır), diğerleri `INFO`.

**Panic:** Değer ve stack trace sadece log'a gider; istemci sabit `internal_error` gövdesini alır. Yanıt başlamışsa ikinci status yazılmaz. `http.ErrAbortHandler` yeniden fırlatılır çünkü `net/http` onu isteği bilinçli iptal etmek için kullanır.

## Consequences

- Hata gövdesi, yanıt header'ı ve log satırları aynı ID'yi taşır.
- Handler'ın başlattığı ayrı goroutine'lerdeki panic'ler bu middleware tarafından yakalanmaz; arka plan işleri kendi recover'ını içermelidir (FTSM-017 ve sonrası).
- `remote_addr` doğrudan bağlantının adresidir. Proxy arkasında gerçek istemci IP'si FTSM-030'da ele alınacak.
- Route pattern'i yerine ham path log'lanıyor; path parametreli route'lar geldiğinde metriklerde `r.Pattern` kullanılacak (FTSM-027).
