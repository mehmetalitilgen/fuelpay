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

## Terminals

Terminal yaşam döngüsü: `pending → active ⇄ suspended → decommissioned`. Yeni kayıtlar `pending` başlar.

### `POST /v1/stations/{id}/terminals`

İstasyona terminal kaydeder.

| Alan | Tip | Kural |
|---|---|---|
| `serial_number` | string | Zorunlu. 6–32 karakter, `A-Z 0-9 -`. **Tüm istasyonlar arasında** benzersiz |
| `model` | string | Zorunlu. En fazla 64 karakter |

`201 Created`, `Location: /v1/terminals/{id}`:

```json
{"id":"019a6b...","station_id":"019a6a...","serial_number":"PAX-A920-0001","model":"PAX A920","status":"pending","created_at":"...","updated_at":"..."}
```

`activated_at` sadece terminal en az bir kez aktive edildiyse yer alır.

| Status | `code` | Ne zaman |
|---|---|---|
| 400 | `validation_failed` | Alan kuralı ihlali |
| 404 | `not_found` | İstasyon yok |
| 409 | `conflict` | Seri numarası kayıtlı (`details`: `serial_number:already_exists`) |

### `GET /v1/terminals/{id}`

`200 OK` veya `404 not_found`.

### `GET /v1/stations/{id}/terminals`

İstasyonun terminalleri, eskiden yeniye: `{"items":[...]}`. Terminal yoksa `{"items":[]}`; istasyon yoksa `404`.

### Durum geçişleri

| Endpoint | Hedef durum | İzin verilen kaynak durumlar |
|---|---|---|
| `POST /v1/terminals/{id}/activate` | `active` | `pending`, `suspended` |
| `POST /v1/terminals/{id}/suspend` | `suspended` | `active` |
| `POST /v1/terminals/{id}/decommission` | `decommissioned` | `pending`, `active`, `suspended` |

- Gövde yok. `200 OK` ile güncel terminal döner.
- Terminal zaten hedef durumdaysa işlem **no-op**'tur: `200` döner, `updated_at` değişmez. Ağ hatası sonrası tekrar gönderilen istek güvenlidir.
- `activated_at` her aktivasyonda güncellenir (son aktivasyon zamanı); askıya alma veya devreden çıkarma onu silmez.
- `decommissioned` son durumdur.

| Status | `code` | `details` | Ne zaman |
|---|---|---|---|
| 404 | `not_found` | — | Terminal yok |
| 409 | `conflict` | `status:invalid_transition` | Geçiş yaşam döngüsünde yok (örn. `pending → suspended`) |
| 409 | `conflict` | `status:concurrent_update` | Durum eşzamanlı başka bir istekle sürekli değişti; tekrar deneyin |

### `GET /v1/terminals/{id}/events`

Terminalin değişiklik geçmişi (audit trail), eskiden yeniye: `{"items":[...]}`.

```json
{"items":[
  {"id":"...","terminal_id":"...","type":"registered","to_status":"pending","request_id":"3f9a...","occurred_at":"..."},
  {"id":"...","terminal_id":"...","type":"status_changed","from_status":"pending","to_status":"active","request_id":"ops-ticket-17","occurred_at":"..."}
]}
```

- Her kayıt ve durum değişikliği, değişiklikle **aynı transaction'da** bir event yazar; biri olmadan diğeri kaydedilmez.
- `request_id`, değişikliği yapan isteğin `X-Request-ID` değeridir; log'larla eşleştirmek için kullanılır.
- No-op geçişler (zaten hedef durumdaki terminal) ve reddedilen geçişler event üretmez.
- Terminal yoksa `404`.
