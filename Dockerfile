ARG BUILD_FROM=node:20-alpine3.21

# Build Stage for Go
FROM golang:alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git build-base

# Copy Go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy Source
COPY backend/ ./backend/
# VERSION is stamped into the binary and surfaced via /api/status and the
# webapp footer, so users on the "latest" image tag can tell what they run.
ARG VERSION=dev
RUN CGO_ENABLED=1 go build \
    -ldflags "-X github.com/aitjcize/esp32-photoframe-server/backend/internal/version.Version=${VERSION}" \
    -o photoframe-server ./backend

# Build Stage for Frontend
FROM node:20-alpine AS frontend-builder
ARG ADDON_PORT=9607
WORKDIR /app
COPY webapp/package*.json ./
RUN npm install --ignore-scripts
COPY webapp/ ./
RUN VITE_ADDON_PORT=${ADDON_PORT} npm run build

# Final Stage
FROM $BUILD_FROM

# Resolved to the current npm release by CI and deploy-dev.sh so the install
# layer's cache key changes when a new converter ships -- an unpinned install
# inside a cached layer silently keeps the old version. Defaults to "latest"
# for builds that pass no args (e.g. the supervisor's on-device build, which
# starts cacheless anyway). ARGs are per-stage: this must live in THIS stage.
ARG CONVERTER_VERSION=latest

WORKDIR /app

# Runtime dependencies. The native `canvas` module (epaper-image-convert) is
# compiled with a throwaway .build-deps virtual package (gcc + *-dev headers +
# python3 + npm) that is removed afterwards, so only the runtime shared libs
# stay in the image. Runtime tools: chromium (HTML overlay rendering),
# imagemagick + libheif (HEIC/RAW conversion), Noto fonts (incl. CJK).
RUN apk add --no-cache \
        nodejs \
        chromium \
        nss \
        freetype \
        harfbuzz \
        cairo \
        pango \
        libjpeg-turbo \
        giflib \
        librsvg \
        pixman \
        tzdata \
        imagemagick \
        libheif \
        font-noto \
        font-noto-emoji \
        font-noto-cjk \
    && apk add --no-cache --virtual .build-deps \
        build-base \
        python3 \
        npm \
        cairo-dev \
        pango-dev \
        jpeg-dev \
        giflib-dev \
        librsvg-dev \
    && npm install -g "@aitjcize/epaper-image-convert@${CONVERTER_VERSION}" \
    && apk del .build-deps \
    && rm -rf /root/.npm /tmp/*

# Material Symbols font for overlay rendering. Fetched from Google Fonts as a
# static instance pinned to the axis values the renderer uses (FILL=1 filled
# icons, GRAD=0, opsz=48, wght=400) — the css2 API serves a plain-TTF
# @font-face to non-browser user agents, and we grep the versioned
# fonts.gstatic.com URL out of it. The previous source (GitHub raw) started
# rejecting anonymous downloads with 404/429. Bonus: the instance is ~1.4MB
# vs the 10.6MB variable font the renderer base64-embeds per overlay render.
RUN wget -O /tmp/material.css \
      "https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined:FILL,GRAD,opsz,wght@1,0,48,400" && \
    FONT_URL=$(grep -o 'https://[^)]*\.ttf' /tmp/material.css | head -n1) && \
    test -n "$FONT_URL" && \
    mkdir -p /usr/share/fonts/material && \
    wget -O /usr/share/fonts/material/MaterialSymbolsOutlined.ttf "$FONT_URL" && \
    fc-cache -f && \
    rm /tmp/material.css

# Create directories
RUN mkdir -p /app/bin /app/static /app/data

# Copy Binary
COPY --from=builder /app/photoframe-server /app/photoframe-server

# Copy Frontend Build
COPY --from=frontend-builder /app/dist /app/static

# Copy Migrations
COPY backend/db/migrations /app/db/migrations

WORKDIR /app

# Environment Variables
ENV PORT=9607
ENV STATIC_DIR=/app/static
ENV DB_PATH=/data/photoframe.db
ENV DATA_DIR=/data
ARG ADDON_PORT=9607
ENV ADDON_PORT=$ADDON_PORT

EXPOSE 9607

ENTRYPOINT ["/app/photoframe-server"]
