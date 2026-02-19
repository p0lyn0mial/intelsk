.PHONY: run setup

# One-time setup: create Python venv, install pip deps, install npm deps, fetch Go modules.
setup:
	@echo "Setting up Python ML sidecar..."
	cd mlservice && python3 -m venv .venv && . .venv/bin/activate && pip install -r requirements.txt
	@echo "Installing frontend dependencies..."
	cd frontend && npm install
	@echo "Fetching Go modules..."
	cd backend && go mod download
	@echo "Setup complete."

# Start all services: ML sidecar, Go backend, and frontend dev server.
# Run 'make setup' first if you haven't already.
run:
	@test -d mlservice/.venv || (echo "Run 'make setup' first." && exit 1)
	@test -d frontend/node_modules || (echo "Run 'make setup' first." && exit 1)
	@echo "Starting intelsk..."
	@trap 'kill 0' EXIT; \
	cd mlservice && . .venv/bin/activate && uvicorn main:app --host 0.0.0.0 --port 8001 & \
	cd backend && go run . serve -root .. & \
	cd frontend && npm run dev & \
	wait
