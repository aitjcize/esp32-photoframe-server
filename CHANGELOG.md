# Changelog

## v1.6.0 (Coming Soon)

### Added
- Telegram Bot: Support for multiple photos and random rotation
- Telegram Bot: Advanced management commands (`/list`, `/get`, `/delete`, `/clear`)
- Telegram Bot: Added inline "Delete" button for received photos
- Telegram Bot: Automatic whitelisting for the first user
- Telegram Bot: Per-photo caption saving and display support
- Telegram Bot: Instant push to multiple devices with real-time feedback
- Image: Support for "Smart Collage" with Telegram photos

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
