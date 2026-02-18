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
  "cameras.offline": "Offline"
}
```

```json
// pl.json
{
  "nav.title": "Monitoring CCTV",
  "nav.cameras": "Kamery",
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
  "cameras.offline": "Offline"
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
- Click a result: opens detail view with larger image, link to play source video

### Camera Dashboard
- Grid of live snapshots (refreshed every 10s)
- Camera status indicators (online/offline)
- Quick link to main page with that camera pre-selected

### Face Registry
- List of enrolled persons with thumbnail
- Upload new face photo to enroll
- Delete enrolled person

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
