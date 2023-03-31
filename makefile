# All targets.
.PHONY: lint test build publish clean help

# Current version of the project.
VersionMain ?= 0.1.1


# Target binaries. You can build multiple binaries for a single project.
TARGETS := wiki
PLAT_FROM := linux darwin windows

# Project main package location (can be multiple ones).
CMD_DIR := .
# Project output directory.
OUTPUT_DIR := ./output
# Git commit sha.
COMMIT := $(shell git rev-parse --short HEAD)
# Build Date
BUILD_DATE=$(shelldate +%FT%T%z)
# Version File
VERSION_FILE=main

lint:  ## use golint to do lint
	golint ./...

test:  ## test
	go test -cover ./...

build: ## build local binary for targets on
	@for target in $(TARGETS); do                                                      \
	  go build -o $(OUTPUT_DIR)/$${target}                                             \
	    -ldflags "-s -w -X $(VERSION_FILE).Version=$(VersionMain)-$(COMMIT)"       \
	    $(CMD_DIR)/.;                                                                  \
	done


build-all: # build cross for targets
	@for plat in $(PLAT_FROM); do                                                                  \
		for target in $(TARGETS); do                                                               \
			CGO_ENABLED=0 GOOS=$${plat} GOARCH=amd64 go build -o $(OUTPUT_DIR)/$${target}_$${plat}_amd64 \
				-ldflags "-s -w -X $(VERSION_FILE).Version=$(VersionMain)-$(COMMIT)"           \
	    			$(CMD_DIR)/.;                                                                  \
		done                                                                                       \
	done


.PHONY: clean
clean:  # clean bin files
	-rm -vrf ${OUTPUT_DIR}

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
