.PHONY: web web-dev dev-faux build test check lint format docker clean

web:
	cd web && npm ci && npm run build

web-dev:
	cd web && npm run dev

dev-faux:
	@echo "启动开发演示：faux-llm (18789) + app (3000)；按 Ctrl-C 退出"
	@FAUX_PORT=18789 go run ./cmd/faux-llm & \
	trap 'kill %1 2>/dev/null || true' EXIT; \
	LLM_API=openai-completions LLM_BASE_URL=http://127.0.0.1:18789/v1 \
	LLM_API_KEY=faux LLM_MODEL=faux-1 go run .

build: web
	CGO_ENABLED=0 go build -trimpath -o bin/pi-durable-assistant .

test:
	go test ./...

check: test
	CGO_ENABLED=0 go build ./...

lint:
	cd web && npm run check
	gofmt -l . | tee /tmp/gofmt.out
	@test ! -s /tmp/gofmt.out

format:
	cd web && npm run format
	gofmt -w .

docker:
	docker compose build

clean:
	rm -rf bin web/dist
