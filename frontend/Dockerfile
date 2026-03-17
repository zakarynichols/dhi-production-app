# syntax=docker/dockerfile:1

FROM dhi.io/node:22-alpine3.22-dev AS builder

WORKDIR /app

COPY frontend/package*.json ./
RUN npm install

COPY frontend/ .

RUN npm run build

FROM dhi.io/nginx:1.29-alpine3.22 AS runtime

COPY --from=builder /app/dist /usr/share/nginx/html
