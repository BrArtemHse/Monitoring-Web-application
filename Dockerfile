# Сборка бинарников
FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod tidy

COPY . .

# Собираем два бинарника: приложение и монитор
RUN go build -o /out/helloworld ./cmd/app \
 && go build -o /out/monitor ./cmd/monitor

# Финальный минимальный образ
FROM alpine:3.20

WORKDIR /app

# бинарники
COPY --from=builder /out/helloworld /app/helloworld
COPY --from=builder /out/monitor /app/monitor

# пример конфигурации (можно переопределить через volume)
COPY config.json /app/config.json

# лог-директория
RUN mkdir -p /var/log

ENV MONITOR_CONFIG=/app/config.json

# Запускаем монитор (он сам запустит веб-приложение как подпроцесс)
ENTRYPOINT ["/app/monitor"]
