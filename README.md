# ⚡ LLMProxy

L7 API Gateway for AI/LLM Traffic Management

## Features

- 🔄 Reverse proxy with connection pooling
- 🧠 Semantic caching (Redis, tenant-isolated)
- 🚦 Token-bucket rate limiting per API key
- ⚖️ Load balancing (round-robin, weighted, least-connections)
- 💓 Active health checking with automatic failover
- 📊 Prometheus + Grafana observability
- 🐳 Docker Compose + Kubernetes ready

## Quick Start

```bash
# Start everything
docker-compose up --build -d

# Test it
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello!"}]}'

# View metrics
# Prometheus:  http://localhost:9090
# Grafana:     http://localhost:3000  (admin / admin)