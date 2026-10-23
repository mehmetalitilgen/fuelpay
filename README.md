# FuelPay

FuelPay API servisi. Şu an tek bir sağlık kontrolü uç noktası (`GET /healthz`) sunar.

## Gereksinimler

- Go 1.25.5 veya üzeri (`go.mod` ile aynı sürüm)

## Yapılandırma

| Değişken    | Açıklama                    | Varsayılan |
|-------------|-----------------------------|------------|
| `HTTP_ADDR` | Sunucunun dinleyeceği adres | `:8080`    |

> Not: Servis `.env` dosyasını kendisi okumaz. Oradaki değerleri kullanmak için
> değişkeni çalıştırmadan önce kabuğa aktarın (aşağıdaki örneklere bakın).

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

Sunucu loglarını JSON olarak standart çıktıya yazar.

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
