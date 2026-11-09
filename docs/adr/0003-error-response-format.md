# ADR 0003: Hata yanıt formatı

- **Durum:** Accepted
- **Tarih:** 2026-11-02
- **Ticket:** FTSM-003

## Context

FTSM-001 ve FTSM-002'de tek endpoint `/healthz` olduğu için hata yanıtı diye bir sorun yoktu; 404 ve 405 stdlib'in düz metin cevabıyla dönüyordu. FTSM-005'te gelecek station API'siyle birlikte request body parse etme, alan doğrulama ve domain hataları (kayıt yok, çakışma) başlayacak. Bunlar başlamadan önce tek bir hata sözleşmesi oturtulmalı, aksi hâlde her handler kendi formatını uydurur ve istemci tarafı her endpoint için ayrı parse kodu yazar.

## Decision

### Zarf formatı

Her hata yanıtı tek bir `error` anahtarının altında döner:

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

- `code` — zorunlu, sabit makine kodu.
- `message` — zorunlu, insan için açıklama.
- `details` — opsiyonel. Alan bazlı hata yoksa anahtar hiç yazılmaz (`omitempty`), `null` veya `[]` dönmez.

Başarılı yanıtlar zarfa sarılmaz; `/healthz` hâlâ düz `{"status":"ok","version":"dev"}` döner. Zarf sadece hata tarafında, çünkü asıl faydası (hata mı değil mi ayrımı) başarı tarafında yok.

Dış zarfı tercih etme sebebimiz, üst seviyenin ileride kardeş alanlara açık kalması. FTSM-004'te `request_id` ekleneceği zaman `{"error": {...}, "request_id": "..."}` olarak girecek; `code`/`message` düz dönseydi bu alanı hata gövdesinin içine sıkıştırmak ya da formatı kırmak gerekirdi. İkinci fayda, istemcinin bir yanıtın hata olup olmadığını status koduna bakmadan `error` anahtarının varlığından anlayabilmesi.

### Kod kataloğu

| Kod | Status | Ne zaman |
|---|---|---|
| `invalid_json` | 400 | Gövde boş, bozuk, obje değil, birden fazla değer içeriyor, bilinmeyen alan var veya bir alanın tipi yanlış |
| `validation_failed` | 400 | Gövde geçerli JSON ama bir veya birden fazla alan iş kurallarına uymuyor |
| `unsupported_media_type` | 415 | `Content-Type` `application/json` değil |
| `payload_too_large` | 413 | Gövde 1 MiB sınırını aşıyor |
| `not_found` | 404 | Yol eşleşmedi veya kaynak yok |
| `method_not_allowed` | 405 | Yol var, method desteklenmiyor (`Allow` header'ı ile birlikte) |
| `conflict` | 409 | Benzersiz alan çakışması veya kaynağın mevcut durumu işleme izin vermiyor (FTSM-005 ile eklendi) |
| `internal_error` | 500 | Beklenmeyen hata; sebebi sadece log'a yazılır, gövdeye sızmaz |

`details[].issue` değerleri de sabit kodlardır, boşluk içermez:

| Issue | Ne zaman |
|---|---|
| `invalid_type` | Alanın JSON tipi beklenenden farklı |
| `unknown_field` | Gövdede şemada olmayan alan var |
| `required` | Zorunlu alan eksik veya sadece boşluk (FTSM-005) |
| `too_short` / `too_long` | Uzunluk sınırı dışında (FTSM-005) |
| `invalid_format` | Değer beklenen biçimde değil, örn. küçük harfli istasyon kodu (FTSM-005) |
| `already_exists` | `conflict` ile birlikte: değer başka bir kayıtta kullanılıyor (FTSM-005) |

Gövde parse hatalarının hepsinin tek bir `invalid_json` kodu altında toplanması bilinçli. İstemci açısından bunların hepsi aynı sonucu doğurur: gövdeyi düzelt ve tekrar gönder. Her alt durum için ayrı kod üretmek (`empty_body`, `malformed_json`, `unknown_field`, ...) istemciye davranış farkı sağlamayan bir switch yazdırır, ama kataloğu ve dolayısıyla API sözleşmesini şişirir. Ayrım gereken yerde `details[].issue` ve `message` zaten mevcut.

### `code` sabit, `message` değişebilir

`code` makine sözleşmesidir. İstemci koduna göre dallanır, alarm ve metrikler bu değere göre gruplanır. Bu yüzden:

- Yeni kod eklemek geriye dönük uyumludur (additive).
- Mevcut bir kodu yeniden adlandırmak veya anlamını değiştirmek **breaking change**'dir ve sürüm artışı gerektirir.
- Kod listesi bu ADR'de ve README'de tutulur; yeni kod ekleyen ticket ikisini de günceller.

`message` insan içindir: hata ayıklayan geliştirici, log'a bakan operasyon. Bu yüzden:

- İsteğe özel detay taşıyabilir (alan adı, byte offset, beklenen tip).
- Herhangi bir sürümde yeniden yazılabilir, netleştirilebilir, ileride yerelleştirilebilir.
- **İstemci `message`'ı parse etmemelidir.** Davranış değiştiren hiçbir bilgi sadece `message`'da durmaz.

Pratik sonucu şu: yanlış tipteki alanda `issue` sabit `invalid_type` kodudur, beklenen tip bilgisi (`int`) `message`'a konur. Tip adı stdlib'in `reflect.Type` çıktısına bağlı olduğu için sabit bir sözleşme sayılamaz; makinenin güvenebileceği kısım `field` + `issue` ikilisidir.

### Neden 422 değil 400

`validation_failed` için 400 kullanıyoruz, 422 Unprocessable Content değil.

RFC 9110 422'yi "iyi biçimlendirilmiş ama semantik olarak işlenemeyen istek" diye tanımlar ve bu tanım alan doğrulamasına teknik olarak uyar. Buna rağmen 400'de kalmamızın sebepleri:

- **İstemci için davranış farkı yok.** Hem `invalid_json` hem `validation_failed` aynı şeyi söyler: gövdeyi düzelt, tekrar dene. İki ayrı status kodu, aynı eylemi gerektiren iki dal üretir. Ayrımı zaten `code` yapıyor.
- **Tek bir 4xx sınıfı operasyonu basitleştirir.** Dashboard, alarm kuralı ve LB/WAF konfigürasyonları 400'ü istisnasız tanır. 422 ise ekip içinde ve ara katmanlarda ayrıca tanımlanması gereken bir istisna olur.
- **422'nin geçmişi WebDAV'dır.** RFC 9110'a taşınmış olsa da yaygın istemci kütüphanelerinde 400 kadar oturmuş değil.

Bu kararı şu durumda gözden geçiririz: istemcinin şema hatası ile iş kuralı hatasını status koduna bakarak ayırması gereken somut bir ihtiyaç çıkarsa. Bugün böyle bir ihtiyaç yok ve `code` bu ayrımı daha ucuza veriyor.

### Neden RFC 9457 (`application/problem+json`) değil

RFC 9457 Problem Details, HTTP API'leri için standart bir hata gövdesi tanımlar (`type`, `title`, `status`, `detail`, `instance` artı uzantı alanları). Standart olmasına rağmen kendi zarfımızı seçtik:

- **`type` bir URI olmalı ve dokümana çözülmesi beklenir.** Yayımlanmış bir doküman sitemiz yok. Çözülmeyen bir URN (`urn:fuelpay:error:invalid-json`) yazmak standardın faydasını değil sadece ağırlığını getirir.
- **`status` gövdede tekrar eder.** Status satırıyla gövde arasında iki kaynak doğar; proxy bir yanıtın status'ünü değiştirdiğinde ikisi çelişebilir.
- **Alan bazlı hatalar için standart yer yok.** 9457 bunu uzantı alanına bırakır; herkesin kendi `invalid-params`/`errors` anahtarını tanımlaması gerekir. Yani asıl ihtiyacımız olan kısımda interop faydası zaten doğmuyor.
- **`application/problem+json` content type'ı istemciyi ikiye böler.** `application/json` üzerinden dallanan istemci kodu ve araçlar (bazı SDK'lar, test araçları) ikinci bir tip görmek zorunda kalır.

**Hangi koşulda geçeriz:** FuelPay API'si kurum dışı taraflarca tüketilmeye başlarsa — özellikle problem+json bekleyen bir partner entegrasyonu veya OpenAPI spec'i yayımlanan public bir API çıkarsa — ya da önümüze problem+json üreten bir API gateway konur ve iki format yan yana dönmeye başlarsa. Geçiş maliyeti düşük tutuldu: eşleme mekanik olacak şekilde alanları seçtik.

| Bizim alan | RFC 9457 karşılığı |
|---|---|
| `code` | `type` URI'sinin son parçası (+ sabit `title`) |
| `message` | `detail` |
| `details[]` | `invalid-params` uzantısı |
| status satırı | `status` |

`code` değerleri sabit tutulduğu için bu eşleme tek bir yerde, `WriteError` içinde yapılabilir; handler'lar değişmez.

## Consequences

- Tüm hata yanıtları tek yerden, `httpjson.WriteError` üzerinden yazılır. Handler'lar `*httpjson.Error` döndürür, status ve serileştirme kararını vermezler.
- 404 ve 405 artık stdlib'in düz metin cevabını değil aynı zarfı döner (`internal/server.withJSONErrors`). Böylece istemcinin routing hatası ile handler hatası için ayrı parse yolu yok.
- `internal_error` dışındaki her kod istemci hatasıdır. `internal_error` gövdesi sabittir; asıl hata `slog.ErrorContext` ile log'lanır, böylece host adı veya SQL gibi iç detay istemciye sızmaz.
- Zarf sözleşmesi testle korunuyor: `TestWriteErrorEnvelopeShape` ham JSON'u birebir karşılaştırır, `server_test.go` üst seviyede `error`'ın tek anahtar olduğunu doğrular. Zarf kaldırılırsa 16 test düşer.
- Açık kalan nokta: `FieldErrors.Add` hâlâ serbest metin `issue` kabul ediyor (`"must not be empty"` gibi). `validation_failed` için de sabit bir issue kataloğu (`required`, `out_of_range`, `invalid_format` vb.) FTSM-005'te station API'sinin ilk gerçek doğrulama kurallarıyla birlikte belirlenecek.
- `request_id` üst seviye alanı FTSM-004'te eklendi (bkz. [ADR 0004](0004-middleware-and-request-id.md)): `{"error": {...}, "request_id": "..."}`. Context'te ID yoksa alan yazılmaz.
