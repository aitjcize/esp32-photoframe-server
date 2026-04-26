# Changelog

## v1.7.5

### Fixed
- Gallery delete failed with HTTP 500 ("database is locked") and 26-30s latency under concurrent image fetches. SQLite is now opened in WAL mode with a 30s busy_timeout (`_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL`), so the per-fetch `device_history` writer no longer starves user-facing writes. The per-fetch insert + prune is also wrapped in a single transaction so the writer holds the lock once instead of three times
- Gallery `DeletePhoto` / `DeletePhotos` now drop the DB row before removing the file, so a timed-out delete can no longer leave a row pointing at a missing file. Thumbnail handler returns 404 instead of 500 when the source file is gone
- Immich users got a Synology-flavored empty-state message; each source now has its own copy that points at the correct Data Sources tab

### Changed
- Gallery card and delete UI restyled to match the device webapp: outlined cards with hover lift/shadow, photos shown at native aspect ratio inside square cards (`contain` + grey letterbox), `mdi-delete` trash-icon affordance revealed on top-right corner hover, inline delete dialogs with thumbnail preview and a destructive Delete button
- Both per-photo and "Delete All" actions now require confirmation
- Gallery shows 6 photos per row at large viewports (down from 8)

## v1.7.4

### Added
- Standalone server-side gallery: photos uploaded from the web UI or sent to the Telegram bot are stored under `data/photos/gallery/` and tracked in the images table. Telegram is now an upload path into the gallery rather than a separate source; push-to-device on bot upload still works when enabled. Migration `000022` rewrites any existing `images.source` / `settings.image_source` rows from `telegram` to `gallery`

### Changed
- Update AI model list: drop deprecated DALL-E entries, add Gemini 3.1 Flash
- Split server-owned device fields from hardware-derived ones in the device model

### Fixed
- Renderer: emit CSS `contain` for fit display mode so cropped images render correctly

### Performance
- Rewrite `device_histories` prune as a range delete so it stays O(log n) once a device's history grows past a few hundred rows

### Build
- Bump Alpine base image to 3.21 for newer libheif

## v1.7.3

### Fixed
- Device image URL is now built against the add-on port (default `9607`, configurable via `VITE_ADDON_PORT`) instead of `window.location.origin`, so the URL works when the webapp is served through Home Assistant ingress (`:8123`) but the ESP32 reaches the server directly

### Performance
- Index the hot-path GORM queries that were doing sequential scans: `api_keys(user_id)` and `(user_id, device_id)` (token lookups on every device config save), `devices(host)` (LAN fallback identification on every image fetch), and `images(source)` / `(source, synology_photo_id)` / `(source, immich_asset_id)` (per-photo dedup during Synology and Immich sync)

## v1.7.1

### Added
- Walnut photo-frame icon, used as both the favicon and a 32×32 prepend in the app-bar so the title bar reads as a branded header

### Changed
- New warm amber color palette: primary `#ce9160`, with matched-saturation error / info / success / warning (`#982f2f`, `#2f6398`, `#2f9852`, `#987e2f`)

### Fixed
- Image fetch failures propagate as errors instead of falling back to a picsum placeholder

## v1.7.0

### Added
- Remote device configuration: manage device settings from the server. The device dialog is now a tabbed UI matching the device webapp (General, Auto Rotate, Processing, Palette, Home Assistant), fetches live config from the device, and pushes changes back on Save with an offline fallback message.
- Display orientation dropdown shows resolution (e.g., "Portrait 480×800")

### Changed
- Display mode "contain" renamed to "fit" for consistency
- Image orientation detection unified across sources; orientation is passed to the CLI for device-aware processing
- Bind flow simplified: image URL and token generated on Save (no separate Bind button)

### Performance
- Cache resolved IP on photoframe client and reuse clients across fetches
- Use dns-sd for fast mDNS resolution on macOS
- Composite index on `device_histories(device_id, served_at)`
- Prune device_histories to 50 entries, remove unnecessary COUNT query

### Fixed
- Increase HTTP client timeout to 120s for image push operations
- Reuse existing device token instead of revoking and regenerating when re-binding
- Return 404 instead of 500 when no URL proxy sources are configured
- Use `--ignore-scripts` for frontend npm install to skip canvas native build

## v1.6.1

### Added
- EPDGZ format: serve compressed 4-bit-per-pixel images by default, saving bandwidth and enabling instant display rendering on the device. Automatically falls back to PNG for firmware older than v2.6.2.
- Internet operation: bind device tokens to specific device IDs for reliable identification over the internet without hostname/IP matching
- Auto-sync scheduler for Immich and Synology photo sources with configurable intervals
- Dev Container configuration for streamlined local development

### Fixed
- Synology: automatically re-login when session expires using saved device token (bypasses 2FA on trusted devices)
- Synology auto-sync UI: fix indentation in settings panel
- Device push: return 503 error when device is unreachable instead of misleading "queued" response
- Token management: add missing PUT endpoint for updating device binding on tokens
- Token backfill: skip ambiguous matches when multiple devices share the same name

## v1.6.0

### Added
- Overlay: display photo creation date from Immich EXIF metadata and Synology timestamps
- Overlay: new per-device "Show Photo Date" toggle in device settings

### Fixed
- Docker: pin Alpine to 3.20 to fix canvas native module build failure with GCC 15
- Docker: fix Go toolchain version mismatch in builder stage
- Immich: fix photo orientation for portrait photos with EXIF rotation (orientations 5-8)
- Increase HTTP client timeout to 120s for slow e-ink display updates
- Restore authentication to image serving endpoint
- Add IPv6 link-local fallback to mDNS transport

## v1.5.6

### Added
- Loading spinner for device list and gallery source switching

### Fixed
- Immich: fix concurrent image request failures by adding preview API fallback
- Immich: fix connection failures with .local mDNS hostnames resolving to link-local IPv6
- Immich: fix data race on shared client during concurrent requests
- Immich: fix source binding for device configuration
- Immich: include response body in error messages for better debugging
- Synology: fix .local mDNS IPv6 link-local connection issues
- Parallelize initialization fetches for faster startup

## v1.5.4

### Added
- Immich: gallery tabs now default to Immich, reordered as Immich → Google Photos → Synology

### Fixed
- Synology: personal album thumbnails no longer return 404

## v1.5.3

### Added
- Google Calendar integration: display today's events as an overlay on the frame
- Calendar: show at least 1 event entry on small screens

## v1.5.2

### Fixed
- Synology: empty orientation field no longer causes layout issues
- Collage: fix potential duplicate photo in collage

## v1.5.0

### Added
- AI Generation: support for Gen AI image rotation
- Overlay: scale fonts and UI elements based on image size

## v1.4.9

### Fixed
- Fix port binding and configuration propagation when running as HA add-on
- Fix auto-binding URL port detection for add-on environment

## v1.4.8

### Changed
- Build Docker images for both x86 and amd64 in CI
- Switch from prebuilt Docker image to local builds
- Fix ingress API base URL for HA add-on
- Fix data location migration to `/data`
- Migrate persistent data to `/data` directory for HA add-on compatibility

## v1.4.6 / v1.4.5

### Added
- Login session management
- Allow HA add-on to appear in the HA side panel
- Admin username and password can now be changed
- Auto-binding: frames are automatically bound to a data source on first connection
- Device binding: manually bind devices to specific data sources
- URL proxy data source support

### Fixed
- Prevent the same image from being served repeatedly

## v1.4.1

### Added
- Multi-device support with per-device resolution settings
- Push image directly from server to a specific frame
- Smart collage: automatically create side-by-side collages when photo orientation mismatches screen

## v1.3.3

### Added
- Push image from server to frame

### Fixed
- Remove stale device last-seen records
- Show an error when the target device is not reachable during push

## v1.3.1

### Fixed
- Fix npm package installation in Docker build

## v1.3.0

### Changed
- Updated UI style to match new firmware web app
- Switched image processing to the `epaper-image-convert` package

## v1.2.1

### Fixed
- Fix clipboard copy for image URL

## v1.2.0

### Added
- Authentication: login with username and password required to access the UI and API

### Fixed
- Set correct `Content-Length` header on image serving endpoint

## v1.1.2

### Added
- Display the image serving endpoint URL in the UI

### Fixed
- Various bug fixes

## v1.1.1

### Fixed
- Fix OAuth redirect URL for Google Photos
- Telegram: push received photo to frame when device is reachable

## v1.1.0

### Added
- Synology DSM Photos integration
- Google Photos and Synology integrations can now be used side by side

## v1.0.2

### Changed
- Improved overlay rendering styles

## v1.0.1

### Fixed
- Fix OAuth redirect URL for Google Photos authentication
