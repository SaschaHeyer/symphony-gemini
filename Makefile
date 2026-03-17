.PHONY: build test run clean

build:
	go build -o bin/symphony ./cmd/symphony

test:
	go test ./...

run: build
	./bin/symphony

run-with-port: build
	./bin/symphony --port 8080

clean:
	rm -rf bin/
