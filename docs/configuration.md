# Configuration

Environment variables; every one has a default. Personal settings (units, weekly goal, rest timer, notifications, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings.

| Variable | Default | What |
|---|---|---|
| `PORT`, `HOST` | `8080`, `0.0.0.0` | Listen address |
| `BASE_URL` | – | Public URL (links in pushes, the OIDC callback) |
| `DATA_DIR` | `./data` | Database, Core token, backups |
| `DB_PATH` | `$DATA_DIR/pocket-fitness.db` | SQLite file |
| `MAX_BODY_BYTES` | `67108864` | Largest JSON request, e.g. an export being imported (64 MB) |
| `TZ` | – | Time zone of the box: what "today" is, the morning message, the week |
| `USER_AGENT` | `PocketFitness/1.0 (+github…)` | Sent to the other apps and AI |
| `AUTH_MODE` | `local` | `local` (accounts), `proxy` (trust `AUTH_PROXY_HEADER`/`AUTH_PROXY_EMAIL_HEADER`), `none` |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | Seed the first admin while the database is empty |
| `SESSION_TTL`, `SECURE_COOKIES` | `720h`, `false` | Set `SECURE_COOKIES=true` behind HTTPS |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_NAME`, `OIDC_AUTO_CREATE` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL`, `NOTIFY_FORMAT` | – | Apprise (`…/notify/key`), ntfy topic URL or JSON webhook; format `apprise`/`ntfy`/`json` |
| `NOTIFY_HOUR` | `8` | Default hour of the daily message (each person can change theirs) |
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` | – | Your training in plain words, routines from a goal and workouts from a sentence, without Pocket AI |
| `CORE_URL`, `CORE_REGISTRATION_KEY`, `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY`, `METRICS_ENABLED`, `LOG_LEVEL`, `LOG_FORMAT` | `true`, `true`, `info`, `text` | |
| `DEMO_DATA` | `false` | Seed four routines on a weekly plan and six months of workouts into the owner account and, without Pocket Core, made-up linked apps (runs and weight from "Pocket Health", the weather, habits, playlists) — for screenshots and trying it out, never for real use |

Commands: `pocket-fitness backup [-o FILE]`, `pocket-fitness reset-password [-email] [-password]`, `pocket-fitness version`.

Connections to Pocket Health, Habits, Calendar, Weather and Music are made per person under Settings → Connected apps: through Pocket Core (nothing to paste) or directly with the app's URL and an API token.

Background work: every minute the morning messages (at each person's hour); every hour workouts left running for more than a day are finished (or dropped when nothing was ticked) and, once a day, planned sessions for the next two weeks are put on Pocket Calendar.
