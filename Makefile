GOLANGCI_LINT_VERSION=v2.5.0

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)

ALL_SOURCES := $(shell find * -type f -name "*.go")

COVERAGE_PROFILE_RAW=./build/coverage_raw.out
COVERAGE_PROFILE_RAW_HTML=./build/coverage_raw.html
COVERAGE_PROFILE_FILTERED=./build/coverage.out
COVERAGE_PROFILE_FILTERED_HTML=./build/coverage.html
COVERAGE_ENFORCER_FLAGS=-skipcode "// COVERAGE" -packagestats -filestats -showcode

.PHONY: build clean test test-coverage test-emulator lint

build:
	go build ./...

clean:
	go clean
	rm -rf ./build

test:
	go test -race -v ./...

test-emulator:
	@echo "Note: This requires the Firestore emulator to be running."
	@echo "Start it with: gcloud emulators firestore start --host-port=localhost:8080"
	@echo ""
	FIRESTORE_EMULATOR_HOST=localhost:8080 go test -race -v ./...

test-coverage: $(COVERAGE_PROFILE_RAW)
	go run github.com/launchdarkly-labs/go-coverage-enforcer@latest $(COVERAGE_ENFORCER_FLAGS) -outprofile $(COVERAGE_PROFILE_FILTERED) $(COVERAGE_PROFILE_RAW)
	go tool cover -html $(COVERAGE_PROFILE_FILTERED) -o $(COVERAGE_PROFILE_FILTERED_HTML)
	go tool cover -html $(COVERAGE_PROFILE_RAW) -o $(COVERAGE_PROFILE_RAW_HTML)

$(COVERAGE_PROFILE_RAW): $(ALL_SOURCES)
	@mkdir -p ./build
	go test -coverprofile $(COVERAGE_PROFILE_RAW) ./... >/dev/null

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)
	touch $(LINTER_VERSION_FILE)

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...
