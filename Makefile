BINARY := kubectl-inventory

.PHONY: build install test fmt tidy clean

build:
	go build -o bin/$(BINARY) ./main.go

install:
	go install ./...

test:
	go test ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf bin dist
