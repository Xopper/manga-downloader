# Library first: the window's new layout

Design chosen on the canvas "Manga Downloader layouts", journey 1 (boards 1a
"Library home" and 1b "Series page"). Colours, type and the existing controls
stay; the layout and the flow change.

## Who it's for

A reader who follows a handful of series and opens the app to grab what is
new. The home screen is their library, not an empty URL box.

## Placement rule

- Top: things that are always there. The brand, one search box (a title, or
  a pasted link), *Check for new chapters*, Settings.
- Middle: the current page. Home (the library), the search results, or a
  series.
- Right (series page only): what belongs to that series. Its facts, Follow,
  how it saves, Download.
- Bottom: what is happening now. A running download as a strip, then the
  status bar with the log.

## Pages

### Home: the library

- Header: "Library", "N series you follow · M new chapters · checked X ago",
  a sort (new first / title / last downloaded) and *Download all new · M*.
- A grid of cards, one per followed series: the cover (fetched from the
  series page in the background; the first letter until then), the title, "site · N chapters", a status line, and one button.
  - Status: "K new · saved up to C", "Up to date · all N saved", "Not checked
    yet", "search only · can't download from this site yet", or the last
    check's error.
  - Button: *Download K new* (loads the series, selects the chapters above
    the saved mark, starts), *Nothing new* (disabled), or *Open on the site*
    for a search-only series.
  - A bookmark on the cover's corner unfollows.
  - The title opens the series page (loading it when it isn't the loaded
    one).
- Empty library: the mascot and "Nothing followed yet. Search a title above
  and press Follow on it."
- The foot says the list lives on this computer only.

### Results

Typing a title in the top box (anything that isn't an http(s) link) shows
the results in the middle instead of the page underneath: a table with the
title, site, language, chapters and a follow star. The chapter count is the
one the app last saw for that series (a series it has loaded or follows);
"—" otherwise, with a tooltip saying it fills in once the series is opened.
Picking a row loads it and opens the series page; a search-only row keeps
the current "can't download from here yet" note with Copy link and Open in
browser. The site picker stays, as a select in the results toolbar. Clearing
the box returns to the page underneath.

Pasting a link loads it (as the URL bar did: after a short pause, or on
Enter) and opens the series page.

### Series page

- A breadcrumb "← Library / Title".
- Middle panel: the chapter list as today (virtual list, Shift spans,
  volumes), with a chip row above it: *New · K* (selects the chapters above
  the saved mark) and *All · N*, the language and scanlator selectors, and
  the existing select-all / count / range toolbar. A followed series marks
  its rows: "new" for chapters above the saved mark, "saved" otherwise.
- Right panel: cover (or the first letter), title, site link; *Following* / *Follow*;
  the facts (chapters on the site, new since the last download, saved up to,
  last downloaded); "Save as": format, reading direction (EPUB), pack as,
  folder; then the Download button with its blocker text, as the dock has
  now.
- The loading and failed states keep their current look, in the middle.

### Settings

A drawer from the gear with the rest of the options: convert images, skip
pages, filename template, the browser window switch, the catalogs line
(sites, titles, updated, *Update*).

### Bottom

While a download runs, a strip: title, "chapter X · a of b done", bar,
percentage, *Details* (opens the Activity drawer: today's panel) and
*Stop*. Under it, the downloads waiting their turn (see "The download
queue"), each with a remove button. When a run ends, the strip shows the
summary headline with *Open folder* and *Details* until dismissed or the
next run. Under it the status bar as today (log toggle, last line, version,
update badge).

### The download queue (added 2026-10-18)

A download never locks the window. Once started, a run keeps its own copy
of everything it needs (site, chapters, settings, output folder), so the
user can open, search and follow other series, change options and pick
chapters while it goes. Pressing *Download* while a run is active appends
the selection to a queue, first in first out, instead of being refused; the
next job starts when the running one ends, whether it finished, failed or
was stopped (remove the waiting ones first to stop everything). The dock
shows the run's progress only on the page of the series it downloads; on
another series it keeps *Download*, with a line saying these start after
the current one.

Two kinds of job. A *selection* job (Download on the series page) keeps the
chapters picked from the list loaded at the time and downloads exactly
those; a *fresh* job (*Download N new*, *Download all new*) names a followed
series and fetches its list when it starts, then takes what the library
counts as new. *Download all new* queues one fresh job per series instead of
owning a run. The same series and chapters queued twice are refused.

The library check stays refused while a download runs (and a download while
it runs): the two share the browser and the sites, and refusing is simpler
than sharing. The window says so on the button.

The queue is the desktop app's alone: the CLI downloads what it's given and
exits, so there is nothing to queue there. This is one of the deliberate
one-sided features (see "Design every feature for both the CLI and the
desktop app" in `AGENTS.md`).

## The library on disk

`library.json` next to `gui.json` (0600, atomic writes):

```json
{
  "followed": [
    {"url": "https://mangadex.org/title/…/one-piece", "title": "One Piece",
     "site": "mangadex.org", "downloadable": true, "chapters": 1120,
     "latest": 1120, "savedUpTo": 1118, "new": 2,
     "followedAt": "2026-10-03T…", "checkedAt": "2026-10-15T…", "error": ""}
  ],
  "seen": {
    "<url key>": {"title": "Berserk", "chapters": 379, "latest": 379, "at": "…"}
  }
}
```

- `savedUpTo` is the highest chapter number the app saved for the series;
  `new` is how many listed chapters are numbered above it, as of the last
  load or check. A series never downloaded has `savedUpTo` 0, so every
  chapter is new and the card says *Download all N*.
- `seen` remembers every series the app loaded (title, count, latest), at
  most 500, newest kept. It feeds the chapter counts in the results.
- A URL is keyed trimmed, scheme and host lower-cased, trailing slash
  dropped.

## What the backend does

- Follow / unfollow from the series page or a result. Following the loaded
  series records its count and latest at once.
- Every successful load updates the followed entry (and `seen`).
- Every finished run raises `savedUpTo` to the highest chapter it saved (a
  bundle counts its chapters; a volume its last chapter) and recomputes
  `new` from the loaded list.
- *Check for new chapters*: fetches each followed, downloadable series in
  turn without touching the loaded one, updating its count, latest, new and
  error. Refused while a series loads or a download runs; loads and
  downloads are refused while it runs. It reports progress ("3 of 5").
- *Download new* for one series queues a fresh job (its list is fetched
  when it starts, the loaded series is left alone). *Download all new*
  queues one per series with new chapters; they run one after the other,
  with the queue in the strip (see "The download queue").

## Out of scope, noted

- Exact chapter counts for series the app has never opened. Since 2026-10-15
  the nightly catalog carries the latest chapter number for the Madara
  family (`Entry.Latest`, whose list pages show it) and the results show it
  as "≈ N" (`HitDTO.approximate`) until the app opens the series; the other
  families' lists don't say.
- Per-series save settings. The options stay global.
