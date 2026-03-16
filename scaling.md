# Scaling Diary

## Measure 1: Baseline (2026-03-16)

**Commit:** 1525acd
**Setup:** 2 API replicas, PostgreSQL, Nginx reverse proxy

### Commands Run

```bash
# Start containers
docker compose up -d --build

# Fix nginx config (try_files path)
# Edit nginx/conf.d/default.conf - change try_files to use root directive

# Frontend tests
vegeta attack -targets=targets-frontend.txt -duration=10s -rate=10 -insecure | vegeta report
vegeta attack -targets=targets-frontend.txt -duration=10s -rate=50 -insecure | vegeta report

# Create test bin
curl -ks -X POST https://localhost:8443/api/bins -H "Content-Type: application/json" -d '{"name":"load-test"}'

# Capture tests
vegeta attack -targets=targets-capture-test.txt -duration=10s -rate=10 -insecure | vegeta report
vegeta attack -targets=targets-capture-test.txt -duration=10s -rate=100 -insecure | vegeta report
vegeta attack -targets=targets-capture-test.txt -duration=10s -rate=500 -insecure | vegeta report
vegeta attack -targets=targets-capture-test.txt -duration=10s -rate=1000 -insecure | vegeta report

# Verify captures
curl -ks https://localhost:8443/api/bins/5166df97/requests | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d))"
docker exec dhi-production-app-postgres-1 psql -U appuser -d appdb -c "SELECT COUNT(*) FROM requests WHERE bin_id = '5166df97';"
```

### Results

| Test | Rate | Success | p95 Latency | Notes |
|------|------|---------|-------------|-------|
| Frontend static | 10 RPS | 100% | 2.4ms | |
| Frontend static | 50 RPS | 100% | 2.3ms | |
| Bin capture | 10 RPS | 100% | 7ms | |
| Bin capture | 100 RPS | 100% | 193ms | |
| Bin capture | 500 RPS | 100% | 446ms | Throughput: 484/s |
| Bin capture | 1000 RPS | 100% | 885ms | Throughput: 910/s |

### Key Findings

- **No breaking point found** at 1000 RPS
- Frontend serves static content efficiently through Nginx
- Database writes keep up with high request volume

### Configuration

- API: 2 replicas (docker-compose deploy.replicas: 2)
- PostgreSQL: default settings
- Rate limiting: disabled in code for testing

---

## Measure 2: 2000 RPS Stress Test (2026-03-16)

**Commit:** e00997c
**Setup:** Same as Measure 1

### Commands Run

```bash
vegeta attack -targets=targets-capture-test.txt -duration=10s -rate=2000 -insecure | vegeta report
docker exec dhi-production-app-postgres-1 psql -U appuser -d appdb -c "SELECT COUNT(*) FROM requests WHERE bin_id = '5166df97';"
```

### Results

| Test | Rate | Success | p95 Latency | Notes |
|------|------|---------|-------------|-------|
| Bin capture | 2000 RPS | 81.2% | 12.1s | **BREAKING POINT** |

**Details:**
- 20,000 requests attempted
- 16,247 successful (202 Accepted)
- 81 requests 502 Bad Gateway
- 3,672 requests failed (connection errors)
- Only 17,247 captured in database (2,753 lost!)

### Key Findings

- **Breaking point found at ~1000 RPS**
- At 2000 RPS: 18.8% failure rate, 2,753 requests lost
- Nginx/API becomes overwhelmed, connections time out
- Need: more API replicas, connection pooling, or rate limiting

### Evidence of Failure

**Vegeta output:**
```
Status Codes  [code:count]  0:3672  202:16247  502:81
Error Set:
Post "https://localhost:8443/5166df97": unexpected EOF
502 Bad Gateway
```

**API logs (dhi-production-app-api-1):**
```
2026/03/16 17:32:20 Error capturing request: context canceled
2026/03/16 17:32:20 Error capturing request: context canceled
... (repeated)
```

**Root cause:** Nginx proxy timeouts (60s) exceeded, client closes connection before API can finish writing to PostgreSQL. The "context canceled" error means the HTTP request context was cancelled/timed out while the database write was still in progress.
