ARG BUILD_FROM=node:20-alpine

# Build Stage for Go
FROM golang:alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git build-base

# Copy Go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy Source
COPY server/ ./server/
RUN CGO_ENABLED=1 go build -o photoframe-server ./server

# Build Stage for Frontend
FROM node:20-alpine AS frontend-builder
ARG ADDON_PORT=9607
WORKDIR /app
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN VITE_ADDON_PORT=${ADDON_PORT} npm run build

# Final Stage
FROM $BUILD_FROM

WORKDIR /app

# Install runtime dependencies
# Canvas dependencies: cairo, pango, jpeg, giflib, librsvg
# Node.js for epaper-image-convert
RUN apk add --no-cache \
    cairo-dev \
    pango-dev \
    jpeg-dev \
    giflib-dev \
    librsvg-dev \
    tzdata \
    ffmpeg \
    build-base \
    python3 \
    font-noto \
    font-noto-emoji \
    nodejs \
    npm

# Create directories
RUN mkdir -p /app/bin /app/static /app/data

# Download and install Material Symbols font
RUN wget -O /tmp/MaterialSymbolsOutlined.ttf https://github.com/google/material-design-icons/raw/master/variablefont/MaterialSymbolsOutlined%5BFILL%2CGRAD%2Copsz%2Cwght%5D.ttf && \
    mkdir -p /usr/share/fonts/material && \
    mv /tmp/MaterialSymbolsOutlined.ttf /usr/share/fonts/material/ && \
    fc-cache -f

# Copy Binary
COPY --from=builder /app/photoframe-server /app/photoframe-server

# Copy Frontend Build
COPY --from=frontend-builder /app/dist /app/static

# Copy Migrations
COPY server/db/migrations /app/db/migrations

# Install epaper-image-convert
RUN npm install -g @aitjcize/epaper-image-convert

WORKDIR /app

# Environment Variables
ENV PORT=8080
ENV STATIC_DIR=/app/static
ENV DB_PATH=/data/photoframe.db
ENV DATA_DIR=/data

EXPOSE 9607

ENTRYPOINT ["/app/photoframe-server"]
