# Changelog

## v1.11.3

### Fixed

- **Random photo selection now respects the device orientation** for DB-backed sources (Immich, Synology, gallery, Google Photos). Previously the normal (non-collage) flow ignored orientation, so a portrait frame could be served landscape photos even when portrait ones were available. It now prefers matching-orientation photos, falling back to any orientation only when none match (`auto`/no-EXIF photos still match either). (#40)

## v1.11.2

### Fixed

- **Photo overlays (date / weather / calendar) failed to render**, returning a 500 with `libLLVM.so.19.1` errors from headless Chrome. An earlier image-size cleanup deleted `libLLVM`, which Alpine 3.21's updated `mesa-gallium` (24.2.8) now hard-requires via `libgallium` — so freshly built images broke Chromium on both amd64 and arm64. Restored the removed libraries. (#39)

## v1.11.1

### Fixed

- **Synology:** manually pushing a selected photo failed with a 404 on libraries synced after the `external_id` migration — the push still used the legacy per-source id (0 for new rows). It now resolves the photo the same way the slideshow does. (#37)
- **Topic sources (Unsplash, Pexels):** manually pushing a selected photo failed with "image file not found on server" — those photos are remote URLs rather than local files, and are now downloaded before pushing.

## v1.11.0

### Added

- **Two-way config sync.** Settings changed on the device itself now propagate back to the server. The device advertises a config timestamp on each image fetch; when it's newer than the server's copy, the server asks the device to stay awake briefly (`X-Post-Rotate-Wait-Sec`) and pulls the updated config. Previously the sync was push-only (server → device), so on-device edits were silently ignored. Requires firmware v2.11.0+.

## v1.10.0

### Added

- **Cron-based rotation schedule builder.** The device auto-rotate schedule is now edited as repeating intervals, specific times, and day-of-week selection, compiled to simplified 3-field cron rules (`minute hour day-of-week`) — replacing the single rotation interval. A unit-test suite covers the cron parser, next-run preview, and rule compilation.

### Changed

- Pre-cron firmware is detected automatically and gets a simplified interval-only editor over the legacy API, so schedule edits can't be silently lost or leave the server showing a schedule the device isn't running.
- The Sleep schedule (quiet hours) is shown only for pre-cron firmware; cron firmware bounds its active hours in the rules instead (e.g. `0 7-23/2 *`, or two rules for overnight coverage).

### Fixed

- Immich: support the v3 album API that drops inline assets.

## v1.9.2

### Fixed
- **Referential integrity is now enforced by the database.** The junction/child tables (album memberships, device↔album/URL mappings, generative state, device history) had no foreign keys, so deleting a photo, album, device, or URL source relied on hand-written cleanup in every code path — and a missed path left orphaned rows behind (a real one was found in production). These now carry `ON DELETE CASCADE` (and `SET NULL` for device-history image references, so the served-history log survives a photo deletion), and the migration purges any pre-existing orphans.
- Startup now runs `VACUUM` once after a migration that rebuilds tables, reclaiming freed space.

## v1.9.1

### Fixed
- **Synced photos no longer vanish after a server restart.** On startup every source auto-synced at once, and their concurrent SQLite writes collided on the single-writer lock (`database is locked`). A source whose import lost that race imported nothing — and because sync cleared the gallery *before* re-importing, it was left empty. Most visible on Unsplash, whose single topic album meant one lost race wiped everything. Two fixes: database access is now serialized so the lock contention can't happen, and sync is non-destructive so a failed import can never wipe existing photos.
- Image dedup for Immich/Synology/Unsplash/Pexels is now enforced by a unique `(source, external_id)` constraint, not just application logic.

## v1.9.0

### Added
- **GC16 grayscale panel support.** Full processing + Floyd-Steinberg dithering pipeline for 16-level grayscale e-paper (IT8951 / reTerminal E1003), with per-device black/white-luminance and gamma calibration.
- **Unsplash & Pexels sources.** Turn any topic (e.g. `black and white`, `landscape`) into a synced album of free stock photography — ideal for a themed or grayscale art frame. Add your free API key, add topics, and each topic becomes an album you can assign to a frame.

### Changed
- **Devices only ever serve their assigned source.** A device is served solely the source configured for it on the server; the `/image/<source>` path can no longer point a device at a source it isn't assigned to (a device with no source set returns an error until it's configured under Settings → Devices).

### Fixed
- **Synology orientation.** Photos Synology returns without dimensions are no longer mislabeled as landscape — the server decodes a thumbnail to recover the true orientation, so portrait photos stop landing in landscape collage slots.
- Gallery thumbnails for Immich/Synology and the new URL-based sources.

## v1.8.2

### Changed
- **Unified Sources view.** The per-source settings are merged into the gallery: one tab per source (Gallery, Immich, Synology, Google, URL Proxy, AI Generation), each showing its gallery on top and that source's settings directly below. The separate "Data Sources" tab is gone; Devices + System remain as their own tabs.
- **Less clicking to set up a source.** Connecting Immich/Synology now auto-loads the album list (no separate "Refresh albums" step), and the gallery refreshes immediately after you clear photos.
- **Consistent connection controls.** Telegram and Synology now show a **Disconnect** button once configured (matching Immich/Google), and the gallery's Telegram/push-to-device settings save automatically (no "Update Settings" button).
- **Tab/label tidy-up.** "Google Photos" → "Google", Synology now sits before Google, the redundant "N photos synced" lines and the gallery info banner were removed, and the album-list border now matches the search field.

### Fixed
- Login form exposes proper `name`/`id`/autocomplete attributes so password managers (e.g. 1Password) reliably detect it. Note: iOS Safari still restricts autofill to HTTPS origins, so use an HTTPS URL (Tailscale/Nabu Casa/reverse proxy) on mobile.

## v1.8.1

### Security
- **JWT signing secret hardening.** The secret is now resolved as `JWT_SECRET` env var → a persisted `jwt_secret` setting → a freshly generated random secret (persisted), so a fresh install is secure with no configuration instead of falling back to a hard-coded default that made tokens forgeable. Existing installs migrate gracefully: device tokens issued under the old default keep working (frames stay online), but admin/session tokens do **not** validate under the legacy secret — a forged admin token can't be honored. Re-login once after upgrading.
- **Manual secret rotation.** A new **Regenerate** button (Settings → System) fully rotates the signing secret for users who want it — this signs you out and invalidates every device token, so each frame needs a new token generated and applied.

### Changed
- Internal code-quality pass: re-enabled TypeScript type-checking (`vue-tsc`) and enforced it in the build/CI/Docker pipeline; extracted reusable `<AlbumPicker>`, `useSnackbar()`, `getApiError()`, and a `SecurityTab` component out of the large Settings view; unified all backend handler error responses behind one helper.
- Added test coverage for the migration chain and the auth/token lifecycle (including the secret-rotation and legacy-fallback paths), and bumped CI actions to current versions.

## v1.8.0

### Added
- **Multi-album sync** for Immich and Synology: pick several albums per source and assign different album sets per device. Albums are now first-class entities (real albums plus Immich `all`/`favorites`/`memories` virtual albums), the gallery is grouped by album, and each album shows a live photo count.
- **Album picker UX**: type-to-filter search, alphabetical sorting, and checked albums grouped at the top for quick review/unchecking.
- **Configurable device-facing server URL** in Settings, so frames fetch images from the correct host in Tailscale / reverse-proxy setups instead of an auto-detected hostname.
- **Background sync with live feedback**: a sync-status endpoint drives an in-progress indicator and auto-refreshes the gallery as photos import, and the gallery tab is restored after a page refresh (hash routing).
- **Mobile-friendly Settings**: no more horizontal scrolling/accidental tab swipes on phones; the device dialog, device list, token list, and gallery header all adapt to small screens.

### Changed
- **Unified `/image` endpoint**: the photo source is resolved server-side from the requesting device (its token) instead of the URL path. The old `/image/:source` paths still work for back-compat, and per-source image URLs were removed from the UI.
- **Album selection now persists immediately**; syncing is manual (a Sync button) or prompted when you leave the tab — no more accidental resyncs on every checkbox toggle.
- Sync runs in the **background** (no more hung spinners), and "General" + "Security" were merged into a single **System** tab.
- Google Photos "Clear All Photos" now matches the Immich/Synology layout.
- **Smaller Docker image** (~1.83 GB → ~1.14 GB) by dropping the unused Mesa/LLVM GL stack and other runtime cruft.
- Internal cleanups: shared source-parameterized sync helpers, a unified Immich/Synology sync handler, and live album counts via a single query.

### Fixed
- **Per-device source isolation**: a configured frame can no longer be served a different source/album via a URL-path override — it always gets the source it's configured for.
- **Album import is now transactional and batched**, eliminating an N+1 per asset and preventing half-synced album state if an import fails midway.
- Deleting a device no longer 500s (it referenced a table that never existed) and now cleans up the device's child rows in one transaction.
- Album chips show live counts, emptied albums are hidden in the gallery, and a refresh no longer shows one tab's photos under another tab.
- The device API token is auto-provisioned for the bare `/image` URL again.

### Security
- **Token-only device identification**: the spoofable `X-Hostname` and client-IP fallbacks were removed; a device is identified solely by its token.
- Auth tokens are redacted from request logs, JWT validation is restricted to HS256, and the server warns when `JWT_SECRET` is unset.

## v1.7.8

### Added
- Immich `memories` sync gained a **Memory Years** option (`immich_memory_mode`): `all` (default — shuffle across every "on this day" lane from all past years) or `latest` (only the most recent year's lane, for a focused "last year on this day" experience). The selector appears in Settings only when Sync Mode is `memories`. Refs #32

### Changed
- The gallery preview at the top of Settings now refreshes automatically after an Immich or Synology sync, so freshly synced photos appear without a manual page reload

## v1.7.7

### Fixed
- Immich `memories` sync mode returned a random/unfiltered mix of photos instead of true "on this day" assets. `GetMemoryAssets()` called `/api/memories` with no query parameters, so Immich returned every persisted memory lane rather than the ones relevant to today. The request is now scoped with `for=<today, UTC>` and `type=on_this_day`. Closes #32

## v1.7.6

### Added
- Immich sync gained per-server **Sync Mode** in Settings: `album` (default, existing behavior), `all` (entire library via `/api/search/metadata`), `favorites` (Immich Favorites), and `memories` ("on this day" assets via `/api/memories`). The album picker only renders when mode is `album`. Closes #32
- Home Assistant add-on store now ships an `icon.png` (128×128 tile) and `logo.png` (250×100 banner) rendered from the firmware project's icon, so the add-on shares the brand mark with the rest of the ecosystem

### Changed
- Image source dispatch refactored into a flat plugin registry (`internal/imagesource.Source` + `Registry`). Each of the eight sources (`ai_generation`, `fractal`, `dla`, `gallery`, `immich`, `synology_photos`, `google_photos`, `url_proxy`) is now its own ~30-line plugin file owning one source name; the handler does a single `registry.Fetch` with zero per-source branching. The four library-backed sources share `RunDBPhotoFlow` (exclusion-aware pick + smart collage + photo-date lookup) parameterized by per-source pick/load callbacks. Adding a new source is now one file plus one `main.go` registration. `handler/image.go` shrinks ~365 lines. Fractal and DLA generative algorithms ported from the standalone `fractalgen` and `dla` CLIs (contributor: Christopher Rowley)

### Fixed
- Gallery uploads from iPhone (and any source that writes EXIF `Orientation` instead of rotating pixels) showed up sideways on the device and in the gallery. Uploads and Google Photos sync now run `magick -auto-orient` to bake the orientation into the pixel grid and reset the tag to 1. Telegram, Immich, and Synology paths were already covered
- Smart collage on a portrait device paired a landscape first photo with a portrait (not landscape) second photo, which `DrawCover` then cropped to a wide strip — and symmetrically on landscape devices. The second-photo query now targets the slot's shape (opposite of the device) instead of the device's own orientation

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
