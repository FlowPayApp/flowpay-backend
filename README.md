# FlowPay — API (Go + Gin)

Backend del MVP **FlowPay**: cobranza y seguimiento de **cobros** (montos que te deben). No reemplaza un sistema de facturación electrónica ni emite DTE ante el SII.

**Pagos** (portal público, Webpay, registro de pago) viven en **`flowpay-payments`**, no en este repo.

## Estructura del código

```
cmd/server/          → main
internal/
  routes/            → registro de rutas Gin
  controller/        → HTTP (cobros, dashboard, mensajería, webhooks)
  service/           → lógica de negocio + WhatsApp inbound
  repository/        → acceso a PostgreSQL
  jobs/              → recordatorios programados
  notify/            → SMTP, SMS, Twilio
  domain/            → reglas (estado de cobro, etc.)
```

## Requisitos

- Go 1.22+
- PostgreSQL 14+ (local, Docker o p. ej. Supabase)

## Base de datos

1. Crea la base y el esquema con los scripts en `../Mysql/postgresql_migration/` (`02_schema.sql` y `03_triggers.sql`). En Supabase suele bastar con ejecutarlos sobre la base `postgres` (esquema `public`).

2. **Legado MySQL:** la carpeta `Mysql/` conserva el esquema y migraciones MySQL por si necesitás exportar datos; el API ya no usa el driver MySQL.

3. Variable `FLOWPAY_DSN`: URI PostgreSQL, p. ej. `postgres://usuario:clave@host:5432/nombre_bd?sslmode=require` (Supabase casi siempre requiere `sslmode=require` y a veces host del pooler).

## Variables de entorno

Copia `.env.example` a `.env` y ajusta, o exporta en la terminal:

| Variable | Descripción |
|----------|-------------|
| `FLOWPAY_DSN` | URI PostgreSQL (`postgres://...`) |
| `FLOWPAY_ADDR` | Dirección HTTP (por defecto `:8080`) |
| `FLOWPAY_REMINDER_INTERVAL` | Intervalo del job de recordatorios (ej. `24h` o `1m` para pruebas) |

### Correo real (SMTP)

Rellena `FLOWPAY_SMTP_HOST`, `FLOWPAY_SMTP_PORT` (587), `FLOWPAY_SMTP_USER`, `FLOWPAY_SMTP_PASSWORD`, `FLOWPAY_SMTP_FROM`. Con **Gmail** hace falta una [contraseña de aplicación](https://support.google.com/accounts/answer/185833), no la clave normal si tienes 2FA.

Para pruebas sin enviar al cliente, usa `FLOWPAY_EMAIL_OVERRIDE=tu@correo.com`: todos los correos irán ahí.

### WhatsApp real (Twilio)

WhatsApp **no** permite enviar desde tu número personal con una API simple. Lo habitual en desarrollo/producción es **Twilio** (o la API de Meta). Configura `FLOWPAY_TWILIO_*` y `FLOWPAY_TWILIO_WHATSAPP_FROM` (número de Twilio o sandbox). En cuenta de prueba, el destino debe estar verificado. `FLOWPAY_WHATSAPP_OVERRIDE` fuerza el envío a tu móvil para pruebas.

### Adjuntos (PDF/imagen por cobro)

En el detalle de cobro puedes **subir un archivo**; se guarda en disco (`FLOWPAY_UPLOAD_DIR`) y, al enviar recordatorios por WhatsApp, Twilio lo descarga desde una URL pública. Debes definir **`FLOWPAY_PUBLIC_BASE_URL`** con la base HTTPS de tu API (en local, **ngrok** u otro túnel). Sin esa URL, el mensaje de texto se envía igual pero **sin archivo**. Los adjuntos no reemplazan un DTE ante el SII; son documentos de apoyo para el cliente.

## Ejecutar

```bash
go run ./cmd/server
```

La API queda en `http://127.0.0.1:8080`. Salud: `GET /health`.

### Endpoints principales

- `GET /api/dashboard?company_id=1`
- `GET|POST /api/clients?company_id=1`
- `GET|POST /api/charges?company_id=1`
- `GET /api/charges/:id`
- `GET /api/charges/:id/reminders`
- `POST /api/charges/:id/reminders` — enviar recordatorio manual (mock email/WhatsApp)
- `POST /api/charges/:id/attachment` — subir PDF/imagen
- `GET /api/public/attachments/:token` — descarga pública de adjunto (WhatsApp)

Los **pagos** (portal `/pay`, Webpay, `POST /api/payments`, tokens) están en el microservicio **`../flowpay-payments`** (`:8081`).

## Lógica de estado del cobro

- **Cobrado**: si `paid_at` está definido (p. ej. tras marcar pagado en UI o vía flowpay-payments).
- **Vencido**: sin cobrar y fecha de vencimiento **anterior** al día calendario actual.
- **Pendiente**: sin cobrar y vencimiento hoy o futuro.

## Job en segundo plano

Un ciclo periódico lista cobros próximos a vencer (ventana de 5 días) y vencidos sin cobrar, escribe en consola mensajes de ejemplo y registra filas en `reminders`. Los envíos de email y WhatsApp son **simulados** (solo logs) si no hay credenciales.

## Multi-empresa y escalado

- **Identidad (repo aparte)**: el servicio de login / JWT / SSO vive en **`../flowpay-sso`** para no mezclar autenticación con la lógica de cobranza. Ver ese README y `docs/INTEGRACION.md`.
- **Modelo de datos**: clientes y cobros ya van ligados a `company_id` (cada negocio FlowPay es una fila en `companies`). Varios negocios pueden convivir en la misma base.
- **Job de recordatorios**: corre **todas** las empresas listadas en `companies` en cada ciclo (no solo `company_id = 1`).
- **API**: hoy el tenant sigue viniendo por query `?company_id=` o valor por defecto en config — **para producción multi-cliente** hace falta autenticación (sesión/JWT) y tomar el `company_id` solo del usuario autorizado, sin confiar en el query string.
- **Rendimiento**: con muchos cobros, el índice `(company_id, due_date)` en `charges` ayuda; está en `create_tables.sql` y en `migration_charges_company_due_index.sql` para BDs ya creadas.
- **Siguientes pasos típicos**: tabla `users` + relación usuario↔empresa, roles, límites por plan, paginación en listados, caché opcional y réplicas de lectura cuando el volumen lo exija.

## Frontend

Ver `../flowpay-frontend/README.md`. En desarrollo, Vite hace proxy de `/api` y `/health` hacia este servidor.
