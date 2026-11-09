# FuelPay

FuelPay TSM API servisi: akaryakıt istasyonları, POS terminalleri ve ödemeler için backend.

Endpoint referansı: [docs/api.md](docs/api.md).

## Gereksinimler

- Go 1.25.5 veya üzeri (`go.mod` ile aynı sürüm)

## Yapılandırma

Tüm ayarlar ortam değişkenlerinden okunur. Boş bırakılan değişken varsayılan değerini alır.

| Değişken                   | Açıklama                                                        | Varsayılan |
|----------------------------|-----------------------------------------------------------------|------------|
| `HTTP_ADDR`                | Sunucunun dinleyeceği adres (`host:port`)                       | `:8080`    |
| `HTTP_READ_HEADER_TIMEOUT` | İstek header'larının okunması için süre sınırı                  | `5s`       |
| `HTTP_READ_TIMEOUT`        | Header ve gövde dahil isteğin tamamının okunması için süre sınırı | `10s`      |
| `HTTP_WRITE_TIMEOUT`       | Yanıtın yazılması için süre sınırı                              | `10s`      |
| `HTTP_IDLE_TIMEOUT`        | Keep-alive bağlantısının boşta bekleyebileceği süre             | `120s`     |
| `SHUTDOWN_TIMEOUT`         | Kapanışta devam eden isteklerin bitmesi için beklenecek süre    | `15s`      |
| `LOG_LEVEL`                | Log seviyesi: `debug`, `info`, `warn`, `error`                  | `info`     |
| `DATABASE_URL`             | PostgreSQL bağlantı adresi; boşsa in-memory depolama (parola içerir, log'lanmaz) | —          |

Süre değerleri Go'nun `time.ParseDuration` biçimindedir: `500ms`, `10s`, `2m` gibi.

Doğrulama kuralları:

- `HTTP_ADDR` geçerli bir `host:port` olmalı ve port `0-65535` aralığında olmalı.
  Host boş bırakılabilir (`:8080`); bu durumda tüm ağ arayüzleri dinlenir.
- Tüm süreler sıfırdan büyük olmalı.
- `HTTP_READ_TIMEOUT`, `HTTP_READ_HEADER_TIMEOUT` değerinden küçük olamaz.

Geçersiz değerler varsa servis port açmadan çıkar (exit code `1`) ve hataların tamamı tek bir JSON log satırında raporlanır:

```json
{"level":"ERROR","msg":"application terminated","err":"load config: SHUTDOWN_TIMEOUT: time: invalid duration \"abc\"\nLOG_LEVEL: must be one of: debug, info, warn, error"}
```

> Not: Servis `.env` dosyasını kendisi okumaz. Oradaki değerleri kullanmak için
> değişkenleri çalıştırmadan önce kabuğa aktarın (aşağıdaki örneklere bakın).

## Veritabanı

Geliştirme için PostgreSQL 18, Docker Compose ile çalışır (port `5433`, lokal bir
PostgreSQL kurulumuyla çakışmasın diye):

```sh
docker compose up -d
```

Şema migration'ları [goose](https://github.com/pressly/goose) ile yazılır, SQL dosyaları
`internal/postgres/migrations/` altındadır ve binary'ye gömülür:

```sh
# bash
export DATABASE_URL="postgres://fuelpay:fuelpay@localhost:5433/fuelpay?sslmode=disable"
go run ./cmd/migrate up       # bekleyen migration'ları uygula
go run ./cmd/migrate status   # durumu listele
go run ./cmd/migrate down     # son migration'ı geri al
```

```powershell
# PowerShell
$env:DATABASE_URL = "postgres://fuelpay:fuelpay@localhost:5433/fuelpay?sslmode=disable"
go run ./cmd/migrate up
```

Yeni migration: `internal/postgres/migrations/` altına sıradaki numarayla
(`00002_aciklama.sql`) `-- +goose Up` / `-- +goose Down` bölümlerini içeren bir dosya ekleyin.
Uygulanmış bir migration dosyası **değiştirilmez**; düzeltme yeni bir migration ile yapılır.

Sorgular `internal/postgres/queries/*.sql` altında elle yazılır; Go kodu
[sqlc](https://sqlc.dev) ile üretilir (`internal/postgres/db`, commit'lenir). Sorgu veya şema
değişince:

```sh
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0   # bir kez
sqlc generate
```

CI, üretilmiş kodun güncel olduğunu `sqlc diff` ile kontrol eder.

API, `DATABASE_URL` tanımlıysa PostgreSQL'i kullanır (şema önceden migrate edilmiş olmalı);
tanımlı değilse in-memory depolamayla açılır ve bir `WARN` log'u yazar — veri restart'ta
kaybolur. Kararlar: [ADR 0005](docs/adr/0005-postgres-persistence.md).

Veritabanı testleri her test için **kendi boş veritabanını** oluşturur ve test bitince siler
(`internal/postgres/pgtest`). Sunucu şöyle seçilir:

1. `TEST_DATABASE_URL` tanımlıysa o sunucu (kullanıcı `CREATE DATABASE` yetkisine sahip olmalı).
2. Değilse Docker çalışıyorsa [testcontainers-go](https://golang.testcontainers.org/) ile
   `postgres:18-alpine` container'ı (test binary'si başına bir kez) başlatılır.
3. İkisi de yoksa veritabanı testleri atlanır (skip).

```sh
go test ./...                       # Docker açıksa container ile
TEST_DATABASE_URL="postgres://fuelpay:fuelpay@localhost:5433/postgres?sslmode=disable" go test ./...
```

CI'da testler testcontainers ile koşar.

## Çalıştırma

Varsayılan adresle (`:8080`):

```sh
go run ./cmd/api
```

Farklı bir adresle:

```sh
# bash
HTTP_ADDR=":9090" go run ./cmd/api
```

```powershell
# PowerShell
$env:HTTP_ADDR = ":9090"; go run ./cmd/api
```

Birden fazla değişkenle:

```sh
# bash
HTTP_ADDR=":9090" LOG_LEVEL=debug SHUTDOWN_TIMEOUT=30s go run ./cmd/api
```

```powershell
# PowerShell
$env:HTTP_ADDR = ":9090"; $env:LOG_LEVEL = "debug"; $env:SHUTDOWN_TIMEOUT = "30s"
go run ./cmd/api
```

`.env` dosyasındaki tüm değerleri bash'te kabuğa aktarmak için:

```sh
set -a; source .env; set +a
go run ./cmd/api
```

Sunucu loglarını JSON olarak standart çıktıya yazar. Port açıldıktan sonra dinlenen
adresi, sürümü, süre sınırlarını ve log seviyesini içeren bir `http server listening` satırı loglanır.

## Kapanış (graceful shutdown)

Servis `SIGINT` (Ctrl+C) veya `SIGTERM` aldığında düzgün şekilde kapanır:

1. `shutdown started` loglanır (`reason` alanında sinyal adı yer alır) ve yeni bağlantı kabul edilmez.
2. Devam eden istekler en fazla `SHUTDOWN_TIMEOUT` süresince tamamlanmaya bırakılır.
   Boştaki keep-alive bağlantıları hemen kapatılır.
3. Tüm istekler bu süre içinde biterse `server stopped` loglanır ve süreç `0` ile çıkar.
4. Süre dolarsa kalan bağlantılar zorla kapatılır, `graceful shutdown: context deadline exceeded`
   hatası loglanır ve süreç `1` ile çıkar.

Kapanış sırasında ikinci bir Ctrl+C veya `SIGTERM` gelirse süreç beklemeden hemen sonlanır.

Notlar:

- Windows'ta servisi durdurmak için Ctrl+C kullanın. `taskkill /F` ve Görev
  Yöneticisi süreci sinyal göndermeden öldürür, bu yüzden düzgün kapanış yapılmaz.
- Docker ve Kubernetes durdururken `SIGTERM` gönderir. `SHUTDOWN_TIMEOUT` değeri
  platformun bekleme süresinden kısa olmalıdır. Örneğin Kubernetes'te
  `terminationGracePeriodSeconds` varsayılan olarak 30 saniyedir; `docker stop` ise 10 saniye bekler.
  `docker stop` ile varsayılan `15s` değeri dolmadan süreç öldürülebilir; gerekirse `docker stop -t 20` kullanın.

## Hata formatı

Tüm hata yanıtları tek bir `error` anahtarının altında, aynı gövdeyle döner. Başarılı
yanıtlar zarfa sarılmaz. Kararın gerekçesi için [ADR 0003](docs/adr/0003-error-response-format.md).

```json
{
  "error": {
    "code": "invalid_json",
    "message": "Request body contains an unknown field.",
    "details": [
      { "field": "colour", "issue": "unknown_field" }
    ]
  }
}
```

- `code` — zorunlu. Sabit makine kodu; istemci buna göre dallanmalıdır.
- `message` — zorunlu. İnsan için açıklama. Herhangi bir sürümde değişebilir,
  **parse edilmemelidir.**
- `details` — opsiyonel. Alan bazlı hata yoksa anahtar hiç yazılmaz; `null` veya `[]` dönmez.

### Kodlar

| Kod | Status | Ne zaman |
|---|---|---|
| `invalid_json` | 400 | Gövde boş, bozuk, obje değil, birden fazla değer içeriyor, bilinmeyen alan var veya bir alanın tipi yanlış |
| `validation_failed` | 400 | Gövde geçerli JSON ama bir veya birden fazla alan iş kurallarına uymuyor |
| `payload_too_large` | 413 | Gövde 1 MiB sınırını aşıyor |
| `unsupported_media_type` | 415 | `Content-Type` `application/json` değil |
| `not_found` | 404 | Yol eşleşmedi veya kaynak yok |
| `method_not_allowed` | 405 | Yol var ama method desteklenmiyor; yanıt `Allow` header'ı taşır |
| `conflict` | 409 | Benzersiz olması gereken bir değer zaten kullanımda veya kaynak istenen işleme uygun durumda değil; `details` hangi alan olduğunu söyler |
| `internal_error` | 500 | Beklenmeyen hata. Gövde sabittir, sebep yalnızca log'a yazılır |

### `details[].issue` değerleri

Bunlar da sabit kodlardır, boşluk içermez.

| Issue | Ne zaman |
|---|---|
| `invalid_type` | Alanın JSON tipi beklenenden farklı. Beklenen tip `message`'da yazar |
| `unknown_field` | Gövdede şemada olmayan alan var |
| `required` | Zorunlu alan eksik veya sadece boşluk |
| `too_short` / `too_long` | Uzunluk sınırı dışında |
| `invalid_format` | Değer beklenen biçimde değil, örn. küçük harfli istasyon kodu |
| `already_exists` | `conflict` ile birlikte: değer başka bir kayıtta kullanılıyor |
| `invalid_transition` | `conflict` ile birlikte: kaynağın mevcut durumu bu işleme izin vermiyor |
| `concurrent_update` | `conflict` ile birlikte: kaynak eşzamanlı başka bir istekle değişti, tekrar deneyin |

### Örnekler

Bilinmeyen yol:

```sh
curl.exe -i http://localhost:8080/does-not-exist
```

```json
{"error":{"code":"not_found","message":"The requested resource was not found."}}
```

Desteklenmeyen method (yanıt `Allow: GET, HEAD` header'ı ile gelir):

```sh
curl.exe -i -X POST http://localhost:8080/healthz
```

```json
{"error":{"code":"method_not_allowed","message":"The request method is not allowed for this resource."}}
```

Yanlış tipte alan:

```json
{"error":{"code":"invalid_json","message":"Request body field \"count\" must be of type int.","details":[{"field":"count","issue":"invalid_type"}]}}
```

Hata gövdeleri ayrıca üst seviyede `request_id` alanını taşır (bkz. sonraki bölüm).

## Request ID ve access log

Her yanıt bir `X-Request-ID` header'ı taşır. İstek geçerli bir `X-Request-ID`
gönderirse (1–128 karakter, sadece `A-Z a-z 0-9 . _ -`) aynı değer kullanılır;
yoksa veya geçersizse 32 karakterlik rastgele hex bir ID üretilir. Aynı değer hata
gövdesinde de döner:

```json
{"error":{"code":"not_found","message":"The requested resource was not found."},"request_id":"test-1"}
```

Her istek tamamlandığında tek bir access log satırı yazılır:

```json
{"time":"...","level":"INFO","msg":"http request","request_id":"test-1","method":"GET","path":"/nope","status":404,"bytes":102,"duration_ms":0.21,"remote_addr":"127.0.0.1"}
```

- Query string, header'lar ve gövdeler log'lanmaz.
- `/healthz` satırları `DEBUG` seviyesindedir; `LOG_LEVEL=debug` ile görünür.
- 5xx yanıtlar `ERROR` seviyesinde log'lanır.
- Handler'daki bir panic istemciye `500 internal_error` olarak döner; panic değeri ve
  stack trace sadece `panic recovered` log satırına yazılır.

Kararların gerekçesi: [ADR 0004](docs/adr/0004-middleware-and-request-id.md).

## Test

Birim testleri ve statik analiz:

```sh
go test ./...
go vet ./...
```

Race detector (Windows'ta CGO ve gcc gerekir):

```sh
# bash
CGO_ENABLED=1 go test -race -count=10 ./...
```

```powershell
# PowerShell
$env:CGO_ENABLED = "1"; go test -race -count=10 ./...
```

Lint ([golangci-lint](https://golangci-lint.run/) v2, ayarlar `.golangci.yml`'de):

```sh
golangci-lint run ./...
golangci-lint fmt ./...   # gofmt + goimports ile düzeltir
```

### CI

Her PR ve `main`'e her push'ta GitHub Actions (`.github/workflows/ci.yml`) üç iş çalıştırır:
`lint` (golangci-lint), `test` (`go mod tidy` kontrolü + `go test -race` + coverage) ve
`build` (statik binary, sürüm = commit SHA). Branch'ler PR üzerinden ve CI yeşilken merge edilmelidir.

Çalışan servisi elle kontrol etmek için (PowerShell'de `curl` bir alias'tır, `curl.exe` kullanın):

```sh
curl.exe -i http://localhost:8080/healthz
```

Beklenen yanıt:

```json
{"status":"ok","version":"dev"}
```

## Sürüm bilgisiyle build

Sürüm, `main.version` değişkenine build sırasında `-ldflags` ile yazılır.
Belirtilmezse `dev` olur ve `/healthz` yanıtında bu değer görünür.

```sh
go build -ldflags "-X main.version=v0.1.0" -o bin/fuelpay ./cmd/api
```

Sürümü git etiketinden almak için:

```sh
# bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o bin/fuelpay ./cmd/api
```

```powershell
# PowerShell
$v = git describe --tags --always --dirty
go build -ldflags "-X main.version=$v" -o bin/fuelpay.exe ./cmd/api
```

Oluşan binary'yi çalıştırıp sürümü doğrulayın:

```sh
./bin/fuelpay
curl http://localhost:8080/healthz
# {"status":"ok","version":"v0.1.0"}
```
