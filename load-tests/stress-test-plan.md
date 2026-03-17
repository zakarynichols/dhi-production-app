# Scenario 1: Create Bin

## Endpoints to Test

1. **POST /api/bins** - Create bin (write)
2. **POST /:bin_id** - Capture request (write) - already tested ✅
3. **GET /api/bins** - List bins (read)
4. **GET /api/bins/:id** - Get bin (read)
5. **GET /api/bins/:id/requests** - List requests (read)
6. **DELETE /api/bins/:id** - Delete bin (delete)

## Testing Approach

### Scenario A: Create Bin Stress Test
- Create bins at high RPS
- Measure success rate

### Scenario B: List Bins Under Load
- Populate with many bins
- List bins while under write load

### Scenario C: Mixed Workload
- 50% capture, 30% list requests, 20% create bins

### Scenario D: Full Workflow
- Create bin → capture → list → delete
- All in one request sequence

## Commands

```bash
# Create target files for each endpoint

# POST /api/bins (create bin)
echo 'POST https://localhost:8443/api/bins' > targets-create-bin.txt
echo 'Content-Type: application/json' >> targets-create-bin.txt
echo '{"name":"load-test"}' >> targets-create-bin.txt

# GET /api/bins (list bins)
GET https://localhost:8443/api/bins

# GET /api/bins/:id/requests (list requests)
GET https://localhost:8443/api/bins/0f06bbc6/requests

# GET /health
GET https://localhost:8443/health
```
