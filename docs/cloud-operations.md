# EnvSync Cloud Operations

## SLOs
- Availability: 99.9% monthly for cloud sync endpoints.
- Latency target: p95 < 300ms for `GET /v1/store`, p95 < 500ms for `PUT /v1/store`.

## Required telemetry
- Request count by route and status code.
- Request latency histogram by route.
- PAT auth failure counters:
  - invalid token hash
  - revoked token
  - expired token
  - missing/invalid scope
- Conflict and abuse counters:
  - `409` revision conflicts
  - `429` rate limit responses
- Token lifecycle:
  - `POST /v1/tokens` success/failure
  - `DELETE /v1/tokens/:id` success/failure

## Dashboard panels
1. **Traffic and error overview**
   - Requests/min by route (`/v1/me`, `/v1/store`, `/v1/tokens`)
   - Error rate % (4xx + 5xx)
2. **Auth quality**
   - 401 count by reason
   - 403 count by reason/scope
3. **Store consistency**
   - 409 conflict rate
   - PUT success rate
4. **Latency**
   - p50/p95/p99 for `GET /v1/store`
   - p50/p95/p99 for `PUT /v1/store`
5. **Abuse control**
   - 429 rate over time
   - top source IPs by throttled requests

## Alert rules
1. **Availability burn alert**
   - Trigger: 5xx rate > 1% for 10m OR > 0.5% for 60m.
2. **Latency alert**
   - Trigger: p95 `GET /v1/store` > 300ms for 15m.
   - Trigger: p95 `PUT /v1/store` > 500ms for 15m.
3. **Auth failure alert**
   - Trigger: 401 rate > 10% over 15m and > 100 requests/min.
4. **Conflict surge alert**
   - Trigger: 409 rate > 5% over 15m.
5. **Rate-limit surge alert**
   - Trigger: 429 responses > 2% over 15m.

## Incident response quick actions
1. Confirm blast radius by route and status.
2. Correlate with deployment changes and DB metrics.
3. For auth spikes, inspect token revocation/expiry patterns and `ENVSYNC_CLOUD_PAT_PEPPER` changes.
4. For latency/error spikes, inspect DB connection saturation and slow query logs.
5. For abuse spikes, tune `ENVSYNC_CLOUD_RATE_LIMIT_RPM` and `ENVSYNC_CLOUD_RATE_LIMIT_BURST` and review source IPs.

## Post-incident
- File timeline with first detection, mitigations, and recovery time.
- Add or tune alert thresholds to reduce MTTD/MTTR.
- Add regression test where issue is reproducible.
