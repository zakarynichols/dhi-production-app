# syntax=docker/dockerfile:1

FROM dhi.io/node:24-debian12-dev AS builder

WORKDIR /app

COPY package*.json ./
RUN npm install

COPY . .

RUN npm run build

FROM dhi.io/nginx:1 AS runtime

COPY --from=builder /app/build /usr/share/nginx/html

COPY nginx.conf /etc/nginx/nginx.conf
COPY conf.d/default.conf /etc/nginx/conf.d

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["wget", "--no-vagrant", "--tries=1", "--spider", "http://localhost:80/health"] || exit 1

ENTRYPOINT ["nginx", "-g", "daemon off;"]
