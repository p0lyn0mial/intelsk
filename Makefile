.PHONY: run

# Start all services: ML sidecar, Go backend, and frontend dev server.
# Requires: Python venv at mlservice/.venv, Go, Node.js, ffmpeg
run:
	@echo "Starting intelsk..."
	@trap 'kill 0' EXIT; \
	cd mlservice && . .venv/bin/activate && uvicorn main:app --host 0.0.0.0 --port 8001 & \
	cd backend && go run . serve -root .. & \
	cd frontend && npm run dev & \
	wait
