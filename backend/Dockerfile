# syntax=docker/dockerfile:1

FROM dhi.io/golang:1.26-alpine3.22-dev AS builder

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/api .

FROM dhi.io/golang:1.26-alpine3.22 AS runtime

WORKDIR /app

COPY --from=builder /app/api .

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"] || exit 1

ENTRYPOINT ["/app/api"]
