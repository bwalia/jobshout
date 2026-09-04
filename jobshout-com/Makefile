# JobShout.com — local development
.PHONY: dev api web migrate seed test lint docker-up help

DATABASE_URL ?= postgres://jobshout:jobshout@127.0.0.1:5434/jobshout_com
export DATABASE_URL

help:
	@echo "Targets: docker-up migrate seed api web test lint"

docker-up:
	docker compose up -d postgres redis nats minio

migrate: docker-up
	@echo "Waiting for Postgres..."
	@until docker compose exec -T postgres pg_isready -U jobshout -d jobshout_com >/dev/null 2>&1; do sleep 1; done
	DATABASE_URL="$(DATABASE_URL)" cargo run -p jobshout-com-migrate

seed: migrate
	docker compose exec -T postgres psql -U jobshout -d jobshout_com < scripts/seed.sql
	@echo "Seeded organisations + jobs"

api:
	DATABASE_URL="$(DATABASE_URL)" HOST=0.0.0.0 PORT=8088 cargo run -p jobshout-com-api

web:
	cd web/nextjs && npm install && JOBSHOUT_COM_API_URL=http://127.0.0.1:8088 npm run dev

dev:
	@echo "1) make docker-up && make migrate && make seed"
	@echo "2) make api    # :8088"
	@echo "3) make web    # :3010"

test:
	cargo test --workspace

lint:
	cargo fmt --all -- --check
	cargo clippy --workspace --all-targets -- -D warnings
