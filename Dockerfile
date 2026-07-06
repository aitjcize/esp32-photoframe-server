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
RUN CGO_ENABLED=1 go build -o photoframe-server ./backend

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
    && npm install -g @aitjcize/epaper-image-convert \
    && apk del .build-deps \
    && rm -rf /root/.npm /tmp/*

# Material Symbols font for overlay rendering
RUN wget -O /tmp/MaterialSymbolsOutlined.ttf https://github.com/google/material-design-icons/raw/master/variablefont/MaterialSymbolsOutlined%5BFILL%2CGRAD%2Copsz%2Cwght%5D.ttf && \
    mkdir -p /usr/share/fonts/material && \
    mv /tmp/MaterialSymbolsOutlined.ttf /usr/share/fonts/material/ && \
    fc-cache -f

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
