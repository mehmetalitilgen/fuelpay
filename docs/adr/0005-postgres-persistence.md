# ADR 0005: PostgreSQL kalıcılığı — pgx, sqlc ve goose

- **Durum:** Accepted
- **Tarih:** 2026-11-09
- **Ticket:** FTSM-008, FTSM-009

## Context

İstasyon ve terminal verisi in-memory tutuluyordu; restart'ta kayboluyor ve birden fazla instance arasında paylaşılamıyordu. Domain paketleri (`station`, `terminal`) zaten bir `Repository` interface'i üzerinden çalışıyordu.

## Decision

- **Sürücü: pgx v5 (`pgxpool`).** `database/sql` yerine doğrudan pgx: PostgreSQL tiplerinin doğal desteği, daha az allocation, havuz metrikleri. goose `database/sql` istediği için migration'da `stdlib.OpenDBFromPool` ile aynı havuz paylaşılıyor.
- **Sorgular: sqlc.** SQL elle yazılıyor (`internal/postgres/queries`), Go kodu üretiliyor (`internal/postgres/db`, commit'lenir). ORM yok: sorgu planı ve index kullanımı görünür kalıyor; şema ile sorgu uyumsuzluğu derleme zamanında yakalanıyor. CI'da `sqlc diff` üretilmiş kodun güncel olduğunu doğruluyor.
- **Migration: goose, binary'ye gömülü, API'den ayrı komut** (`cmd/migrate`). API açılışında otomatik migration çoklu instance'ta yarışır; deploy pipeline'ında ayrı adım.
- **Domain hataları constraint adından:** unique/FK ihlalleri (`23505`/`23503`) isimlendirilmiş constraint'e göre `ErrCodeTaken`, `ErrSerialTaken`, `ErrStationNotFound`'a çevriliyor. "Önce SELECT, sonra INSERT" yerine veritabanı kısıtına güveniyoruz; eşzamanlı isteklerde tek doğru kaynak orası.
- **Durum geçişi tek koşullu UPDATE:** `UPDATE ... WHERE id = $1 AND status = $old RETURNING *`. Satır kilidi beklemeden kayıp güncellemeyi önler; satır dönmezse ikinci bir okuma "terminal yok" ile "durum değişti"yi ayırır.
- **UUID dışı ID'ler sorgusuz `not found`:** `uuid.Parse` başarısızsa veritabanına gidilmiyor; geçersiz input'un `22P02` hatası 500'e dönüşmüyor.
- **Zaman damgaları UTC ve mikrosaniye:** servis zamanı mikrosaniyeye kırpıyor (PostgreSQL `timestamptz` hassasiyeti), okunan değerler UTC'ye çevriliyor; round-trip eşitliği testte doğrulanıyor.
- **`DATABASE_URL` boşsa in-memory**, açılışta WARN log'u ile. Lokal deneme kolaylığı için; production'da readiness (FTSM-030) ve deploy konfigürasyonu bu değeri zorunlu kılacak. URL parola içerdiği için hiçbir log'a yazılmıyor.

## Consequences

- Repository testleri `TEST_DATABASE_URL` ile gerçek PostgreSQL'e karşı koşuyor (CI'da servis container'ı). FTSM-011'de testcontainers ile kendi kendine yeten hâle gelecek.
- Şema değişikliği = yeni migration + `sqlc generate`; uygulanmış migration düzenlenmez.
