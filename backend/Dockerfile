# Сборка из корня репозитория: docker build -f backend/Dockerfile .
# (нужны мок-данные команды из docs/04-mock-data и страница из frontend/web)
FROM golang:1.27-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/server /app/server
COPY docs/04-mock-data /app/mock-data
COPY frontend/web /app/web
ENV MOCK_DATA_DIR=/app/mock-data STATIC_DIR=/app/web PORT=8080 TZ=Europe/Moscow
EXPOSE 8080
USER nobody
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://localhost:8080/api/health || exit 1
CMD ["/app/server"]
