BIN := mison
CMD := ./cmd/mison

.PHONY: build test lint fmt vet tidy clean install

build:
	go build -o $(BIN) $(CMD)

test:
	go test ./...

test-e2e:
	go test -tags e2e ./internal/e2e/

test-coverage:
	go test -coverprofile=coverage.txt ./...
	go tool cover -func=coverage.txt | tail -1

lint:
	golangci-lint run

fmt:
	gofmt -w .
	go mod tidy

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f $(BIN) coverage.txt
	rm -rf dist/

install:
	go install $(CMD)
