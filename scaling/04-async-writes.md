# Measure 4: Async Writes (2026-03-16)

**Setup:** 4 API replicas, async DB writes, PostgreSQL, Nginx

## Root Cause Discovery

Previous measures showed:
- 2 replicas: 81.2% success at 2000 RPS
- 4 replicas: 91.3% success at 2000 RPS

Investigation revealed:
- Nginx upstream was only connecting to ONE container (DNS resolution issue)
- Even with 4 replicas, PostgreSQL couldn't handle synchronous writes at 2000 RPS
- API logs showed "context canceled" - client timeout before DB write completed

## Solution: Async Writes

Modified `backend/main.go` - captureRequestHandler:

```go
// Before (synchronous)
_, err = app.db.ExecContext(r.Context(), ...)

// After (asynchronous)  
go func() {
    _, err := app.db.ExecContext(context.Background(), ...)
}()
```

Key changes:
1. Spawn goroutine for each DB write
2. Return 202 immediately without waiting for DB
3. Check bin existence asynchronously (fire and forget)

## Commands Run

```bash
# Edit backend/main.go - add goroutine for async writes
docker compose build api
docker compose up -d --scale api=4

# Test
vegeta attack -targets=targets-capture-test.txt -duration=10s -rate=2000 -insecure | vegeta report

# Verify captures
docker exec dhi-production-app-postgres-1 psql -U appuser -d appdb -c "SELECT COUNT(*) FROM requests WHERE bin_id = '0f06bbc6';"
```

## Results

| Test | Rate | Success | p95 Latency | Captured |
|------|------|---------|-------------|----------|
| Bin capture | 2000 RPS | **100%** | 1.3s | 19,998/19,998 |

## Comparison

| Metric | 4 Replicas (sync) | 4 Replicas (async) | Improvement |
|--------|------------------|-------------------|-------------|
| Success | 91.3% | 100% | +8.7% |
| p95 Latency | 11.5s | 1.3s | -10.2s |
| Lost | 1,712 | 0 | -1,712 |

## Key Findings

- **Async writes solved the bottleneck**
- PostgreSQL can't handle 2000 synchronous writes/sec
- By decoupling accept from write, API can accept requests faster than DB can persist
- Trade-off: Small risk of losing writes if API crashes (acceptable for request bin)

## Next Steps

- Test at higher RPS (3000, 5000)
- Add connection pooling for DB
- Implement batch inserts for even higher throughput
