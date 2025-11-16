# PR Reviewer Assignment Service

Тестовое задание для стажировки в Avito.

## Требования

- Go 1.25+
- Docker + docker compose
- golangci-lint

## Конфигурация

Настройки читаются из окружения (см. `pkg/config/config.go`):

| Переменная             | Назначение                    | Значение по умолчанию                                      |
| ---------------------- | ----------------------------- | ---------------------------------------------------------- |
| `PORT`                 | HTTP-порт сервиса             | `8080`                                                     |
| `DATABASE_URL`         | строка подключения PostgreSQL | `postgres://postgres:postgres@db:5432/app?sslmode=disable` |
| `SERVER_READ_TIMEOUT`  | `ReadTimeout` HTTP-сервера    | `15s`                                                      |
| `SERVER_WRITE_TIMEOUT` | `WriteTimeout` HTTP-сервера   | `15s`                                                      |
| `SERVER_IDLE_TIMEOUT`  | `IdleTimeout` HTTP-сервера    | `60s`                                                      |

Пример готового `.env` лежит в корне проекта.

## Сборка и запуск

```bash
# генерация oapi-кода
make gen

# сборка бинарного файла
make build

# локальный запуск
make run

# запуск тестирования
make test

# запуск линтера
make lint

# Запуск docker-compose
make up

# остановка docker-compose
make down
```

Сервис доступен на `http://localhost:8080`.

## Линтер

Для запуска проверки выполните команду:

```bash
make lint
```

## Тестирование

Команда прогоняет интеграционные и unit-тесты. Перед запуском необходимо убедиться, что запущен Docker Compose

```bash
make test
```
