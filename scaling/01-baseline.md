# Measure 1: Baseline (2026-03-16)

**Commit:** 1525acd
**Setup:** 2 API replicas, PostgreSQL, Nginx reverse proxy

## Commands Run

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

## Results

| Test | Rate | Success | p95 Latency | Notes |
|------|------|---------|-------------|-------|
| Frontend static | 10 RPS | 100% | 2.4ms | |
| Frontend static | 50 RPS | 100% | 2.3ms | |
| Bin capture | 10 RPS | 100% | 7ms | |
| Bin capture | 100 RPS | 100% | 193ms | |
| Bin capture | 500 RPS | 100% | 446ms | Throughput: 484/s |
| Bin capture | 1000 RPS | 100% | 885ms | Throughput: 910/s |

## Key Findings

- **No breaking point found** at 1000 RPS
- Frontend serves static content efficiently through Nginx
- Database writes keep up with high request volume

## Configuration

- API: 2 replicas (docker-compose deploy.replicas: 2)
- PostgreSQL: default settings
- Rate limiting: disabled in code for testing
