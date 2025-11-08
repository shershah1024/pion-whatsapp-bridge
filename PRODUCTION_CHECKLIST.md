# Production Deployment Checklist

This guide covers everything needed to productionalize the WhatsApp Voice Bridge.

## 🚀 Infrastructure & Deployment

### 1. Server Setup

**Current:** Running locally with `go run .`
**Production:** Deploy to production server

#### Option A: VPS Deployment (DigitalOcean, Linode, AWS EC2)

```bash
# Build the binary
go build -o whatsapp-bridge .

# Create systemd service
sudo nano /etc/systemd/system/whatsapp-bridge.service
```

**`/etc/systemd/system/whatsapp-bridge.service`:**
```ini
[Unit]
Description=WhatsApp Voice Bridge
After=network.target

[Service]
Type=simple
User=bridge
Group=bridge
WorkingDirectory=/opt/whatsapp-bridge
ExecStart=/opt/whatsapp-bridge/whatsapp-bridge
Restart=always
RestartSec=10
StandardOutput=append:/var/log/whatsapp-bridge/output.log
StandardError=append:/var/log/whatsapp-bridge/error.log

# Environment variables
Environment="AZURE_OPENAI_API_KEY=xxx"
Environment="AZURE_OPENAI_ENDPOINT=xxx"
Environment="AZURE_OPENAI_DEPLOYMENT=xxx"
Environment="SUPABASE_URL=xxx"
Environment="SUPABASE_ANON_KEY=xxx"
Environment="WHATSAPP_PHONE_NUMBER_ID=xxx"
Environment="WHATSAPP_API_VERSION=v21.0"
Environment="WHATSAPP_ACCESS_TOKEN=xxx"

# Security
NoNewPrivileges=true
PrivateTmp=true

# Resource limits
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable whatsapp-bridge
sudo systemctl start whatsapp-bridge
sudo systemctl status whatsapp-bridge
```

#### Option B: Docker Deployment

**`Dockerfile`:**
```dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o whatsapp-bridge .

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/whatsapp-bridge .

EXPOSE 3011

CMD ["./whatsapp-bridge"]
```

**`docker-compose.yml`:**
```yaml
version: '3.8'

services:
  whatsapp-bridge:
    build: .
    ports:
      - "3011:3011"
    environment:
      - AZURE_OPENAI_API_KEY=${AZURE_OPENAI_API_KEY}
      - AZURE_OPENAI_ENDPOINT=${AZURE_OPENAI_ENDPOINT}
      - AZURE_OPENAI_DEPLOYMENT=${AZURE_OPENAI_DEPLOYMENT}
      - SUPABASE_URL=${SUPABASE_URL}
      - SUPABASE_ANON_KEY=${SUPABASE_ANON_KEY}
      - WHATSAPP_PHONE_NUMBER_ID=${WHATSAPP_PHONE_NUMBER_ID}
      - WHATSAPP_API_VERSION=${WHATSAPP_API_VERSION}
      - WHATSAPP_ACCESS_TOKEN=${WHATSAPP_ACCESS_TOKEN}
    restart: unless-stopped
    volumes:
      - ./logs:/var/log/whatsapp-bridge
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

**Deploy:**
```bash
docker-compose up -d
docker-compose logs -f
```

### 2. Cloudflare Tunnel (Already Configured!)

**Current setup:** ✅ `whatsapp-bridge.tslfiles.org`

**Production hardening:**
```bash
# Run as systemd service
sudo nano /etc/systemd/system/cloudflared.service
```

```ini
[Unit]
Description=Cloudflare Tunnel
After=network.target

[Service]
Type=simple
User=cloudflared
ExecStart=/usr/local/bin/cloudflared tunnel --config /etc/cloudflared/config.yml run
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable cloudflared
sudo systemctl start cloudflared
```

---

## 🔐 Security

### 1. Environment Variables Management

**Current:** `.env` file
**Production:** Use secrets manager

#### Option A: Systemd Environment File
```bash
# Create secure env file
sudo mkdir -p /etc/whatsapp-bridge
sudo nano /etc/whatsapp-bridge/environment

# Set proper permissions
sudo chmod 600 /etc/whatsapp-bridge/environment
sudo chown bridge:bridge /etc/whatsapp-bridge/environment
```

Update systemd service:
```ini
[Service]
EnvironmentFile=/etc/whatsapp-bridge/environment
```

#### Option B: AWS Secrets Manager / Vault

```go
// Add to main.go
func loadSecretsFromVault() error {
    // Use AWS SDK or Vault client
    // Set os.Setenv() for each secret
}
```

### 2. API Key Rotation

**Implement key rotation:**
- Set up WhatsApp access token rotation
- Azure OpenAI key rotation schedule
- Supabase key rotation

**Add monitoring for expiring keys:**
```go
func checkKeyExpiry() {
    // Alert if keys expire in < 7 days
}
```

### 3. Rate Limiting

**Add to main.go:**
```go
import "golang.org/x/time/rate"

var limiter = rate.NewLimiter(10, 20) // 10 req/sec, burst 20

func rateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### 4. Input Validation

**Add validation to webhook handler:**
```go
func validateWebhookSignature(r *http.Request) bool {
    // Verify X-Hub-Signature header
    // Validate webhook comes from WhatsApp
    return true
}
```

---

## 📊 Observability

### 1. Structured Logging

**Current:** `log.Printf()`
**Production:** Use structured logger

```bash
go get go.uber.org/zap
```

**Update logging:**
```go
import "go.uber.org/zap"

var logger *zap.Logger

func init() {
    var err error
    logger, err = zap.NewProduction()
    if err != nil {
        panic(err)
    }
}

// Replace log.Printf with:
logger.Info("Call accepted",
    zap.String("call_id", callID),
    zap.String("caller", phoneNumber),
    zap.Duration("duration", time.Since(start)),
)
```

### 2. Metrics & Monitoring

**Add Prometheus metrics:**
```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

```go
var (
    callsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "whatsapp_calls_total",
            Help: "Total number of WhatsApp calls",
        },
        []string{"type", "status"},
    )

    callDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "whatsapp_call_duration_seconds",
            Help: "Duration of WhatsApp calls",
        },
        []string{"type"},
    )

    activeConnections = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "whatsapp_active_connections",
            Help: "Number of active WebRTC connections",
        },
    )
)

func init() {
    prometheus.MustRegister(callsTotal)
    prometheus.MustRegister(callDuration)
    prometheus.MustRegister(activeConnections)
}

// Expose metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

**Set up monitoring stack:**
- **Prometheus** for metrics collection
- **Grafana** for dashboards
- **AlertManager** for alerts

### 3. Health Checks

**Add health endpoint:**
```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
    health := map[string]interface{}{
        "status": "healthy",
        "timestamp": time.Now().Unix(),
        "active_calls": len(activeCalls),
        "uptime": time.Since(startTime).Seconds(),
    }

    // Check dependencies
    if err := checkSupabase(); err != nil {
        health["status"] = "degraded"
        health["supabase"] = "unavailable"
    }

    if err := checkAzureOpenAI(); err != nil {
        health["status"] = "degraded"
        health["openai"] = "unavailable"
    }

    json.NewEncoder(w).Encode(health)
}

http.HandleFunc("/health", healthHandler)
```

### 4. Alerting

**Set up alerts for:**
- Service down (health check fails)
- High error rate (>5% of calls fail)
- Memory usage >80%
- Active calls >100 (capacity planning)
- Supabase/OpenAI API errors
- Certificate expiring
- Disk space low

**Use:**
- PagerDuty
- Opsgenie
- Slack webhooks

---

## 🔄 Reliability & Error Handling

### 1. Graceful Shutdown

**Add to main.go:**
```go
func main() {
    // ... existing setup ...

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-quit
        log.Println("🛑 Shutting down gracefully...")

        // Close active calls
        bridge.mu.Lock()
        for callID, call := range bridge.activeCalls {
            log.Printf("Closing call %s", callID)
            call.PeerConnection.Close()
        }
        bridge.mu.Unlock()

        // Shutdown server
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        if err := server.Shutdown(ctx); err != nil {
            log.Printf("Server forced to shutdown: %v", err)
        }

        os.Exit(0)
    }()

    log.Fatal(server.ListenAndServe())
}
```

### 2. Circuit Breaker

**For external API calls:**
```bash
go get github.com/sony/gobreaker
```

```go
import "github.com/sony/gobreaker"

var cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "whatsapp-api",
    MaxRequests: 3,
    Interval:    time.Minute,
    Timeout:     30 * time.Second,
})

func callWhatsAppAPI(action, callID, sdp string) error {
    _, err := cb.Execute(func() (interface{}, error) {
        // Existing API call logic
        return nil, actualAPICall(action, callID, sdp)
    })
    return err
}
```

### 3. Retry Logic with Backoff

```bash
go get github.com/cenkalti/backoff/v4
```

```go
import "github.com/cenkalti/backoff/v4"

func retryWithBackoff(operation func() error) error {
    exponentialBackOff := backoff.NewExponentialBackOff()
    exponentialBackOff.MaxElapsedTime = 5 * time.Minute

    return backoff.Retry(operation, exponentialBackOff)
}
```

---

## 💾 Database

### 1. Supabase Production Configuration

**Enable:**
- ✅ Connection pooling (already enabled by default)
- ✅ Read replicas (if needed for scale)
- ✅ Point-in-time recovery (PITR)
- ✅ Daily backups

**In Supabase Dashboard:**
1. Settings → Database → Enable PITR
2. Settings → Database → Set backup retention (7-30 days)

### 2. Index Optimization

**Run in Supabase SQL Editor:**
```sql
-- Check missing indexes
SELECT schemaname, tablename, attname, n_distinct, correlation
FROM pg_stats
WHERE schemaname = 'public'
  AND tablename IN ('ziggy_tasks', 'ziggy_reminders')
ORDER BY abs(correlation) DESC;

-- Add indexes based on query patterns
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reminders_phone_status_time
ON ziggy_reminders(phone_number, status, reminder_time);
```

### 3. Query Performance Monitoring

```sql
-- Enable pg_stat_statements
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Find slow queries
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
WHERE query LIKE '%ziggy_%'
ORDER BY mean_exec_time DESC
LIMIT 10;
```

---

## 🧪 Testing & CI/CD

### 1. Automated Testing

**Create `main_test.go`:**
```go
package main

import (
    "testing"
    "net/http/httptest"
)

func TestHealthEndpoint(t *testing.T) {
    req := httptest.NewRequest("GET", "/health", nil)
    w := httptest.NewRecorder()

    healthHandler(w, req)

    if w.Code != 200 {
        t.Errorf("Expected 200, got %d", w.Code)
    }
}

func TestWebhookValidation(t *testing.T) {
    // Test webhook signature validation
}

func TestTimezoneDetection(t *testing.T) {
    tz, err := GetTimezoneFromPhoneNumber("+919885842349")
    if err != nil {
        t.Errorf("Failed: %v", err)
    }
    if tz != "Asia/Kolkata" {
        t.Errorf("Expected Asia/Kolkata, got %s", tz)
    }
}
```

**Run tests:**
```bash
go test -v -cover ./...
```

### 2. GitHub Actions CI/CD

**`.github/workflows/deploy.yml`:**
```yaml
name: Deploy

on:
  push:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'
      - run: go test -v ./...

  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'
      - run: go build -o whatsapp-bridge .
      - uses: actions/upload-artifact@v3
        with:
          name: whatsapp-bridge
          path: whatsapp-bridge

  deploy:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v3
      - name: Deploy to server
        run: |
          # SSH and deploy
          scp whatsapp-bridge user@server:/opt/whatsapp-bridge/
          ssh user@server 'sudo systemctl restart whatsapp-bridge'
```

---

## 📈 Scalability

### 1. Connection Limits

**Current:** Single instance
**Production:** Consider limits

```go
const (
    MaxConcurrentCalls = 100  // Adjust based on server resources
)

var semaphore = make(chan struct{}, MaxConcurrentCalls)

func handleCall() {
    semaphore <- struct{}{}  // Acquire
    defer func() { <-semaphore }()  // Release

    // Handle call...
}
```

### 2. Load Balancing (if needed)

**If traffic grows:**
- Multiple instances behind load balancer
- Use Redis for shared state (active calls)
- Sticky sessions for WebRTC connections

### 3. Resource Limits

**Set ulimits:**
```bash
# /etc/security/limits.conf
bridge soft nofile 65535
bridge hard nofile 65535
```

---

## 📝 Compliance & Legal

### 1. Call Recording Disclosure

**Add to AI instructions:**
```go
instructions := "IMPORTANT: At the start of every call, announce: 'This call may be recorded for quality and training purposes.' Wait for user acknowledgment before proceeding."
```

### 2. Data Retention Policy

```sql
-- Auto-delete old completed reminders after 90 days
CREATE OR REPLACE FUNCTION cleanup_old_reminders()
RETURNS void AS $$
BEGIN
    DELETE FROM ziggy_reminders
    WHERE status IN ('completed', 'cancelled')
      AND updated_at < NOW() - INTERVAL '90 days';
END;
$$ LANGUAGE plpgsql;

-- Schedule cleanup weekly
SELECT cron.schedule(
    'cleanup-old-reminders',
    '0 0 * * 0',  -- Sunday midnight
    'SELECT cleanup_old_reminders()'
);
```

### 3. Privacy Compliance (GDPR)

**Add data export endpoint:**
```go
func exportUserData(w http.ResponseWriter, r *http.Request) {
    phoneNumber := r.URL.Query().Get("phone")

    // Export all user data
    tasks, _ := ListTasks(phoneNumber, "")
    reminders, _ := ListReminders(phoneNumber, "")

    data := map[string]interface{}{
        "tasks": tasks,
        "reminders": reminders,
    }

    json.NewEncoder(w).Encode(data)
}
```

**Add data deletion endpoint:**
```go
func deleteUserData(w http.ResponseWriter, r *http.Request) {
    phoneNumber := r.URL.Query().Get("phone")

    // Delete all user data
    db.Exec("DELETE FROM ziggy_tasks WHERE phone_number = ?", phoneNumber)
    db.Exec("DELETE FROM ziggy_reminders WHERE phone_number = ?", phoneNumber)

    w.WriteHeader(http.StatusNoContent)
}
```

---

## 🎯 Production Readiness Checklist

### Essential (Must Have)
- [ ] Deploy to production server (VPS/Docker)
- [ ] Systemd service with auto-restart
- [ ] Cloudflare Tunnel as systemd service
- [ ] Environment variables in secrets manager
- [ ] Structured logging (zap)
- [ ] Health check endpoint
- [ ] Graceful shutdown
- [ ] Supabase backups enabled
- [ ] WhatsApp webhook signature validation
- [ ] Rate limiting
- [ ] Run Supabase migrations (recurring reminders)

### Important (Should Have)
- [ ] Prometheus metrics
- [ ] Grafana dashboards
- [ ] AlertManager setup
- [ ] Circuit breaker for API calls
- [ ] Retry logic with backoff
- [ ] Automated tests
- [ ] CI/CD pipeline
- [ ] Connection limits
- [ ] Call recording disclosure
- [ ] Data retention policy

### Nice to Have
- [ ] Distributed tracing (Jaeger)
- [ ] Load balancing
- [ ] Redis for shared state
- [ ] Performance profiling (pprof)
- [ ] Chaos engineering tests
- [ ] Blue-green deployments

---

## 🚨 Day 1 Production Tasks

1. **Deploy infrastructure:**
   ```bash
   # Build and deploy
   go build -o whatsapp-bridge .
   scp whatsapp-bridge user@server:/opt/whatsapp-bridge/

   # Set up systemd
   sudo systemctl enable whatsapp-bridge
   sudo systemctl start whatsapp-bridge
   ```

2. **Run database migrations:**
   - Execute `supabase_recurring_reminders_migration.sql`
   - Re-run `supabase_cron_setup.sql`

3. **Set up monitoring:**
   - Configure health check monitoring (UptimeRobot, Pingdom)
   - Set up basic alerts (service down, high error rate)

4. **Test end-to-end:**
   - Make test inbound call
   - Make test outbound call
   - Test one-time reminder
   - Test recurring reminder
   - Test reminder cancellation

5. **Documentation:**
   - Document runbook for common issues
   - Create on-call playbook
   - Set up status page

---

## 📞 Support & Maintenance

### Daily
- Check error logs
- Monitor active connections
- Verify cron jobs running

### Weekly
- Review slow query logs
- Check disk space
- Rotate logs if needed
- Review alerts

### Monthly
- Review and optimize database indexes
- Update dependencies: `go get -u all`
- Security patches
- Backup verification (restore test)
- Capacity planning review

---

## 🆘 Incident Response

### Service Down
1. Check systemd: `sudo systemctl status whatsapp-bridge`
2. Check logs: `sudo journalctl -u whatsapp-bridge -n 100`
3. Restart: `sudo systemctl restart whatsapp-bridge`
4. Check dependencies (Supabase, Azure OpenAI)

### High Error Rate
1. Check logs for error patterns
2. Check external API status (WhatsApp, Azure)
3. Check resource usage: `htop`, `df -h`
4. Enable circuit breaker if APIs are failing

### Memory Leak
1. Check active connections: `curl localhost:3011/health`
2. Profile: `curl localhost:3011/debug/pprof/heap > heap.prof`
3. Analyze: `go tool pprof heap.prof`
4. Restart service if critical

This guide covers all major production concerns. Start with the "Essential" checklist and expand from there!
