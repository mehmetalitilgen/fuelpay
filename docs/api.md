# FuelPay TSM API

Tüm endpoint'ler JSON alır ve döner. Hata formatı ve kod kataloğu: [README › Hata formatı](../README.md#hata-formatı), [ADR 0003](adr/0003-error-response-format.md). Her yanıt `X-Request-ID` header'ı taşır.

## Stations

### `POST /v1/stations`

İstasyon oluşturur.

| Alan | Tip | Kural |
|---|---|---|
| `code` | string | Zorunlu. 3–32 karakter, `A-Z 0-9 -`. Tüm istasyonlar arasında benzersiz |
| `name` | string | Zorunlu. En fazla 100 karakter (baş/son boşluk kırpılır) |
| `city` | string | Zorunlu. En fazla 64 karakter |
| `address` | string | Opsiyonel. En fazla 200 karakter |

```http
POST /v1/stations
Content-Type: application/json

{"code":"IST-0001","name":"Kadıköy","city":"İstanbul"}
```

`201 Created`, `Location: /v1/stations/{id}`:

```json
{"id":"019a6b2e-...","code":"IST-0001","name":"Kadıköy","city":"İstanbul","created_at":"2026-11-09T09:00:00.123456Z"}
```

| Status | `code` | Ne zaman |
|---|---|---|
| 400 | `validation_failed` | Alan kuralı ihlali (`details`: `required`, `too_short`, `too_long`, `invalid_format`) |
| 409 | `conflict` | `code` başka bir istasyonda kullanılıyor (`details`: `code:already_exists`) |

### `GET /v1/stations/{id}`

`200 OK` ile istasyonu döner, yoksa `404 not_found`.
