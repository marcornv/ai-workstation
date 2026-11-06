# Bancada — common tasks.
#   make run     build everything and start the app
#   make dev     backend + Vite dev server with hot reload (open http://localhost:5173)
#   make test    run Go and Python tests
#   make clean   remove build outputs (keeps your data and settings)

.PHONY: build run dev test clean frontend backend evaluator

build: frontend backend

frontend:
	cd frontend && npm install --no-fund --no-audit && npm run build

backend:
	cd backend && go build -o ../bancada .

evaluator:
	cd evaluator && python3 -m venv .venv && .venv/bin/pip install -q -r requirements.txt

run: build
	./bancada

dev:
	@echo "API on :8080, UI on http://localhost:5173"
	(cd backend && go run . -dir .. -no-browser) & (cd frontend && npm run dev)

test:
	cd backend && go vet ./... && go test ./...
	cd evaluator && .venv/bin/python -m pytest -q

clean:
	rm -rf bancada bancada.exe backend/web/dist frontend/tsconfig.tsbuildinfo
