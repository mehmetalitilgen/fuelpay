# FuelPay

FuelPay API servisi. Şu an tek bir sağlık kontrolü uç noktası (`GET /healthz`) sunar.

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

Süre değerleri Go'nun `time.ParseDuration` biçimindedir: `500ms`, `10s`, `2m` gibi.

Doğrulama kuralları:

- `HTTP_ADDR` geçerli bir `host:port` olmalı ve port `0-65535` aralığında olmalı.
  Host boş bırakılabilir (`:8080`); bu durumda tüm ağ arayüzleri dinlenir.
- Tüm süreler sıfırdan büyük olmalı.
- `HTTP_READ_TIMEOUT`, `HTTP_READ_HEADER_TIMEOUT` değerinden küçük olamaz.

Geçersiz değerler varsa servis başlamaz ve hataların tamamı tek bir log satırında raporlanır:

```text
ERROR application terminated err="load config: SHUTDOWN_TIMEOUT: time: invalid duration \"abc\"\nLOG_LEVEL: must be one of: debug, info, warn, error"
```

> Not: Servis `.env` dosyasını kendisi okumaz. Oradaki değerleri kullanmak için
> değişkenleri çalıştırmadan önce kabuğa aktarın (aşağıdaki örneklere bakın).

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

Sunucu loglarını JSON olarak standart çıktıya yazar. Başlangıçta kullanılan
adresi, süre sınırlarını ve log seviyesini içeren bir `http server listening` satırı loglanır.

## Kapanış (graceful shutdown)

Servis `SIGINT` (Ctrl+C) veya `SIGTERM` aldığında düzgün şekilde kapanır:

1. `shutting down` loglanır ve yeni bağlantı kabul edilmez.
2. Devam eden istekler en fazla `SHUTDOWN_TIMEOUT` süresince tamamlanmaya bırakılır.
   Boştaki keep-alive bağlantıları hemen kapatılır.
3. Tüm istekler bu süre içinde biterse `server stopped` loglanır.
4. Süre dolarsa `graceful shutdown: context deadline exceeded` hatası loglanır
   ve süreç kalan bağlantıları beklemeden kapanır.

Kapanış sırasında ikinci bir Ctrl+C veya `SIGTERM` gelirse süreç beklemeden hemen sonlanır.

Notlar:

- Windows'ta servisi durdurmak için Ctrl+C kullanın. `taskkill /F` ve Görev
  Yöneticisi süreci sinyal göndermeden öldürür, bu yüzden düzgün kapanış yapılmaz.
- Docker ve Kubernetes durdururken `SIGTERM` gönderir. `SHUTDOWN_TIMEOUT` değeri
  platformun bekleme süresinden kısa olmalıdır. Örneğin Kubernetes'te
  `terminationGracePeriodSeconds` varsayılan olarak 30 saniyedir; `docker stop` ise 10 saniye bekler.
  `docker stop` ile varsayılan `15s` değeri dolmadan süreç öldürülebilir; gerekirse `docker stop -t 20` kullanın.

## Test

Birim testleri ve statik analiz:

```sh
go test ./...
go vet ./...
```

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
