# Web photo frame

Open `/frame/` on the existing NAS service. The page uses original JPEGs from the
bounded frame sampler, independently of the AI publication pool. It does not
import or index the photo archive, upload images, or require a new server.

## A predictable, read-only surface

- Fixed full-window canvas and fixed-size thumbnail slots. No skeletons, loading
  text, counters, placeholders that resize, reload timers, or automatic redirects.
- Images download and decode out of view. Only a complete, decoded photo can
  replace the current canvas. Foreground and its softened background crossfade
  together. Reduced-motion preferences disable the crossfade.
- Clicking or dragging the photograph does nothing. Image dragging, text selection,
  drag/drop navigation, context menus, double-click actions, scroll and rubber-band
  movement are disabled inside the page. Browser/OS menus and shortcuts still work.
- Only explicit buttons or shortcuts change photos, pause, or enter fullscreen.
  The bottom controls reveal when the pointer moves into the bottom area or Down
  is pressed. They hide after inactivity. Merely focusing another app does not
  pause the photo display on a second monitor.
- Missing next photo means no movement, with no queued key presses. Network and
  decode failures leave the current photograph intact; retries are silent.
- A software update never forces an open photo display to reload.

## Controls and Mac installation

- Left/right: previous/next ready photo. Down: reveal controls. Up/Escape: hide.
- Space: play/pause. F or fullscreen button: request fullscreen.
- Normal playback shows each photo for ten seconds. Opening the controls pauses
  the timer; closing them gives the current photograph a fresh ten seconds.
- In Safari on macOS 14+, open the page, choose File > Add to Dock, open the saved
  web app and turn off navigation controls if desired. Move it to the other
  display, then use the Mac fullscreen button. Screen assignment after reconnect
  is managed by macOS; automatic display restoration is not implemented.
- System sleep and screen locking remain under macOS control. No wake lock or
  permission prompt is requested on opening the page.

## Cache, formats and offline limits

The client serially prepares up to twenty future photographs, refilling below
six. The timeline holds at most sixty entries, including at most forty earlier
entries. Current selection and ordered history restore from IndexedDB. Repeated
photo IDs can represent separate visits; downloading more never shuffles history.

Each display JPEG fits within 2560×1440 and is limited to 2 MiB; each thumbnail is
at most 32 KiB, with a small 128×72 softened background. At most sixty referenced
records plus one incoming record are retained, approximately 128 MiB or less of
application image data. Stale records and object URLs are cleaned as the timeline
advances. IndexedDB is browser-managed storage and may be reclaimed; private mode
or quota failures fall back to bounded memory for the current session.

Only JPEG originals up to 40 MiB and 40 megapixels are accepted by the web client.
The browser applies EXIF orientation once. The existing TV client is unchanged.
No full-size raw image is placed in the app's disk cache; browser responses for
web originals are marked no-store.

A service worker caches **only the application shell**, not an unbounded image
collection. This requires a trusted HTTPS origin (or localhost for development).
The NAS HTTP URL works online and keeps prepared photographs during outages in an
already-open window, but cannot guarantee a fresh offline launch. For that,
configure HTTPS for both page and API behind the same origin. Do not promise
that Add to Dock by itself provides offline support. Service worker upgrades
wait for existing clients to close; no forced activation or page refresh occurs.

## LAN access

Like the existing management page, the web frame is account-free on the LAN.
`/frame/*` rejects clients whose direct address is not private or loopback.
Sampling additionally requires a matching Origin, and cross-site image fetches
are rejected. No Android/worker bearer token is embedded in the page or URL.
Existing `/v1/frame/*` endpoints keep their bearer authentication.

This service and its account-free photo routes must remain on a trusted private
network. A reverse proxy must restrict outside access itself; the backend sees
the proxy's address. Preserve Host when proxying, including for HTTPS.

## Development and verification

```sh
node --test tests/frame-core.test.mjs
# From backend/:
go test -race ./...
```

For browser tests, generate synthetic JPEG fixtures (no private photo required):

```sh
go run tests/frame-fixtures.go
# From backend/ in a separate terminal:
NAS_PHOTO_TOKEN=test-only-photo-frame-token-not-production go run . \
  --root /tmp/nas-frame-fixtures --data /tmp/nas-frame-test-data \
  --listen 127.0.0.1:18791
```

Install Playwright in a temporary directory, then run `tests/frame-browser.mjs`
with `FRAME_PLAYWRIGHT_MODULE` pointing to its `index.mjs`. Use `FRAME_CHROME` to
select a local Chrome executable, or set `FRAME_BROWSER=webkit` after installing
Playwright's WebKit browser. Tests cover delayed images without layout movement,
inert canvas interactions, opaque thumbnails, unavailable next-photo presses,
offline scene stability, history restoration, and offline shell restart on
localhost. Generated screenshots remain outside the repository.

Deployment verification: backend 1.6.1 was installed on the NAS with a rollback
copy of the pre-web service. The existing AI pool remained intact. Live NAS
JPEGs rendered without JavaScript errors in Chrome. Synthetic-fixture browser
tests ran in Chrome and WebKit, including offline restart and selected-photo
restoration; image records use byte buffers for WebKit IndexedDB compatibility.
Preloading was additionally checked not to move the selected thumbnail.
The full Go race suite and focused route tests passed, together with seven
frontend state/geometry/blur tests. The Safari Add to Dock flow and a physical
second monitor have not been exercised in this session.
