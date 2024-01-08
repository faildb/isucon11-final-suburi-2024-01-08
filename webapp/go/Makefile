DEST=$(PWD)/isucholar
COMPILER=go
GO_FILES=$(wildcard ./*.go ./**/*.go)
APP_VERSION=$(shell git rev-parse --short HEAD)

.PHONY: all
all: clean build ## Cleanup and Build

.PHONY: build
build: $(GO_FILES) ## Build executable files
	@$(COMPILER) build -o $(DEST) -ldflags "-s -w -X github.com/isucon/isucon11-final/webapp/go.AppVersion=$(APP_VERSION)"

.PHONY: clean
clean: ## Cleanup files
	@$(RM) -r $(DEST)

.PHONY: help
help: ## Show help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
