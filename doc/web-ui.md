# Web UI

[Back to main design](DESIGN.md)

## Tech Stack

- **React 18** + TypeScript
- **Vite** for bundling
- **Tailwind CSS** for styling
- **React Query** for API data fetching
- **react-i18next** + **i18next** for internationalization (EN / PL)
- **date-fns** for timestamp formatting

## Internationalization (i18n)

The UI supports English and Polish. All user-facing strings are stored in
translation JSON files, loaded via react-i18next.

```
frontend/src/
  i18n/
    en.json          # English translations
    pl.json          # Polish translations
    i18n.ts          # i18next config (default: pl, fallback: en)
```

Language switcher in the nav bar (flag icon or "EN / PL" toggle). Selected
language is persisted in `localStorage`.

Example translation keys:
```json
// en.json
{
  "nav.title": "CCTV Intelligence",
  "nav.cameras": "Cameras",
  "nav.faces": "Faces",
  "process.title": "Select footage to process",
  "process.cameras": "Cameras",
  "process.date": "Date",
  "process.button": "Process",
  "process.progress.downloading": "Downloading from {{camera}}...",
  "process.progress.extracting": "Extracting frames from {{camera}}...",
  "process.progress.indexing": "Indexing frames...",
  "search.title": "Search",
  "search.placeholder": "Describe what you're looking for...",
  "search.button": "Search",
  "search.translated": "Searching for: {{query}}",
  "results.camera": "Camera",
  "results.time": "Time",
  "results.score": "Score",
  "cameras.title": "Camera Dashboard",
  "cameras.online": "Online",
  "cameras.offline": "Offline",
  "faces.title": "Face Registry",
  "faces.discover": "Discover Faces",
  "faces.discovering": "Clustering faces...",
  "faces.clusters": "Discovered Clusters",
  "faces.cluster.count": "{{count}} faces",
  "faces.cluster.assign": "Assign",
  "faces.cluster.name_placeholder": "Enter name...",
  "faces.cluster.skip": "Skip",
  "faces.enrolled": "Enrolled Persons",
  "faces.enrolled.empty": "No persons enrolled yet",
  "faces.enrolled.delete": "Delete",
  "faces.enrolled.faces_count": "{{count}} face samples",
  "faces.no_clusters": "No unassigned faces found. Process some footage first.",
  "faces.assigned": "Name assigned successfully",
  "video.play": "Play video",
  "video.close": "Close player",
  "video.loading": "Loading video...",
  "video.error": "Video unavailable",
  "video.camera": "Camera",
  "video.timestamp": "Timestamp",
  "video.seek_hint": "Seeked to capture moment"
}
```

```json
// pl.json
{
  "nav.title": "Monitoring CCTV",
  "nav.cameras": "Kamery",
  "nav.faces": "Twarze",
  "process.title": "Wybierz nagrania do przetworzenia",
  "process.cameras": "Kamery",
  "process.date": "Data",
  "process.button": "Przetwórz",
  "process.progress.downloading": "Pobieranie z {{camera}}...",
  "process.progress.extracting": "Ekstrakcja klatek z {{camera}}...",
  "process.progress.indexing": "Indeksowanie klatek...",
  "search.title": "Szukaj",
  "search.placeholder": "Opisz czego szukasz...",
  "search.button": "Szukaj",
  "search.translated": "Wyszukiwanie: {{query}}",
  "results.camera": "Kamera",
  "results.time": "Czas",
  "results.score": "Wynik",
  "cameras.title": "Podgląd kamer",
  "cameras.online": "Online",
  "cameras.offline": "Offline",
  "faces.title": "Rejestr twarzy",
  "faces.discover": "Wykryj twarze",
  "faces.discovering": "Grupowanie twarzy...",
  "faces.clusters": "Wykryte grupy",
  "faces.cluster.count": "{{count}} twarzy",
  "faces.cluster.assign": "Przypisz",
  "faces.cluster.name_placeholder": "Wpisz imię...",
  "faces.cluster.skip": "Pomiń",
  "faces.enrolled": "Zarejestrowane osoby",
  "faces.enrolled.empty": "Brak zarejestrowanych osób",
  "faces.enrolled.delete": "Usuń",
  "faces.enrolled.faces_count": "{{count}} próbek twarzy",
  "faces.no_clusters": "Brak nieprzypisanych twarzy. Najpierw przetwórz nagrania.",
  "faces.assigned": "Imię przypisane pomyślnie",
  "video.play": "Odtwórz wideo",
  "video.close": "Zamknij odtwarzacz",
  "video.loading": "Ładowanie wideo...",
  "video.error": "Wideo niedostępne",
  "video.camera": "Kamera",
  "video.timestamp": "Czas",
  "video.seek_hint": "Przewinięto do momentu ujęcia"
}
```

## Pages

### Main Page — Process & Search

Two-phase UI on a single page:

**Phase 1: Select & Process**
- Camera selector (checkboxes, loaded from config)
- Date picker (single date or date range)
- "Process" button — starts download + extraction + indexing
- Progress bar with stage labels (Downloading → Extracting → Indexing)
- If camera+date was already processed, shows "Ready" immediately

**Phase 2: Search (enabled after processing completes)**
- Search bar for text query
- Toggle: "Text Search" / "Person Search"
  - Text Search: type a natural language query
  - Person Search: select from enrolled persons dropdown
- Results grid: thumbnails with timestamp, camera name, similarity score
- Play button overlay on each thumbnail (visible on hover) — opens video player modal
- Click a result: opens detail view with larger image, metadata

### Camera Dashboard
- Grid of live snapshots (refreshed every 10s)
- Camera status indicators (online/offline)
- Quick link to main page with that camera pre-selected

### Face Registry (Discovery-Based)

**Discover Faces:**
- "Discover Faces" button triggers clustering of all unassigned faces across the
  entire database (all cameras, all dates)
- Loading state while clustering runs (can take time for large datasets)

**Cluster Review:**
- Grid of discovered face clusters
- Each cluster shows up to 5 representative face thumbnails and total count
- Name input field per cluster
- "Assign" button to save name → creates a face registry entry
- Option to skip/dismiss clusters (faces remain unassigned for future discovery)

**Enrolled Persons:**
- List of already-named persons with their face samples
- Face count per person
- Delete button to remove an enrolled person
- Option to merge: drag a cluster onto an existing person to add faces

**No manual upload** — enrollment is discovery-only. Faces are found in
processed frames and presented for naming.

## Responsive / Mobile

The UI must be fully usable on phones and tablets. Tailwind's responsive
utilities (`sm:`, `md:`, `lg:`) handle all breakpoints — no separate mobile
build is needed.

### Breakpoint strategy

| Breakpoint | Grid columns | Notes |
|---|---|---|
| < 640px (mobile) | 1–2 | Single-column forms, 2-col result grid |
| 640–1024px (tablet) | 2–3 | Side-by-side form fields, 3-col results |
| > 1024px (desktop) | 3–4 | Full layout as shown in wireframes below |

### Component adaptations

- **Nav bar**: collapses to a hamburger menu on mobile; language toggle and nav
  links move into a slide-out drawer
- **Camera selector**: checkboxes stack vertically on mobile instead of inline
- **Date picker**: full-width input on mobile
- **Search bar**: full-width, search button below the input on mobile
- **Results grid**: 2 columns on mobile (thumbnails scale down), 3–4 on wider
  screens. Camera name and score move below the thumbnail instead of beside it
- **Play button overlay**: always visible on mobile (no hover state on touch
  devices); uses `@media (hover: none)` to keep the play icon permanently shown
  at reduced opacity
- **Video player modal**: full-screen on mobile (no side margins), with a
  prominent close button in the top corner. The `<video>` element uses `width: 100%`
  and maintains aspect ratio
- **Face cluster review**: clusters stack vertically on mobile; representative
  face thumbnails scroll horizontally within each cluster card
- **Camera dashboard**: 1-column snapshot grid on mobile, 2 on tablet

### Touch considerations

- All tap targets are at least 44×44px (Apple HIG / WCAG minimum)
- Swipe gestures are not used — standard scroll and tap only
- Video player uses native HTML5 controls on mobile (better touch UX than
  custom controls)

## Layout

```
+------------------------------------------------------------------+
|  CCTV Intelligence              [Cameras]  [EN|PL]               |
+------------------------------------------------------------------+
|                                                                   |
|  Step 1: Select footage to process                               |
|                                                                   |
|  Cameras: [x] Front Door  [x] Driveway  [ ] Backyard            |
|  Date:    [2026-02-18] to [2026-02-18]                           |
|                                                           [Process]|
|                                                                   |
|  [===============================>          ] 72%                |
|  Extracting frames from Front Door...                            |
|                                                                   |
+------------------------------------------------------------------+
|                                                                   |
|  Step 2: Search                                                  |
|                                                                   |
|  [==== "show me deliveries today" ========================] [Go] |
|  Mode: (x) Text Search  ( ) Person Search                       |
|                                                                   |
+------------------------------------------------------------------+
|                                                                   |
|  +----------+  +----------+  +----------+  +----------+          |
|  |          |  |          |  |          |  |          |          |
|  |  thumb   |  |  thumb   |  |  thumb   |  |  thumb   |          |
|  |          |  |          |  |          |  |          |          |
|  +----------+  +----------+  +----------+  +----------+          |
|  Front Door    Driveway      Front Door    Front Door            |
|  14:23:05      14:25:10      14:31:42      14:45:18              |
|  score: 0.82   score: 0.79   score: 0.76   score: 0.71          |
|                                                                   |
+------------------------------------------------------------------+
```

## Video Playback

### PlayButtonOverlay (`PlayButtonOverlay.tsx`)

A semi-transparent play icon centered on each `ResultCard` thumbnail. The icon
fades in on hover, providing a clear affordance that clicking will play the source
video.

```
+----------+       +----------+
|          |       |    ▶     |   ← hover reveals play icon
|  thumb   |  →    |  thumb   |     (semi-transparent white circle + triangle)
|          |       |          |
+----------+       +----------+
```

Props: `onClick: () => void`

### VideoPlayerModal (`VideoPlayerModal.tsx`)

An inline modal with an HTML5 `<video>` element that plays the source video
segment, seeked to the exact frame capture time.

```
+------------------------------------------------------------------+
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  Front Door — 2026-02-18 14:23:05                        [X] | |
|  +--------------------------------------------------------------+ |
|  |                                                              | |
|  |                                                              | |
|  |                      <video>                                 | |
|  |                    (HTML5 player)                             | |
|  |                                                              | |
|  |                                                              | |
|  +--------------------------------------------------------------+ |
|  |  Seeked to capture moment (23:05)                            | |
|  +--------------------------------------------------------------+ |
|                                                                    |
+------------------------------------------------------------------+
  ^^^^ dark semi-transparent backdrop, click to close
```

**Behavior:**
- On open: loads video from `source_video_url`
- On `loadedmetadata` event: sets `video.currentTime = seekOffsetSec`
- Autoplays muted (browser autoplay policy requires muted), user can unmute
- Close on Escape key or clicking the backdrop
- Shows camera name and formatted timestamp above the player
- Shows seek hint below the player (e.g., "Seeked to capture moment")
- On 404 (video purged by retention cleanup): shows "Video unavailable" message

**Props:**
```typescript
interface VideoPlayerModalProps {
  isOpen: boolean;
  onClose: () => void;
  sourceVideoUrl: string;
  seekOffsetSec: number;
  cameraName: string;
  timestamp: string;
}
```

### useVideoPlayer Hook (`useVideoPlayer.ts`)

Manages the video player modal state: which video is open, seek offset, and
open/close actions.

```typescript
interface VideoPlayerState {
  isOpen: boolean;
  sourceVideoUrl: string;
  seekOffsetSec: number;
  cameraName: string;
  timestamp: string;
}

function useVideoPlayer() {
  // Returns:
  //   state: VideoPlayerState
  //   openVideo(result: SearchResult): void
  //   closeVideo(): void
}
```

Called from `MainPage.tsx`. `openVideo` is passed down to each `ResultCard`, which
renders `PlayButtonOverlay` wired to it. `VideoPlayerModal` is rendered once at
the page level, controlled by the hook's state.

### Updated TypeScript Types

```typescript
// api/types.ts — additions
interface SearchResult {
  // ... existing fields ...
  source_video_url?: string;
  seek_offset_sec: number;
}
```
