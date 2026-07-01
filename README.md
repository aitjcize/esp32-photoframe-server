# ESP32 PhotoFrame Server

A image server for the [ESP32 PhotoFrame](https://github.com/aitjcize/esp32-photoframe) project. This server acts as a bridge between the E-paper display and various photo sources (Google Photos, Immich, Synology Photos, Unsplash, Pexels, and more), handling image processing, resizing, dithering, and overlay generation.

## Features

### Supported Sources

Mix and match photo sources per device:

| Source | Description |
| --- | --- |
| Gallery | Photos you upload from the dashboard, or send in via a [Telegram bot](https://core.telegram.org/bots). |
| [Google Photos](https://photos.google.com/) | Pick albums & photos securely via the Picker API. |
| [Immich](https://immich.app/) | Self-hosted photo server — sync selected albums or the All / Favorites / Memories views. |
| [Synology Photos](https://www.synology.com/en-us/dsm/feature/photos) | Your Synology NAS (DSM 7 Personal & Shared spaces). |
| [Unsplash](https://unsplash.com/) · [Pexels](https://www.pexels.com/) | Free stock-photo search — add topics (e.g. `black and white`, `landscape`) and each becomes a synced album. |
| URL Proxy | Display images from any image URL. |
| AI Generation | Generate images with [OpenAI](https://platform.openai.com/) (GPT Image, DALL·E) or [Google Gemini](https://ai.google.dev/). |

### Other Features

- **Smart Image Processing**:
    - Automatic cropping to each device's aspect ratio and resolution.
    - **Smart Collage**: Automatically combines two landscape photos in portrait mode (or vice versa) to maximize screen usage.
    - **Dithering**: Floyd-Steinberg dithering for both Spectra 6 color and GC16 grayscale e-paper panels, with per-device palette/gamma calibration.
- **Overlays**:
    - Customizable Date/Time display.
    - Real-time Weather status (Temperature + Condition) based on location.
    - "iPhone Lockscreen" style aesthetics with Inter font and drop shadows.
- **Authentication**:
    - User account system with login/registration.
    - Revocable API tokens for device access.
    - Session management.

## Deployment

### Home Assistant Add-on (Recommended)

The easiest way to run the server is as a Home Assistant add-on.

#### Installation

1. **Add Repository**:
   - Go to **Settings** → **Add-ons** → **Add-on Store** → **⋮** (three dots) → **Repositories**
   - Add: `https://github.com/aitjcize/esp32-photoframe-server`

2. **Install Add-on**:
   - Find "ESP32 PhotoFrame Server" in the add-on store
   - Click **Install**
   - Wait for the build to complete (5-15 minutes on first install)

3. **Configure**:
   - The add-on uses `/data` for persistent storage (automatically backed up)
   - Port 9607 is exposed for direct device access
   - **Ingress** is enabled - access via Home Assistant sidebar

4. **Start**:
   - Click **Start**
   - Enable **Start on boot** if desired
   - Access via the sidebar or `http://homeassistant.local:9607`

#### Data Migration

If upgrading from a previous version that used `/config/esp32-photoframe-server/`:
- Data is automatically migrated to `/data` on first startup
- Check logs to verify migration completed successfully
- Old data in `/config` can be manually removed after verification

### Docker (Standalone)

For non-Home Assistant deployments:

```bash
docker run -d \
  -p 9607:9607 \
  -v /path/to/data:/data \
  --name photoframe-server \
  aitjcize/esp32-photoframe-server:latest
```

## Configuration

Access the dashboard at `http://localhost:9607` (or your server IP, or via Home Assistant ingress).

### Initial Setup

1. **Create Account**:
   - On first launch, you'll be prompted to create an admin account
   - Enter a username and password

2. **Configure a Source & Frame**:
   - Set up a photo source under **Settings** → **Data Sources** (see [Supported Sources](#supported-sources) below).
   - Configure your frame under **Settings** → **Devices**.
   - Device access tokens are issued and managed automatically — you no longer generate or copy them by hand.

### Google Photos Setup

> [!IMPORTANT]
> **Google OAuth Restriction**: Google does not allow `.local` domains or private IP addresses in OAuth redirect URIs. If running on Home Assistant, you must use one of these methods:
> - **Port Forwarding** (recommended for one-time setup): `ssh -L 9607:localhost:9607 root@homeassistant.local -p 22222`
> - **Public Domain**: Use a domain name with Cloudflare Tunnel or similar

#### Steps:

1. **Create OAuth Credentials**:
   - Go to [Google Cloud Console](https://console.cloud.google.com/)
   - Create a new project or select an existing one
   - Enable the **Google Photos Picker API**
   - Go to **Credentials** → **Create Credentials** → **OAuth 2.0 Client ID**
   - Application type: **Web application**
   - **Authorized JavaScript Origins**: `http://localhost:9607`
   - **Authorized Redirect URIs**: `http://localhost:9607/api/auth/google/callback`
   - Click **Create** and save your Client ID and Client Secret

2. **Configure the Server**:
   - If running on Home Assistant, set up port forwarding first:
     ```bash
     ssh -L 9607:localhost:9607 root@homeassistant.local -p 22222
     ```
   - Access the dashboard at `http://localhost:9607`
   - Go to **Settings** → **Data Sources**
   - Select **Source: Google Photos**
   - Enter your **Client ID** and **Client Secret**
   - Click **Save All Settings**

3. **Authenticate and Import Photos**:
   - Go to the **Gallery** tab
   - Click **Add Photos via Google**
   - You'll be redirected to Google OAuth (sign in if needed)
   - Select the photos you want to display
   - Click **Add** to import them

4. **After Setup**:
   - The OAuth token is saved in the database
   - You can close the SSH tunnel (if used)
   - Access the server normally via Home Assistant ingress or `http://homeassistant.local:9607`
   - Re-authentication is only needed if you revoke access or want to add more photos

### Synology Setup

1. Go to **Settings** → **Data Sources** in the dashboard.
2. Enable **Synology Photos**.
3. Enter your **NAS URL** (e.g., `https://192.168.1.10:5001`), **Account**, and **Password**.
4. If using 2FA, enter the **OTP Code** when testing the connection.
5. Select the **Photo Space** (Personal or Shared) and optionally a specific **Album**.
6. Click **Sync Now** to import metadata.

### Immich Setup

1. Go to **Settings** → **Data Sources** and open the **Immich** tab.
2. Enter your **Server URL** (e.g., `https://immich.example.com`) and an **API Key** (in Immich: **Account Settings** → **API Keys**).
3. Click **Connect** to validate, then choose the albums to sync — or the **All Photos**, **Favorites**, or **Memories** views.
4. Click **Sync Now** to import.

### Telegram Bot (Gallery)

A Telegram bot is an optional way to add photos to the **Gallery** source — send a photo to the bot and it's uploaded to your gallery.

1. Create a new bot via [@BotFather](https://t.me/botfather) and copy its **Bot Token**.
2. Go to **Settings** → **Data Sources** → **Gallery** and enter the **Telegram Bot Token**, then save.
3. Send a photo to your bot. It's added to the Gallery, and frames using the Gallery source will show it.

### URL Proxy Setup

1. Go to **Settings** → **Data Sources**.
2. Select **Source: URL Proxy**.
3. Add URLs to images you want to display.
4. Assign URLs to specific devices.

### AI Generation Setup

Generate unique AI artwork for your photo frame using OpenAI or Google Gemini.

1. **Get an API Key**:
   - **OpenAI**: Get your key at [platform.openai.com/api-keys](https://platform.openai.com/api-keys)
   - **Google Gemini**: Get your key at [aistudio.google.com/app/apikey](https://aistudio.google.com/app/apikey)

2. **Configure API Keys**:
   - Go to **Settings** → **Data Sources** → **AI Generation**
   - Enter your API key(s) and click **Save API Keys**

3. **Configure Per-Device**:
   - Go to **Settings** → **Devices**
   - Click **Edit** on the device you want to configure
   - Under **AI Image Generation**, select a provider (OpenAI or Google Gemini)
   - Choose a model and enter a prompt describing the images you want
   - Click **Save**

4. **Available Models**:
   - **OpenAI**: GPT Image 1, DALL-E 3, DALL-E 2
   - **Google Gemini**: Gemini 2.5 Flash Image, Gemini 3 Pro Image

### Stock Photo Search (Unsplash & Pexels)

Turn any topic into a rotating album of free stock photography — great for a themed or black-and-white art frame.

1. **Get a free API key**:
   - **Unsplash**: create an app at [unsplash.com/developers](https://unsplash.com/developers) and copy the **Access Key** (the Secret Key is not needed).
   - **Pexels**: request a key at [pexels.com/api](https://www.pexels.com/api/).
2. Go to **Settings** → **Data Sources**, open the **Unsplash** or **Pexels** tab, paste the key, and click **Connect**.
3. Add one or more **topics** (e.g., `black and white`, `mountains`, `minimalist`). Each topic becomes a synced album and saves automatically.
4. Click **Sync Now** to import, then assign the source (and specific topics) to a device under **Settings** → **Devices**.

## Photo Frame Configuration

Frames are configured from the dashboard — the server issues the access token and pushes settings to the device automatically:

1. Open **Settings** → **Devices** and select your frame.
2. In the **Auto Rotate** tab, enable **"Use this server"** and pick a **Source** (and, for album/topic sources, which albums or topics to rotate through).
3. Save. The server generates the device's token and pushes the image URL and settings to the frame — no manual token or URL copying required.

## API Endpoints (For ESP32)

### Image Endpoints

- **`GET /image`**: The endpoint frames use. The device is identified by its bearer token and served its **server-assigned source** (cropped and dithered for its panel). Configure the source under **Settings → Devices**; a device with no source assigned gets an error.
- **`GET /image/<source>`**: A legacy URL form still routed for older firmware — the `<source>` is **ignored**. A device is only ever served the source assigned to it in the server, never one it isn't assigned.

### Authentication

All image endpoints require authentication via Bearer token:

```
Authorization: Bearer <your-device-token>
```

#### Building

```bash
# Build Docker image
docker build -t esp32-photoframe-server .

# Or use make
make build
make run
```

### Home Assistant Add-on Development

Use the included `deploy-dev.sh` script for rapid local testing:

```bash
./deploy-dev.sh [ssh-host]
```

This script:
- Syncs code to Home Assistant's local add-on directory
- Modifies config for development (port 9608, dev slug)
- Triggers Supervisor to rebuild and restart the add-on

## Support

If you find this project useful, consider buying me a coffee! ☕

[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-ffdd00?style=for-the-badge&logo=buy-me-a-coffee&logoColor=black)](https://buymeacoffee.com/aitjcize)

## License

MIT License - see LICENSE file for details.
