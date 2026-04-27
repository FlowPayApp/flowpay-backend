# FlowPay — API (Go + Gin)

Backend del MVP **FlowPay**: cobranza y seguimiento de **cobros** (montos que te deben). No reemplaza un sistema de facturación electrónica ni emite DTE ante el SII.

## Requisitos

- Go 1.22+
- MySQL 8 (local o Docker)

## Base de datos

1. Levanta MySQL. Con Docker, desde la carpeta `Mysql`:

   ```bash
   docker compose up -d
   ```

   Esto crea la base `flowpay`, usuario `flowpay` / contraseña `flowpay`, puerto `3306`, y aplica `create_tables.sql` y `seed_data.sql`.

2. Si instalas MySQL a mano, ejecuta en orden:

   ```bash
   mysql -u root -p < ../Mysql/create_tables.sql
   mysql -u root -p < ../Mysql/seed_data.sql
   ```

   Crea el usuario si hace falta:

   ```sql
   CREATE USER IF NOT EXISTS 'flowpay'@'%' IDENTIFIED BY 'flowpay';
   GRANT ALL PRIVILEGES ON flowpay.* TO 'flowpay'@'%';
   FLUSH PRIVILEGES;
   ```

3. Si tu BD antigua aún tiene la tabla `invoices`, ejecuta **una vez** `../Mysql/migration_rename_invoices_to_charges.sql` antes de alinear el código.

## Variables de entorno

Copia `.env.example` a `.env` y ajusta, o exporta en la terminal:

| Variable | Descripción |
|----------|-------------|
| `FLOWPAY_DSN` | DSN MySQL (por defecto usuario `flowpay`, BD `flowpay`) |
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
- `POST /api/payments` — `{ "charge_id": 1, "amount": 12345 }` (MVP: debe cubrir el total)

## Lógica de estado del cobro

- **Cobrado**: si `paid_at` está definido (p. ej. tras `POST /payments`).
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
