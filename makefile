.PHONY: run build

run: build
	@./bin/scrape

build:
	@go build -o bin/scrape .
