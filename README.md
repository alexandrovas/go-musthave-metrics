# go-musthave-metrics-tpl

Шаблон репозитория для трека «Сервер сбора метрик и алертинга».

## Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без префикса `https://`) для создания модуля.

## Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-metrics-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/v2 .github
```

Затем добавьте полученные изменения в свой репозиторий.

## Запуск автотестов

Для успешного запуска автотестов называйте ветки `iter<number>`, где `<number>` — порядковый номер инкремента. Например, в ветке с названием `iter4` запустятся автотесты для инкрементов с первого по четвёртый.

При мёрже ветки с инкрементом в основную ветку `main` будут запускаться все автотесты.

Подробнее про локальный и автоматический запуск читайте в [README автотестов](https://github.com/Yandex-Practicum/go-autotests).

## Структура проекта

Приведённая в этом репозитории структура проекта является рекомендуемой, но не обязательной.

Это лишь пример организации кода, который поможет вам в реализации сервиса.

При необходимости можно вносить изменения в структуру проекта, использовать любые библиотеки и предпочитаемые структурные паттерны организации кода приложения, например:
- **DDD** (Domain-Driven Design)
- **Clean Architecture**
- **Hexagonal Architecture**
- **Layered Architecture**

## Бенчмарки

Бенчмарки измеряют скорость и количество аллокаций ключевых компонентов системы:

| Компонент | Бенчмарк | Что измеряет |
| --- | --- | --- |
| `internal/service` | `BenchmarkMetricsService_GetAllMetrics` | сбор всех метрик из хранилища в срез |
| `internal/service` | `BenchmarkMetricsService_UpdateMetric` | обновление одной метрики |
| `internal/repository` | `BenchmarkMemStorage_UpdateBatch` | пакетная запись метрик |
| `internal/repository` | `BenchmarkMemStorage_Gauges` | копирование среза gauge-метрик |
| `internal/repository` | `BenchmarkMemStorage_Save` | сериализация состояния в JSON-файл |
| `internal/collectors` | `BenchmarkBaseCollector_Drain` | снятие снимка накопленных метрик |
| `internal/collectors` | `BenchmarkRuntime_Poll` | опрос runtime-метрик |
| `internal/sign` | `BenchmarkCompute` | вычисление HMAC-SHA256 подписи |
| `internal/middleware` | `BenchmarkCompression_*` | gzip-сжатие ответов и распаковка запросов |

Запуск:

```bash
go test -run '^$' -bench . -benchmem ./internal/...
```

## Профилирование памяти

Профиль потребления памяти снимается профилировщиком `pprof` по бенчмарку
`BenchmarkMetricsService_GetAllMetrics` (самый аллокационно-нагруженный путь —
сбор всех метрик). Профили лежат в директории `profiles/`:

- `profiles/base.pprof` — до оптимизации;
- `profiles/result.pprof` — после оптимизации.

Как воспроизвести:

```bash
mkdir -p profiles
go test -run '^$' -bench 'BenchmarkMetricsService_GetAllMetrics' -benchmem \
  -memprofile profiles/base.pprof ./internal/service/
# ...внести изменения в код...
go test -run '^$' -bench 'BenchmarkMetricsService_GetAllMetrics' -benchmem \
  -memprofile profiles/result.pprof ./internal/service/
```

### Что было оптимизировано

Анализ профиля (`pprof top`, `list`, `peek`) показал, что
`service.(*metricsService).GetAllMetrics` отвечает за ~99.84% всех аллокаций.
Причина — построение результирующего среза `[]models.Metrics` без
предварительного выделения ёмкости: `append` многократно переаллоцировал
растущий срез.

Исправление — выделение ёмкости заранее:

```go
result := make([]models.Metrics, 0, len(gauges)+len(counters))
```

Результат для `BenchmarkMetricsService_GetAllMetrics`:

| Метрика | До | После |
| --- | --- | --- |
| `ns/op` | 7 923 810 | 1 857 699 (~4.3× быстрее) |
| `B/op` | 35 311 579 | 7 206 186 (~4.9× меньше) |
| `allocs/op` | 100 029 | 100 001 |

### Проверка изменений

```bash
pprof -top -diff_base=profiles/base.pprof profiles/result.pprof
```

Вывод (отрицательные значения означают снижение потребления памяти):

```text
File: service.test
Type: alloc_space
Time: 2026-10-25 18:16:04 MSK
Showing nodes accounting for -493.39MB, 9.94% of 4963.45MB total
Dropped 25 nodes (cum <= 24.82MB)
      flat  flat%   sum%        cum   cum%
 -493.39MB  9.94%  9.94%  -493.39MB  9.94%  github.com/alexandrovas/go-musthave-metrics/internal/service.(*metricsService).GetAllMetrics
         0     0%  9.94%  -493.86MB  9.95%  github.com/alexandrovas/go-musthave-metrics/internal/service.BenchmarkMetricsService_GetAllMetrics
         0     0%  9.94%  -493.86MB  9.95%  testing.(*B).run1.func1
         0     0%  9.94%  -493.86MB  9.95%  testing.(*B).runN
```

> `pprof` доступен как `go tool pprof` (идёт в комплекте с Go) либо
> устанавливается отдельно: `go install github.com/google/pprof@latest`.
