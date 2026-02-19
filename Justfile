image := "olifant/postgresql:18"

[working-directory: "postgresql"]
build-img:
    podman build -t "{{image}}" .

postgres-run:
    podman run -it -p 5432:5432 -e POSTGRES_USER="test" \
      -e POSTGRES_PASSWORD="test" \
      -e POSTGRES_DB="postgres" \
      --tmpfs /var/lib/postgresql/18/docker:rw,size=512m \
      {{image}}

goose-install:
    mkdir -p bin
    GOBIN="{{justfile_directory()}}/bin" go install github.com/pressly/goose/v3/cmd/goose@latest

add-migration name:
    bin/goose -dir "{{justfile_directory()}}/sql" -s create "{{name}}" sql

migrate:
    GOOSE_DRIVER="postgres" \
    GOOSE_DBSTRING="postgres://test:test@localhost:5432/postgres?sslmode=disable" \
    GOOSE_MIGRATION_DIR="{{justfile_directory()}}/sql" \
    bin/goose up

[working-directory: "client"]
client-build:
    go build -o client -ldflags "-s -w" main.go