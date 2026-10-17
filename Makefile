.PHONY: test test-all run

test:
	go vet ./...
	go test -race -count=1 ./...

# Also runs the Postgres and MongoDB contract tests (see docker-compose.yml).
test-all:
	POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable \
	MONGO_URI=mongodb://localhost:27017 \
	go test -race -count=1 ./...

run:
	REGISTRY_USERS=admin:admin-pass:admin,viewer:viewer-pass:viewer \
	JWT_SECRET=change-me-please-123 go run ./cmd/registry
