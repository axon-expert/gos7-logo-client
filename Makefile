include version/Makefile

.ONESHELL:
.PHONY: format lint watch-templ format-lint tidy-golangci-lint
SHELL := /bin/bash


####################################################################################################
# FORMAT                                                                                           #
####################################################################################################

format:
	@go tool -modfile=golangci-lint.mod golangci-lint fmt ./... && \
	echo "FORMAT OK!"

lint:
	@go tool -modfile=golangci-lint.mod golangci-lint run ./... && \
	echo "LINT OK!"

format-lint: format lint

tidy-golangci-lint:
	@set -e
	@export GOLANGCI_LINT_TMP_DIR=$$(mktemp -d)
	@trap 'rm -rf "$${GOLANGCI_LINT_TMP_DIR}"' EXIT
	@cp golangci-lint.mod "$${GOLANGCI_LINT_TMP_DIR}/go.mod"
	@cp golangci-lint.sum "$${GOLANGCI_LINT_TMP_DIR}/go.sum"
	@cd "$${GOLANGCI_LINT_TMP_DIR}"
	@go mod tidy
	@cp go.mod "$(CURDIR)/golangci-lint.mod"
	@cp go.sum "$(CURDIR)/golangci-lint.sum"
