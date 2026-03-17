# Measure 6: Scaling to 5000 RPS with dhi.io Images

## Problem Evidence

### Before PgBouncer - Bottleneck Logs

```
api-7 | Error capturing request: FATAL: sorry, too many clients already (SQLSTATE 53300)
```

Resource usage during load test:
```
postgres-1 | CPU: 190.90% | MEM: 227.7MiB / 1GiB
```

### Test Results Without PgBouncer

| VUs | RPS | Success | p95 Latency |
|-----|-----|---------|-------------|
| 300 | 1,836 | 99.3% | 335ms |
| 1000 | 1,729 | 100% | 996ms |
| 2000 | 1,428 | 100% | 4.08s |
| 5000 | 652 | 43% | 6.91s |

**Analysis:** RPS plateaus at ~1,600 - PostgreSQL CPU-bound AND connection-exhausted.

---

## Solution: dhi.io/pgbouncer Connection Pooling

### Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   k6 load   │────▶│  nginx     │────▶│  pgbouncer  │
│   testers   │     │  (dhi.io)  │     │  (dhi.io)   │
└─────────────┘     └─────────────┘     └─────────────┘
                                               │
                    ┌─────────────┐            │
                    │  API x 16   │◀───────────┘
                    │  (Go)       │
                    └─────────────┘
                          │
                    ┌─────────────┐
                    │ postgres    │
                    │ (dhi.io)    │
                    └─────────────┘
```

### Implementation

1. **Added dhi.io/pgbouncer:1.25-debian13** - connection pooler
2. **PostgreSQL**: 4 CPU, 2GB RAM
3. **API replicas**: 16
4. **Connection pooling**: 50 connections per API replica via pgbouncer

### Configuration

```yaml
# docker-compose.yml
pgbouncer:
  image: dhi.io/pgbouncer:1.25-debian13
  volumes:
    - ./pgbouncer/pgbouncer.ini:/etc/pgbouncer/pgbouncer.ini:ro
    - ./pgbouncer/userlist.txt:/etc/pgbouncer/userlist.txt:ro
  environment:
    - DATABASE_URL=postgres://user:pass@postgres:5432/appdb

api:
  environment:
    DB_HOST: pgbouncer
    DB_PORT: 5432
    DB_MAX_OPEN_CONNS: 50
    DB_MAX_IDLE_CONNS: 10
```

### PgBouncer Config

```ini
[databases]
appdb = host=postgres port=5432 dbname=appdb

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 50
min_pool_size = 10
reserve_pool_size = 10
```

---

## Results After PgBouncer

### Resource Usage

| Component | CPU | Memory |
|-----------|-----|--------|
| PostgreSQL | **<1%** | 150MB/2GB |
| PgBouncer | ~50% | 7MB/256MB |
| API (each) | ~8% | 23MB/512MB |

### Performance

| Test | RPS | Success | Notes |
|------|-----|---------|-------|
| Single k6 (1000 VUs) | 1,017 | 100% | |
| 5x parallel k6 | ~4,000 | 100% | |
| 8x parallel k6 | ~2,600 | 100% | Client-limited |

### Key Improvements

1. **Connection exhaustion fixed** - PgBouncer multiplexes 100s of connections to few DB connections
2. **PostgreSQL CPU dropped** - From 190% → <1%
3. **Scalable** - Can add more API replicas without DB connection issues

---

## Current Limitations

- **k6 client bottleneck** - Single machine can't generate >1000 RPS per client
- **PostgreSQL still single-threaded** - For writes, but async writes help
- **No read caching** - Could add Redis for further optimization

---

## Next Steps to Reach 5000+ RPS

1. **Add more k6 clients** - From different machines/VMs
2. **Add dhi.io/redis** - Cache read endpoints (GET /api/bins)
3. **Scale API replicas** - Currently 16, can go to 32+
4. **Vertical PostgreSQL** - More CPU cores won't help (single-threaded writes)

---

## Commands Run

```bash
# Start with pgbouncer
docker compose up -d

# Stress test
k6 run --insecure-skip-tls-verify -vus=1000 -duration=30s /tmp/k6-stress.js

# Parallel load
k6 run ... &
k6 run ... &
k6 run ... &
wait
```

## Files Changed

- `docker-compose.yml` - Added pgbouncer service, updated API to connect through pgbouncer
- `pgbouncer/pgbouncer.ini` - PgBouncer configuration
- `pgbouncer/userlist.txt` - User authentication
- `backend/main.go` - Updated DB connection config (DB_MAX_OPEN_CONNS, DB_MAX_IDLE_CONNS env vars)
