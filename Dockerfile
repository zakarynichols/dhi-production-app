# syntax=docker/dockerfile:1

FROM dhi.io/node:24-debian12-dev AS builder

WORKDIR /app

COPY frontend/package*.json ./
RUN npm install

COPY frontend/ .

RUN npm run build

FROM dhi.io/golang:1.24-debian12-dev AS api-builder

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/api ./cmd/api

FROM dhi.io/golang:1.24-debian12 AS runtime

WORKDIR /app

COPY --from=api-builder /app/api .
COPY --from=builder /app/build ./static

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"] || exit 1

ENTRYPOINT ["/app/api"]
