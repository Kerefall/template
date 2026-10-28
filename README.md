# TripGo — Trip Service

Сервис поездок для такси (MVP). Лабораторная работа 1: HTTP API и PostgreSQL. Лабораторная работа 2: слои, тесты, линтер и observability.

Сервис создаёт поездку, отдаёт её и завершает. Данные лежат в PostgreSQL, схема
накатывается миграциями, создание и завершение поездки пишут в две таблицы в
одной транзакции.

## Требования

- Go 1.26+
- [`tripgoctl`](https://github.com/course-go-autumn-2026/course-infra) — поднимает PostgreSQL
- `golangci-lint` — только для `make lint`

`oapi-codegen` и `goose` отдельно ставить не нужно: они подключены в `go.mod`
как инструменты (`go tool ...`).

## Быстрый старт

```bash
tripgoctl cluster start          # один раз на машине
tripgoctl environment start      # поднимет PostgreSQL и создаст .env
make migrate                     # накатить миграции
make run                         # запустить сервис
```

Адреса берутся из `.env`, который создаёт `tripgoctl`. Файлы `.env` и `.tripgo/`
в git не попадают. Список переменных с безопасными значениями по умолчанию лежит
в `.env.example`.

## Переменные окружения

| Переменная | Обязательная | Что задаёт |
|---|---|---|
| `HTTP_ADDR` | да | адрес HTTP-сервера, например `:8080` |
| `LOG_LEVEL` | да | уровень логов: `debug`, `info`, `warn`, `error` |
| `SHUTDOWN_TIMEOUT` | да | общий бюджет на остановку, например `10s` |
| `DATABASE_URL` | да | строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS` | да | максимум соединений в пуле |
| `DATABASE_MIN_CONNS` | да | минимум соединений в пуле |
| `DATABASE_MAX_CONN_LIFETIME` | да | время жизни соединения, например `30m` |
| `DATABASE_CONNECT_TIMEOUT` | да | таймаут подключения к бд (и первого `Ping`) |
| `DATABASE_QUERY_TIMEOUT` | да | таймаут операции с бд в одном запросе |
| `HTTP_READ_TIMEOUT` | нет, `10s` | `ReadTimeout` сервера |
| `HTTP_READ_HEADER_TIMEOUT` | нет, `5s` | `ReadHeaderTimeout` сервера |
| `HTTP_WRITE_TIMEOUT` | нет, `10s` | `WriteTimeout` сервера |
| `HTTP_IDLE_TIMEOUT` | нет, `60s` | `IdleTimeout` сервера |

Телеметрия настраивается стандартными переменными OpenTelemetry (значения по
умолчанию лежат в `.env.example`, адрес коллектора приходит в `.env`):

| Переменная | Что задаёт |
|---|---|
| `OTEL_SERVICE_NAME` | имя сервиса, `trip-service` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | адрес коллектора |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | протокол: `http/protobuf` или `grpc` |
| `OTEL_RESOURCE_ATTRIBUTES` | атрибуты ресурса (`service.namespace`, `deployment.environment`) |
| `OTEL_TRACES_SAMPLER` | сэмплирование трейсов, например `parentbased_always_on` |

Если коллектор недоступен, сервис работает штатно: ошибки экспорта пишутся в лог
уровнем `warn`.

Если обязательная переменная не задана или значение некорректно, сервис не
стартует и завершается с кодом 1 и понятным сообщением. Таймауты HTTP сделаны
необязательными, потому что `tripgoctl` кладёт в `.env` только переменные базы и
сервера.

## Команды

| Команда | Что делает |
|---|---|
| `make deps` | ставит зависимости этой работы (gomock, testify, OpenTelemetry, otelpgx) и делает `go mod tidy`; нужна сеть |
| `make generate` | OpenAPI-код (`api/oapi-codegen.yaml`) и моки (`go generate ./...`) |
| `make migrate` | накатывает миграции (`goose up`) |
| `make migrate-down` | откатывает одну последнюю миграцию (`goose down`) |
| `make run` | запускает сервис локально |
| `make test` | `go test -race` по всему проекту, в конце печатает покрытие `usecase` и `domain` |
| `make test-integration` | интеграционные тесты репозитория на реальном PostgreSQL |
| `make lint` | `golangci-lint run ./...` |
| `make fmt` | `golangci-lint fmt` (gofumpt и gci) |
| `make check-layers` | проверяет, что `internal/domain` не импортирует инфраструктуру |

Сгенерированный код (`internal/generated/api.gen.go`) и моки (`mocks_test.go`)
коммитятся, руками не правятся. Контракт в `contracts/` не менялся. В генерации
оставлены только операции этой работы: `createTrip`, `getTrip`, `finishTrip`,
`health`, `ready`.

## Архитектура

```
cmd/service/main.go          читает конфиг, собирает контейнер, запускает, ждёт сигнала
internal/
  config/                    разбор и валидация переменных окружения
  container/                 создаёт все зависимости и связывает их между собой
  domain/trip/               поездка: статусы, переход active -> completed, правила, ошибки домена
  model/                     структуры, которые ходят между слоями
  usecase/trip/{create,get,finish}/   сценарии: usecase.go, contract.go, тесты, моки
  http/
    trip/{create,get,finish}/         ручки: handler.go, contract.go, convert.go, тесты, моки
    health/, ready/                   liveness и readiness
    domainerror/             единственное место, где ошибка превращается в Problem
    middleware/              Recover -> Tracing -> Logging -> Metrics
    router/                  связывает сгенерированный интерфейс с хендлерами
    respond/                 запись JSON-ответа
  repository/trip/           хранение поездок в PostgreSQL (pgx + squirrel)
  infra/postgres/            пул pgx, трейсинг запросов
  infra/postgres/transaction/  менеджер транзакций
  infra/telemetry/           OpenTelemetry и slog
  generated/                 код, сгенерированный из OpenAPI
migrations/                  SQL-миграции goose
```

**Кто от кого зависит.** Зависимости направлены внутрь: транспорт и репозитории
знают про usecase, usecase знает про домен, домен не знает ни про кого. Это
проверяет `depguard` в `.golangci.yml`: `domain` не импортирует `net/http`,
`chi`, `pgx`, `squirrel`, `otel` и сгенерированный код, `usecase` не импортирует
`net/http`, `chi`, `pgx`, `squirrel` и сгенерированный код, `http/*` не
импортирует `repository/*` и `infra/*`. Пакет на действие: у каждого
usecase и каждой ручки свой пакет с одним именем на всех слоях.

**Где объявлены интерфейсы и почему.** Интерфейсы лежат в `contract.go` пакета,
который их использует, и неэкспортируемые: `tripRepository`, `txManager` и
`metrics` у usecase, `usecase` у хендлеров, `pinger` у `ready`. Потребитель сам
формулирует, что ему нужно, поэтому интерфейсы узкие (у `get` один метод) и их
легко мокать. Реализаций может быть несколько, а репозиторий и менеджер
транзакций про эти интерфейсы не знают. Когда в работе 4 появится второй
транспорт (gRPC), usecase не меняется: нужен новый пакет в `internal/` с
хендлерами, который вызывает те же usecase, и своя запись в `container`.

**Сборка.** Все зависимости создаются в `internal/container`, вручную, без
DI-фреймворка. Только там встречаются `postgres.NewPool` и конструкторы
репозиториев; `main.go` ничего не создаёт сам.

**Ошибки.** Доменные ошибки (`ErrTripNotFound`, `ErrTripCompleted`,
`ErrDriverBusy`, `ValidationError`) объявлены в `internal/domain/trip`.
`domainerror.Problem` — единственный `switch` по `errors.Is`/`errors.As`,
который превращает ошибку в `Problem` (RFC 9457); хендлер передаёт ему ошибку
usecase и не разбирает её сам. Всё незнакомое уходит клиенту как `500` без
подробностей, подробности пишутся в лог и в спан.

**Отступления от раскладки курса.** Пакеты `health` и `ready` лежат в `http/`
рядом с ручками поездок, а общий помощник записи JSON вынесен в
`http/respond`. `.golangci.yml` написан по требованиям работы, потому что
исходного `config/golangci.yml` курса в репозитории нет.

## Тесты

- `make test` запускает всё с `-race` и печатает покрытие `internal/usecase/...` и
  `internal/domain/...` (должно быть не ниже 50%). Тесты не зависят от порядка.
- Доменные правила (`New`, `Finish`) проверяют обычные table-driven тесты без
  моков, usecase — gomock (`MocktripRepository`, `MocktxManager`, `Mockmetrics`),
  хендлеры — `httptest` плюс мок usecase, маппинг ошибок — table-driven тест на
  `domainerror.Problem`.
- `make test-integration` (тег `integration`) работает с базой из `tripgoctl
  environment start`, адрес берётся из `DATABASE_URL`, миграции должны быть
  накатаны. Каждый тест создаёт нового водителя, убирает за собой свои данные и
  проходит при повторном запуске. Один из тестов проверяет, что при ошибке второй
  вставки откатывается и первая.
- Моки генерирует `mockgen` по `//go:generate` (`go tool mockgen`) в
  `mocks_test.go` рядом с кодом: `make generate`.

## Телеметрия

- **Логи.** `log/slog`, JSON, stdout, уровень из `LOG_LEVEL`. Логгер передаётся
  зависимостью, глобального `slog.Default()` в коде нет. Если запись сделана
  внутри спана, в неё добавляются `trace_id` и `span_id`. Middleware пишет одну
  запись `request handled` на запрос (`method`, `route`, `status`, `duration_ms`),
  `Info` для 2xx и 4xx и `Error` только для 5xx. Тела запроса и ответа не
  логируются. Создание и завершение поездки логируются отдельно, в usecase.
- **Трейсы.** Корневой спан на запрос с именем `POST /api/v1/trips` (метод и шаблон
  маршрута), каждый запрос в БД — отдельный спан от `otelpgx`, подключённого на
  уровне пула, с атрибутами `db.operation.name` и `db.collection.name`.
  5xx помечают спан `codes.Error` и `error.type`. Контекст W3C TraceContext
  продолжается из заголовков.
- **Метрики.** `http.server.request.duration`, `http.server.active_requests`,
  `db.client.operation.duration`, `tripgo.trips.created`, `tripgo.trips.completed`.
  В `http.route` кладётся только шаблон маршрута: если подставлять туда
  конкретный `tripId`, на каждую поездку заведётся своя временная серия, и метрика
  перестанет агрегироваться.
- **Порядок middleware:** `Recover -> Tracing -> Logging -> Metrics -> роутер`.
  `Recover` ловит панику, логирует её со стектрейсом и отдаёт `500 internal_error`.

## API

| Метод | Путь | Ответы |
|---|---|---|
| `POST` | `/api/v1/trips` | `201`, `400`, `409`, `500` |
| `GET` | `/api/v1/trips/{tripId}` | `200`, `400`, `404`, `500` |
| `POST` | `/api/v1/trips/{tripId}/finish` | `200`, `400`, `404`, `409`, `500` |
| `GET` | `/health` | `200` |
| `GET` | `/ready` | `200`, `503` |

Ошибки отдаются по RFC 9457 в `application/problem+json`. Клиенту нужно
ориентироваться на поле `code`:

| HTTP | `code` | Когда |
|---|---|---|
| 400 | `invalid_request` | тело не прошло валидацию или `tripId` не UUID |
| 404 | `trip_not_found` | поездки нет |
| 409 | `trip_completed` | поездка уже завершена |
| 409 | `driver_busy` | у водителя уже есть активная поездка |
| 500 | `internal_error` | всё остальное (подробности только в логе) |

`/health` и `/ready` отдают `application/json` (схема `HealthResponse`).
`/health` отвечает, пока жив процесс, и в бд не ходит. `/ready` делает `Ping` базы
и отвечает `503`, если она недоступна. Поэтому оркестратор не перезапускает
сервис из-за недоступной базы, а просто перестаёт слать на него трафик.

Незаполненные необязательные поля (`finished_at`, `last_position_at`) в ответе
опускаются: в контракте они `nullable` и необязательные, так генерирует код
`oapi-codegen`.

## Решения

**Менеджер транзакций** (`internal/infra/postgres/transaction`). `Do(ctx, fn)` открывает
транзакцию, кладёт её в `context` и вызывает `fn` с обогащённым контекстом. Если
`fn` вернула `nil`, делается `COMMIT`; если ошибку или паникнула, `ROLLBACK`, а
ошибка или паника пробрасываются дальше. Репозиторий достаёт исполнителя из
контекста: есть транзакция — работает через неё, нет — через пул. Транзакция не
передаётся аргументом. Вложенный `Do` видит транзакцию в контексте и
переиспользует её, вторая не открывается. Откат выполняется с контекстом без
отмены (`context.WithoutCancel`) и своим таймаутом, чтобы он прошёл, даже если
запрос уже отменён.

**Уровень изоляции — `READ COMMITTED`.** Оба инварианта держатся самой базой, а
не изоляцией транзакций, поэтому более строгие уровни не нужны (они добавили бы
ошибки сериализации и повторы): запрет двух активных поездок обеспечивает
уникальный индекс, а от двойного завершения защищает условие в самом `UPDATE`.
Уровень задаётся явно при старте транзакции в `Do`.

**Запрет двух активных поездок у водителя.** Частичный уникальный индекс
`trips_one_active_per_driver_idx ON trips (driver_id) WHERE status = 'active'`.
Проверка `SELECT` перед `INSERT` не годится: при параллельных запросах оба
пройдут её до вставки. С индексом второй `INSERT` ждёт первый и после его
коммита получает ошибку `23505`, которую репозиторий превращает в
`domain.ErrDriverBusy`, а хендлер — в `409 driver_busy`. Индекс освобождается,
когда поездка завершается.

**Защита от гонки в `finish`.** Поездка читается через `SELECT ... FOR UPDATE`
внутри транзакции, строка блокируется до коммита. Второй параллельный вызов
дожидается первого, читает уже `completed` и получает `409 trip_completed`.
`finished_at` второй раз не перезаписывается, а «поездки нет» (`404`) и «уже
завершена» (`409`) различаются самим чтением.

**Журнал статусов.** `trip_status_history` пишется в той же транзакции и при
создании (`from_status = NULL`, `to_status = active`), и при завершении.

**Валидация.** Выполняется до обращения к бд: поля обязательны (проверяются по
сырому JSON, `null` считается отсутствием), неизвестные поля запрещены,
UUID непустые, координаты в допустимых диапазонах, `price >= 0`. Цена хранится в
целых рублях (`bigint`), как в контракте.

**Остановка.** По `SIGINT`/`SIGTERM` сервис перестаёт принимать новые запросы,
дожидается текущих и закрывает пул соединений. Весь останов укладывается в
`SHUTDOWN_TIMEOUT`, по его истечении сервис выходит с кодом 1 и пишет об этом
в лог.

## Как проверить

```bash
tripgoctl environment start && make migrate && make run
```

Нужен `jq` (`sudo apt install jq`). На одного водителя разрешена только одна
активная поездка, поэтому для каждого прогона берём нового водителя:

```bash
# новый водитель и тело запроса (Linux/WSL)
DRIVER_ID=$(cat /proc/sys/kernel/random/uuid)
cat > /tmp/trip.json <<JSON
{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
 "driver_id":"$DRIVER_ID",
 "start_point":{"latitude":59.9398,"longitude":30.3146},
 "end_point":{"latitude":59.9290,"longitude":30.3626},
 "price":1450}
JSON

# создание: 201 и заголовок Location; id поездки сохраняем в TRIP_ID
TRIP_ID=$(curl -sS -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' -d @/tmp/trip.json | jq -r .id)
echo "$TRIP_ID"          # должен быть UUID, а не пусто и не null

# повтор на том же водителе: 409 driver_busy
curl -sS -i -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' -d @/tmp/trip.json

# получение: 200
curl -sS localhost:8080/api/v1/trips/$TRIP_ID | jq

# завершение и повторное завершение: 200, затем 409 trip_completed
curl -sS -i -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish
curl -sS -i -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish

# tripId не UUID: 400 invalid_request
curl -sS -i localhost:8080/api/v1/trips/abc
```

Если `echo "$TRIP_ID"` печатает пустую строку или `null`, значит, создание не
прошло (скорее всего, `409 driver_busy` на уже занятом водителе) — запустите
первый блок заново с новым `DRIVER_ID`.

Гонка на создании (для нового водителя — ровно один `201`, остальные `409`):

```bash
DRIVER_ID=$(cat /proc/sys/kernel/random/uuid)
sed -i "s/\"driver_id\":\"[^\"]*\"/\"driver_id\":\"$DRIVER_ID\"/" /tmp/trip.json
seq 20 | xargs -P20 -I{} curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST localhost:8080/api/v1/trips -H 'content-type: application/json' \
  -d @/tmp/trip.json | sort | uniq -c
```

Атомарность создания: временно добавить в `trip_status_history` ограничение,
из-за которого вторая вставка упадёт, и убедиться, что в `trips` ничего не
осталось.

```sql
ALTER TABLE trip_status_history
    ADD CONSTRAINT tmp_fail CHECK (reason <> 'trip created') NOT VALID;
-- создать поездку: ответ 500, а select count(*) from trips; не вырос
ALTER TABLE trip_status_history DROP CONSTRAINT tmp_fail;
```

Прочее: `make migrate-down` откатывает последнюю миграцию (повторить для обеих);
`/ready` отдаёт `503`, если остановить PostgreSQL, и снова `200` после старта;
по `SIGTERM` сервис дожидается текущего запроса и выходит за `SHUTDOWN_TIMEOUT`.

## Что не входит в эту работу

Фоновый опрос координат, gRPC и брокер — следующие работы. Задания со
звёздочкой (идемпотентность `Idempotency-Key`, `Dockerfile`, мутационное
тестирование) не делались: заголовок `Idempotency-Key` сейчас игнорируется.
