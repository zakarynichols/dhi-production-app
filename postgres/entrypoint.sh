#!/bin/bash
set -e

# Generate SSL certs if they don't exist
if [ ! -f "/certs/server.crt" ]; then
    openssl req -new -x509 -days 365 -nodes \
        -text -out /certs/server.crt \
        -keyout /certs/server.key \
        -subj "/CN=postgres" 2>/dev/null
    chmod 600 /certs/server.key
    chown postgres:postgres /certs/server.key
fi

# Start PostgreSQL with SSL
exec docker-entrypoint.sh postgres \
    -c ssl=on \
    -c ssl_cert_file=/certs/server.crt \
    -c ssl_key_file=/certs/server.key
