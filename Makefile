.PHONY: build run docker-up docker-down clean

build:
	go build -o bin/sidekick ./cmd/sidekick

run: build
	./bin/sidekick

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-build:
	docker compose up -d --build

clean:
	rm -rf bin/
	docker compose down -v
