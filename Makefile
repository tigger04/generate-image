# ABOUTME: Build, test, lint, and install targets for generate-image CLI.
# ABOUTME: Standard entry points per CLAUDE.md §10.

BINARY := generate-image
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build lint test test-one-off install uninstall sync

build:
	go build -o $(BINARY) .

lint:
	go vet ./...

test: lint
	HOME=$$(mktemp -d) go test ./tests/regression/ -v -count=1

test-one-off:
ifdef ISSUE
	go test ./tests/one_off/ -v -count=1 -run "$(ISSUE)"
else
	go test ./tests/one_off/ -v -count=1
endif

CONF_DIR := $(HOME)/.config/generate-image

install: build
	mkdir -p $(INSTALL_DIR) $(CONF_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	cp config.yaml $(CONF_DIR)/config.yaml
	@if [ -f .env ]; then \
		cp .env $(CONF_DIR)/.env; \
	elif [ ! -f $(CONF_DIR)/.env ]; then \
		cp .env.example $(CONF_DIR)/.env; \
		echo "Created $(CONF_DIR)/.env from template -- edit it to add your FAL_KEY"; \
	fi

uninstall:
	trash -- $(INSTALL_DIR)/$(BINARY) $(CONF_DIR)

sync:
	git add --all
	@if git diff --cached --quiet; then \
		echo "Nothing to commit"; \
	else \
		git commit; \
	fi
	git pull
	git push
