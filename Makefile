.PHONY: build run test clean docker-up docker-down load-test

# ─── Build ──────────────────────────────────────────
build:
	go build -o bin/proxy ./cmd/proxy/
	go build -o bin/worker ./cmd/worker/

# ─── Run locally ────────────────────────────────────
run-proxy: build
	REDIS_ADDR=localhost:6379 ./bin/proxy

run-worker-1:
	WORKER_PORT=9001 WORKER_ID=worker-1 go run ./cmd/worker/

run-worker-2:
	WORKER_PORT=9002 WORKER_ID=worker-2 go run ./cmd/worker/

run-worker-3:
	WORKER_PORT=9003 WORKER_ID=worker-3 go run ./cmd/worker/

# ─── Docker ─────────────────────────────────────────
docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down -v

docker-logs:
	docker-compose logs -f proxy

# ─── Testing ────────────────────────────────────────
test:
	go test ./... -v -count=1

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# ─── Load Testing ───────────────────────────────────
load-test:
	chmod +x loadtest/vegeta_attack.sh
	./loadtest/vegeta_attack.sh

# ─── Quick test with curl ───────────────────────────
test-request:
	curl -s -X POST http://localhost:8080/v1/chat/completions \
		-H "Content-Type: application/json" \
		-H "X-API-Key: test-key" \
		-d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello, how are you?"}]}' | jq .

test-cache:
	@echo "=== First request (cache MISS) ==="
	curl -s -X POST http://localhost:8080/v1/chat/completions \
		-H "Content-Type: application/json" \
		-H "X-API-Key: test-key" \
		-D - \
		-d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is Go?"}]}' | head -20
	@echo ""
	@echo "=== Second request (cache HIT) ==="
	curl -s -X POST http://localhost:8080/v1/chat/completions \
		-H "Content-Type: application/json" \
		-H "X-API-Key: test-key" \
		-D - \
		-d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is Go?"}]}' | head -20

test-rate-limit:
	@echo "=== Firing 10 rapid requests with free key (limit: 2/sec) ==="
	@for i in $$(seq 1 10); do \
		echo "Request $$i:"; \
		curl -s -o /dev/null -w "HTTP %{http_code}\n" -X POST http://localhost:8080/v1/chat/completions \
			-H "Content-Type: application/json" \
			-H "X-API-Key: key-free" \
			-d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Request '$$i'"}]}'; \
	done

test-auth:
	@echo "=== No API key ==="
	curl -s http://localhost:8080/v1/chat/completions | jq .
	@echo ""
	@echo "=== Invalid API key ==="
	curl -s -H "X-API-Key: invalid" http://localhost:8080/v1/chat/completions | jq .

# ─── Cleanup ────────────────────────────────────────
clean:
	rm -rf bin/ coverage.out coverage.html