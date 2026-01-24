.PHONY: format build run dev

format:
	@echo "Formatting Go code..."
	gofmt -w server/
	@echo "Formatting Frontend code..."
	cd frontend && npm run format

build:
	docker build -t photoframe-server .

run:
	docker rm -f photoframe-server || true
	docker run -d -p 9607:9607 -v "$(PWD)/data:/data" --name photoframe-server photoframe-server

dev:
	@if ! command -v photoframe-process >/dev/null 2>&1; then \
		echo "Installing photoframe-process..."; \
		git clone --depth 1 https://github.com/aitjcize/esp32-photoframe.git /tmp/esp32-photoframe; \
		cd /tmp/esp32-photoframe/process-cli && npm install -g .; \
		rm -rf /tmp/esp32-photoframe; \
	else \
		echo "photoframe-process already installed, skipping..."; \
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
	@cd frontend && npm install && npm run build
	@echo "Starting server locally..."
	@cd server && DATA_DIR=$(PWD)/data DB_PATH=$(PWD)/data/photoframe.db STATIC_DIR=../frontend/dist go run .
