# Measure 3: 4 API Replicas (2026-03-16)

**Setup:** 4 API replicas (was 2 in Measure 2), PostgreSQL, Nginx

## Commands Run

```bash
# Edit docker-compose.yml - change replicas: 2 -> 4
docker compose up -d --scale api=4

# Create test bin
curl -ks -X POST https://localhost:8443/api/bins -H "Content-Type: application/json" -d '{"name":"scaling-test"}'

# Run test
vegeta attack -targets=targets-capture-test.txt -duration=10s -rate=2000 -insecure | vegeta report

# Check captures
docker exec dhi-production-app-postgres-1 psql -U appuser -d appdb -c "SELECT COUNT(*) FROM requests WHERE bin_id = '3f499403';"
```

## Results

| Test | Rate | Success | p95 Latency | Notes |
|------|------|---------|-------------|-------|
| Bin capture | 2000 RPS | 91.3% | 11.5s | Improved from 81.2% |

**Details:**
- 20,000 requests attempted
- 18,264 successful (202 Accepted)
- 1,734 requests 502 Bad Gateway
- 18,288 captured in database (1,712 lost!)

## Comparison

| Metric | 2 Replicas | 4 Replicas | Improvement |
|--------|------------|------------|-------------|
| Success | 81.2% | 91.3% | +10.1% |
| Lost | 2,753 | 1,712 | -1,041 |
| p95 Latency | 12.1s | 11.5s | -0.6s |

## Key Findings

- More replicas helps but **not enough** at 2000 RPS
- Still 8.7% failure rate
- Bottleneck is likely:
  - PostgreSQL connection saturation
  - DB write throughput
  - Nginx upstream connection limits

## Next Steps

- Try async writes (return 202 immediately, write in background)
- Or batch inserts
- Or add connection pooling / increase PostgreSQL resources
