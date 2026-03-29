FROM debian:bookworm

# Install additional OS packages.
# Dependencies for image processing (matching the Alpine packages in production Dockerfile)
# Also includes tools for deployment (rsync, ssh) and development (sqlite3, python3)
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends \
    libcairo2-dev \
    libpango1.0-dev \
    libjpeg-dev \
    libgif-dev \
    librsvg2-dev \
    ffmpeg \
    rsync \
    openssh-client \
    sqlite3 \
    python3 \
    build-essential \
    pkg-config \
    && apt-get clean -y && rm -rf /var/lib/apt/lists/*

# Note: Node.js 20 is installed via the 'features' in devcontainer.json