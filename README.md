# ESP32 PhotoFrame Server

A middleware server for the [ESP32 PhotoFrame](https://github.com/aitjcize/esp32-photoframe) project. This server acts as a bridge between the E-Ink display and various photo sources (Google Photos, Telegram), handling image processing, resizing, dithering, and overlay generation.

## Features

-   **Multiple Data Sources**:
    -   **Google Photos**: Uses the Picker API to securely select albums and photos.
    -   **Synology Photos**: Connect directly to your Synology NAS (supports DSM 7 Personal and Shared spaces).
    -   **Telegram Bot**: Send photos directly to your frame via a Telegram bot.
-   **Smart Image Processing**:
    -   Automatic cropping to device aspect ratio (800x480 or 480x800).
    -   **Smart Collage**: Automatically combines two landscape photos in portrait mode (or vice versa) to maximize screen usage.
    -   **Dithering**: Applies Floyd-Steinberg dithering optimized for 7-color E-Ink displays (reusing the same logic as the firmware).
-   **Overlays**:
    -   Customizable Date/Time display.
    -   Real-time Weather status (Temperature + Condition) based on location.
    -   "iPhone Lockscreen" style aesthetics with Inter font and drop shadows.
-   **Web Interface**:
    -   Modern Vue 3 + Tailwind CSS dashboard.
    -   Manage settings: Orientation, Weather location, Collage mode.
    -   Manage gallery: View and delete imported photos.
    -   Import photos via Google Photos Picker.

## Deployment (Docker)

The easiest way to run the server is using Docker.

### 1. Build & Run locally

```bash
# Build the image
make build

# Run the container
# -p 9607:9607 : Expose web UI
# -v $(pwd)/data:/data : Persist database and photos
make run
```

### 2. Manual Docker Run

```bash
docker run -d \
  -p 9607:9607 \
  -v /path/to/data:/data \
  --name photoframe-server \
  aitjcize/esp32-photoframe-server:latest
```

### 3. Docker Compose (tested on Synology DSM 7)
For use in Container Manager or Portainer. Update the `volume` and `user` sections with values appropriate to your environment.
```
name: esp32-photoframe-server
services:
    esp32-photoframe-server:
        ports:
            - 9607:9607
        volumes:
            - /volume1/docker/esp32-photoframe-server:/data
        container_name: photoframe-server
        image: aitjcize/esp32-photoframe-server:latest
        user: <uid>:<gid>
```

## Configuration

Access the dashboard at `http://localhost:9607` (or your server IP).

### Google Photos Setup

> [!IMPORTANT]
> **Google OAuth Restriction**: Google does not allow `.local` domains or private IP addresses in OAuth redirect URIs. If running on Home Assistant, you must use one of these methods:
> - **Port Forwarding** (recommended for one-time setup): `ssh -L 9607:localhost:9607 root@homeassistant.local -p 22222`
> - **Public Domain**: Use a domain name with Cloudflare Tunnel or similar

#### Steps:

1.  **Create OAuth Credentials**:
    -   Go to [Google Cloud Console](https://console.cloud.google.com/)
    -   Create a new project or select an existing one
    -   Enable the **Google Photos Picker API**
    -   Go to **Credentials** → **Create Credentials** → **OAuth 2.0 Client ID**
    -   Application type: **Web application**
    -   **Authorized JavaScript Origins**: `http://localhost:9607`
    -   **Authorized Redirect URIs**: `http://localhost:9607/api/auth/google/callback`
    -   Click **Create** and save your Client ID and Client Secret

2.  **Configure the Server**:
    -   If running on Home Assistant, set up port forwarding first:
        ```bash
        ssh -L 9607:localhost:9607 root@homeassistant.local -p 22222
        ```
    -   Access the dashboard at `http://localhost:9607`
    -   Go to **Settings**
    -   Select **Source: Google Photos**
    -   Enter your **Client ID** and **Client Secret**
    -   Click **Save All Settings**

3.  **Authenticate and Import Photos**:
    -   Go to the **Gallery** tab
    -   Click **Add Photos**
    -   You'll be redirected to Google OAuth (sign in if needed)
    -   Select the photos you want to display
    -   Click **Add** to import them

4.  **After Setup**:
    -   The OAuth token is saved in the database
    -   You can close the SSH tunnel (if used)
    -   Access the server normally via `http://homeassistant.local:9607` or your regular URL
    -   Re-authentication is only needed if you revoke access or want to add more photos

### Synology Setup
1.  Go to **Settings** in the dashboard.
2.  Enable **Synology Photos**.
3.  Enter your **NAS URL** (e.g., `https://192.168.1.10:5001`), **Account**, and **Password**.
4.  If using 2FA, enter the **OTP Code** when testing the connection.
5.  Select the **Photo Space** (Personal or Shared) and optionally a specific **Album**.
6.  Click **Sync Now** to import metadata.

### Telegram Setup
1.  Create a new bot via [@BotFather](https://t.me/botfather) on Telegram.
2.  Get the **Bot Token**.
3.  Go to **Settings** in the dashboard.
4.  Select **Source: Telegram Bot**.
5.  Enter your Bot Token and save.
6.  Send a photo to your bot on Telegram. The frame will update to show this photo immediately.

## Photo Frame Configuration
Once you've configured a photo source, a box with the correct URL will appear on that tab. It should be in the format
```
http(s)://<hostname/IP address>/image/<integration>
```
1. Copy the URL.
2. Generate an access token.
3. Log into the photo frame web app.
4. Go to the Auto-Rotate tab and paste the URL and the Token in the appropriate boxes.
5. Click the 'Save Settings' button

## API Endpoints (For ESP32)

-   **`GET /image/google`**: Returns a random image specifically from **Google Photos**.
-   **`GET /image/synology`**: Returns a random image specifically from **Synology Photos**. 
-   **`GET /image/telegram`**: Returns the last photo sent via **Telegram Bot**.

### Technical Details:
-   **Output Format**: Processed 7-color PNG, optimized with Floyd-Steinberg dithering.
-   **Automatic Scaling**: Images are automatically cropped and resized to your frame's dimensions.
-   **Headers**: 
    -   `X-Thumbnail-URL`: Link to a temporary JPEG thumbnail for fast preview in the firmware if supported.

## Development

### Tech Stack
-   **Backend**: Go (Golang) + Echo Framework + GORM (SQLite).
-   **Frontend**: Vue 3 + Vite + Tailwind CSS.

### Commands
-   `make build`: Build Docker image.
-   `make run`: Run Docker container.
-   `make format`: Format Go and Frontend code.
