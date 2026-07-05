.PHONY: format format-check install-hooks build run dev

format:
	@echo "Formatting Go code..."
	gofmt -w backend/
	@echo "Formatting Frontend code..."
	cd webapp && npm run format

format-check:
	@echo "Checking Go formatting..."
	@unformatted=$$(gofmt -l backend/); \
	if [ -n "$$unformatted" ]; then \
		echo "These Go files need formatting (run 'make format'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	@echo "Checking Frontend formatting..."
	@cd webapp && [ -d node_modules ] || npm ci
	@cd webapp && npm run format:check
	@echo "All files are properly formatted!"

# Enable the repo's git hooks (pre-commit runs `make format-check`).
install-hooks:
	@git config core.hooksPath .githooks
	@echo "Git hooks enabled (core.hooksPath = .githooks)."
	@echo "Commits now run 'make format-check' first."

build:
	docker build -t photoframe-server .

run:
	docker rm -f photoframe-server || true
	docker run -d -p 9607:9607 -v "$(PWD)/data:/data" --name photoframe-server photoframe-server

dev:
	@if ! command -v epaper-image-convert >/dev/null 2>&1; then \
		echo "Installing epaper-image-convert..."; \
		npm install -g @aitjcize/epaper-image-convert; \
	else \
		echo "epaper-image-convert already installed, skipping..."; \
	fi
	@if [ ! -f bin/fonts/NotoSans-Regular.ttf ]; then \
		echo "Downloading NotoSans font..."; \
		mkdir -p bin/fonts; \
		curl -sL "https://github.com/google/fonts/raw/main/ofl/notosans/NotoSans-Regular.ttf" -o bin/fonts/NotoSans-Regular.ttf; \
	fi
	@if [ ! -f bin/fonts/MaterialSymbolsOutlined.ttf ]; then \
		echo "Downloading Material Symbols font..."; \
		mkdir -p bin/fonts; \
		curl -sL "https://github.com/google/material-design-icons/raw/master/variablefont/MaterialSymbolsOutlined%5BFILL%2CGRAD%2Copsz%2Cwght%5D.ttf" -o bin/fonts/MaterialSymbolsOutlined.ttf; \
	fi
	@echo "Building frontend..."
	@cd webapp && npm install && npm run build
	@echo "Starting server locally..."
	@cd backend && CGO_ENABLED=1 DATA_DIR=$(PWD)/data DB_PATH=$(PWD)/data/photoframe.db STATIC_DIR=../webapp/dist go run .
