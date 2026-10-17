// A stand-in for the Go App, for working on the UI in a plain browser
// (pnpm dev) and in tests. It keeps its own state, simulates a load and a
// download, and pushes the same events the Go side does.
//
// Add ?mock to the URL to use it inside the app too. ?scenario= starts it in
// a given state, so screenshots need no clicking: empty, loading, error,
// loaded, stale, running, bundle, cancelling, done, update, check, volumes
// (loaded, packing by volume, with three volumes and a half selected) or
// volumes-running (a run by volume in progress). update is loaded too, as a
// release build (v1.17.0 with &shot, v1.12.0 without) with a newer release
// (v1.18.0 with &shot, v1.13.0 without): the status bar shows the yellow "!".
// check is loading a site that needs the browser window, with the alert asking
// the user to look at it; loading a URL with "check" in it does the same (and
// one with "slow" in it just takes 3 s) until
// the load ends. &freeze stops the simulated run from moving (stable
// screenshots), &dupes lists some numbers twice in two languages, &chapters=N
// shows the newest N chapters, &novolumes lists no volumes (the series
// otherwise files its chapters by their real One Piece volume), &locked marks
// some chapters locked (see below), &fail makes
// the fourth chapter fail halfway and &skip makes the sixth one come out
// skipped, every page excluded by --skip-pages (both off by default; every run
// saves every chapter without them), &fetching has each chapter's row spend its
// first few ticks fetching its page list (frozen, the newest rows are still
// fetching), &epub starts the output as EPUB, &prompt
// (with update) makes the update one-click and opens its prompt on load, &shot
// uses production release versions and a macOS output path (for screenshots),
// and VITE_MOCK=1 at build time includes the mock in production builds (e.g.
// for the screenshot capture build; set via Wails frontend env).
//
// &site= picks what the series lists for the two filters: multi (the default:
// languages ca, en, es with All, Catalan shown by default, no groups), plus
// (Manga Plus: languages without All, the URL edition shown by default),
// groups (groups with All, every group shown by default, each chapter listed
// once per group, no languages), or none (both fields disabled: a typed value shows cleared, as
// the site ignores it, and is kept for the sites that take it). A language or
// group the series doesn't list falls back on load, as the backend does: typed
// "xx" under multi loads "all", under plus the edition the URL points at.
// Typed values match ignoring case, and take the spelling the series lists.
// &locked lists every 50th chapter as locked: the list shows its lock
// and never selects it, as the backend does.

import type { Api, Unsubscribe } from './api';
import { newIndices, urlKey } from './library';
import type { CatalogInfo, Chapter, FilterChoice, Followed, Hit, Job, LibraryInfo, LogLine, Options, SiteInfo, State, Summary } from './types';
import { groupChapters, parseVolumes, siteVolumes } from './volumes';
import { OP_CHAPTERS } from './mock-onepiece';

const TEMPLATE_DEFAULT =
  '{{.Series}} {{.Number}} - {{.Title}}{{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}';
const TEMPLATE_WARNING =
  'This template has no {{.Number}}, {{.Title}}, {{.SiteTitle}} or {{.Volume}}, so files only differ by a version suffix.';

/** Mirrors packer.TemplateWarning */
export function templateWarning(templ: string): string {
  const t = templ.trim();
  return t === '' || /\{\{-?\s*[^}]*\.(Number|Title|SiteTitle|Volume)\b/.test(t) ? '' : TEMPLATE_WARNING;
}

/** Total chapters in the Catalan One Piece feed (MangaDex, ch 598-1194) */
const CHAPTERS = OP_CHAPTERS.length; // 597
const PAGES = 12;
// a page's steps while downloading and while archiving, as core's
// DownloadWeight and ArchiveWeight: a bar fills 80% in the download
const DOWNLOAD_WEIGHT = 4;
const ARCHIVE_WEIGHT = 1;
const STEPS_PER_PAGE = DOWNLOAD_WEIGHT + ARCHIVE_WEIGHT;
const TICK_MS = 160;

const SCENARIOS = [
  'empty',
  'loading',
  'error',
  'loaded',
  'reloading',
  'stale',
  'running',
  'bundle',
  'cancelling',
  'done',
  'update',
  'check',
  'volumes',
  'volumes-running',
] as const;

export type Scenario = (typeof SCENARIOS)[number];

const SITES = ['multi', 'plus', 'groups', 'none'] as const;

export type MockSite = (typeof SITES)[number];

export interface MockSetup {
  scenario: Scenario;
  /** The simulated run doesn't advance */
  freeze: boolean;
  /** Some numbers are listed twice, in two languages */
  dupes: boolean;
  chapters: number;
  site: MockSite;
  /** The series lists no volumes */
  novolumes: boolean;
  /** The fourth chapter fails halfway through download */
  fail: boolean;
  /** The sixth chapter comes out skipped (every page excluded by --skip-pages) */
  skip: boolean;
  /** Every 50th chapter is listed as locked */
  locked: boolean;
  /** Use production release versions and a macOS output path (for screenshots) */
  shot: boolean;
  /** Chapter rows start fetching their page list before they download */
  fetching: boolean;
  /** Start with EPUB as the output format */
  epub: boolean;
  /** Start without any search catalog on disk */
  nocatalog: boolean;
  /** Start with nothing followed */
  nolibrary: boolean;
  /** With the update scenario: one-click update, its prompt open on load */
  prompt: boolean;
}

/** What the mock series offers for each filter; null when the site lists none */
interface MockOffer {
  values: string[];
  all: boolean;
  /** The value shown when no filter is set, for the sites that show one */
  current: string;
}

interface MockOffers {
  language: MockOffer | null;
  scanlator: MockOffer | null;
}

const OFFERS: Record<MockSite, MockOffers> = {
  multi: { language: { values: ['ca', 'en', 'es'], all: true, current: 'ca' }, scanlator: null },
  plus: { language: { values: ['en', 'es'], all: false, current: 'en' }, scanlator: null },
  groups: { language: null, scanlator: { values: ['Alpha', 'Delta'], all: true, current: '' } },
  none: { language: null, scanlator: null },
};

const MOCK_ALERT =
  "Brave opened a window to pass a check on check.example.com. There may be a captcha or puzzle to solve there; the download continues by itself once it's done.";

const REPO = 'https://github.com/elboletaire/manga-downloader';
const RELEASE_VERSION_DEV = 'v1.12.0';
const RELEASE_VERSION_SHOT = 'v1.18.0';
const UPDATE_VERSION_DEV = 'v1.13.0';
const UPDATE_VERSION_SHOT = 'v1.19.0';

function siteFrom(search: string): MockSite {
  const value = new URLSearchParams(search).get('site');
  return (SITES as readonly string[]).includes(value ?? '') ? (value as MockSite) : 'multi';
}

function scenarioFrom(search: string): Scenario {
  const value = new URLSearchParams(search).get('scenario');
  return (SCENARIOS as readonly string[]).includes(value ?? '') ? (value as Scenario) : 'empty';
}

/** The mock's whole setup from the page's query string */
export function mockSetupFrom(search: string): MockSetup {
  const params = new URLSearchParams(search);
  const n = Number(params.get('chapters'));
  return {
    scenario: scenarioFrom(search),
    freeze: params.has('freeze'),
    dupes: params.has('dupes'),
    chapters: Number.isInteger(n) && n > 0 ? n : CHAPTERS,
    site: siteFrom(search),
    novolumes: params.has('novolumes'),
    fail: params.has('fail'),
    skip: params.has('skip'),
    locked: params.has('locked'),
    shot: params.has('shot'),
    fetching: params.has('fetching'),
    epub: params.has('epub'),
    nocatalog: params.has('nocatalog'),
    nolibrary: params.has('nolibrary'),
    prompt: params.has('prompt'),
  };
}

/** A fake cover, for the mock only: a flat 2:3 panel with a circle, in the given colours */
function mockCover(ground: string, mark: string): string {
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 200 300'><rect width='200' height='300' fill='${ground}'/><circle cx='100' cy='118' r='46' fill='${mark}'/><rect x='40' y='200' width='120' height='14' rx='4' fill='${mark}' opacity='0.7'/><rect x="60" y="226" width="80" height="10" rx="4" fill='${mark}' opacity='0.45'/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

/** The mock's library: the mock series (12 chapters past its saved mark), one up to date, one search only; the first two have covers */
/** A followed entry's first list is known, as the backend's MarkSeen takes it */
function baseline(f: Followed, chapters: Chapter[]): void {
  if (!f.baselined) {
    f.known = [...new Set(chapters.map((c) => c.number))].sort((a, b) => a - b);
    f.baselined = true;
  }
}

function mockLibrary(): Followed[] {
  return [
    {
      url: 'https://example.com/manga/one-piece',
      title: 'One Piece',
      site: 'example.com',
      downloadable: true,
      chapters: CHAPTERS,
      latest: 1194,
      savedUpTo: 1182,
      saved: OP_CHAPTERS.map(([number]) => number).filter((number) => number <= 1182),
      everSaved: OP_CHAPTERS.map(([number]) => number).filter((number) => number <= 1182),
      known: OP_CHAPTERS.map(([number]) => number).filter((number) => number <= 1182),
      baselined: true,
      new: 12,
      followedAt: '2026-10-03T10:00:00Z',
      checkedAt: '2026-10-15T08:50:00Z',
      lastSaved: '2026-10-11T19:00:00Z',
      error: '',
      cover: mockCover('#1d4ed8', '#fbbf24'),
    },
    {
      url: 'https://www.mangaread.org/manga/berserk/',
      title: 'Berserk',
      site: 'mangaread.org',
      downloadable: true,
      chapters: 379,
      latest: 379,
      savedUpTo: 379,
      saved: Array.from({ length: 379 }, (_, i) => i + 1),
      everSaved: Array.from({ length: 379 }, (_, i) => i + 1),
      known: Array.from({ length: 379 }, (_, i) => i + 1),
      baselined: true,
      new: 0,
      followedAt: '2026-09-20T10:00:00Z',
      checkedAt: '2026-10-15T08:50:00Z',
      lastSaved: '2026-10-02T19:00:00Z',
      error: '',
      cover: mockCover('#3f3f46', '#e4e4e7'),
    },
    {
      url: 'https://manhwaclan.example/manga/kingdom/',
      title: 'Kingdom',
      site: 'manhwaclan.example',
      downloadable: false,
      chapters: 0,
      latest: 0,
      savedUpTo: 0,
      saved: [],
      everSaved: [],
      known: [],
      baselined: false,
      new: 0,
      followedAt: '2026-10-12T10:00:00Z',
      checkedAt: '',
      lastSaved: '',
      error: '',
      cover: '',
    },
  ];
}

// the extension each format's output carries; raw writes a folder without one
const EXTENSIONS: Record<Options['format'], string> = { cbz: '.cbz', epub: '.epub', raw: '' };

function defaultOptions(shot: boolean, epub: boolean): Options {
  return {
    outputDir: shot ? '/Users/reader/Downloads/Manga' : '/home/reader/Downloads',
    format: epub ? 'epub' : 'cbz',
    readingDirection: 'rtl',
    packAs: 'chapters',
    language: '',
    scanlator: '',
    convertImages: 'avif',
    savePagesAs: 'original',
    skipPages: '',
    filenameTemplate: TEMPLATE_DEFAULT,
    browserVisible: false,
    flaresolverr: '',
  };
}

/**
 * Returns the newest `count` chapters from the One Piece Catalan dataset.
 * Volume 0 means not yet collected. With withLanguages, some chapters also
 * appear in English to simulate a multi-language listing.
 */
function mockChapters(count: number, withLanguages: boolean, withVolumes = true, withLocked = false): Chapter[] {
  const list: Chapter[] = [];
  const source =
    count >= OP_CHAPTERS.length ? OP_CHAPTERS : OP_CHAPTERS.slice(OP_CHAPTERS.length - count);

  for (const [number, catTitle, vol] of source) {
    const volume = withVolumes ? vol : 0;
    const locked = withLocked && isMockLocked(number);
    list.push({ index: list.length, number, title: catTitle, language: withLanguages ? 'ca' : '', group: '', volume, locked });
    if (withLanguages && number % 5 === 0) {
      list.push({ index: list.length, number, title: catTitle, language: 'en', group: '', volume, locked });
    }
  }
  return list;
}

/** The error a locked chapter fails with, as core.ErrLocked says it */
const LOCKED_REASON = 'locked chapter';

/** Every 50th chapter is locked (a listing's coin lock), so a few show in dev. */
function isMockLocked(number: number): boolean {
  return number % 50 === 0;
}

/** Every chapter by Alpha, and every fifth again by Delta, so a number repeats in one language. */
function groupedChapters(list: Chapter[]): Chapter[] {
  const out: Chapter[] = [];
  for (const c of list) {
    out.push({ ...c, group: 'Alpha' });
    if (c.number % 5 === 0) {
      out.push({ ...c, group: 'Delta' });
    }
  }

  return out.map((c, index) => ({ ...c, index }));
}

type Listener<T> = (v: T) => void;

class Bus<T> {
  private listeners = new Set<Listener<T>>();
  subscribe(cb: Listener<T>): Unsubscribe {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
  }
  emit(v: T): void {
    for (const cb of this.listeners) {
      cb(v);
    }
  }
}

export function createMockApi(setup: Scenario | Partial<MockSetup> = 'empty'): Api {
  const {
    scenario,
    freeze,
    dupes,
    chapters: chapterCount,
    site,
    novolumes,
    fail,
    skip,
    locked,
    shot,
    fetching,
    epub,
    nocatalog,
    nolibrary,
    prompt,
  } = {
    scenario: 'empty' as Scenario,
    freeze: false,
    dupes: false,
    chapters: CHAPTERS,
    site: 'multi' as MockSite,
    novolumes: false,
    fail: false,
    skip: false,
    locked: false,
    shot: false,
    fetching: false,
    epub: false,
    nocatalog: false,
    nolibrary: false,
    prompt: false,
    ...(typeof setup === 'string' ? { scenario: setup } : setup),
  };
  const offers = OFFERS[site];
  const options = defaultOptions(shot, epub);
  let url = '';
  let loading = false;
  let loadError = '';
  let loaded = false;
  let title = '';
  let chapters: Chapter[] = [];
  let selected = new Set<number>();
  // the Volumes box and the site's volumes, as the backend keeps them per load
  let volumesText = '';
  let volumesSite = '';
  let applied = { language: '', scanlator: '' };
  // whether a load has been attempted, so the filters are the series' own choices
  let identified = false;
  let running = false;
  let cancelling = false;
  let runChapters = 0;
  // the bundle option as the run started with it, as the backend's RunInfo
  let runBundle = false;
  let runVolumes = false;
  let runRows = 0;
  let jobs: Job[] = [];
  let summary: Summary | null = null;
  let log: LogLine[] = [];
  let timer: ReturnType<typeof setInterval> | undefined;
  // the loaded site needs the browser window, and the alert about it
  let browserAuto = false;
  let browserAlert = '';
  // the search catalog: a fixed list over three sites, or none with &nocatalog
  let catalogSites = nocatalog ? [] : MOCK_CATALOG;
  let catalogUpdating = false;
  let catalogGeneration = 1;
  let catalogUpdatedAt = nocatalog ? '' : '2026-10-15T03:20:00Z';
  const catalogInfo = (): CatalogInfo => ({
    sites: catalogSites.length,
    entries: catalogSites.reduce((n, s) => n + s.entries.length, 0),
    searchOnly: catalogSites.filter((s) => !s.downloadable).length,
    updatedAt: catalogUpdatedAt,
    updating: catalogUpdating,
    error: '',
    generation: catalogGeneration,
  });

  // the library: the followed series, what the mock saw of every series it
  // loaded (chapter counts for the hits), the check and the batch
  let followed: Followed[] = nolibrary ? [] : mockLibrary();
  const seen = new Map<string, number>();
  let checking = false;
  let checkDone = 0;
  let checkTotal = 0;
  let batch = { active: false, done: 0, total: 0, current: '', currentURL: '' };
  let batchStop = false;
  const followedOf = (target: string): Followed | undefined => followed.find((f) => urlKey(f.url) === urlKey(target));
  const libraryInfo = (): LibraryInfo => ({
    followed: followed.map((f) => ({ ...f })),
    checking,
    checkDone,
    checkTotal,
    batch: { ...batch },
  });

  const stateBus = new Bus<State>();
  const chaptersBus = new Bus<Chapter[]>();
  const selectionBus = new Bus<number[]>();
  const logBus = new Bus<LogLine[]>();

  const selectionOf = () => [...selected].sort((a, b) => a - b);
  const pushLog = (level: LogLine['level'], text: string) => {
    log = [...log, { level, text }].slice(-500);
    logBus.emit(log);
  };
  // what a filter's selector shows: the value the list was loaded with, else
  // the site's default (its current value, else every value where it takes all)
  const filterOf = (kind: 'language' | 'scanlator'): FilterChoice | null => {
    const offer = offers[kind];
    if (!identified || offer === null) {
      return null;
    }
    const loadedWith = applied[kind];
    let selected = loadedWith;
    if (loadedWith === '') {
      selected = offer.current !== '' ? offer.current : offer.all ? 'all' : '';
    }
    return { values: [...offer.values], all: offer.all, selected };
  };
  // a typed value the series doesn't offer falls back as the backend does: to
  // "all" where the site takes it, else to the site's current value, else to no
  // filter. A value that matches one ignoring case takes the listed spelling.
  const fallbackFor = (kind: 'language' | 'scanlator', label: string): void => {
    const offer = offers[kind];
    const typed = options[kind];
    if (offer === null || typed === '' || typed === 'all') {
      return;
    }
    const listed = offer.values.find((v) => v.toLowerCase() === typed.toLowerCase());
    if (listed !== undefined) {
      options[kind] = listed;
      return;
    }
    const fallback = offer.all ? 'all' : offer.current;
    options[kind] = fallback;
    const shown = fallback === 'all' ? 'all of them' : fallback === '' ? 'no filter' : `"${fallback}"`;
    pushLog('info', `${label} "${typed}" isn't in this series, loading ${shown} instead`);
  };

  const releaseVersion = shot ? RELEASE_VERSION_SHOT : RELEASE_VERSION_DEV;
  // update stands for a release build, and so does every scenario with &shot,
  // which reports a release version: the status bar links it to its release
  const isReleaseBuild = (sc: Scenario) => sc === 'update';
  const reportsRelease = shot || isReleaseBuild(scenario);

  const snapshot = (): State => ({
    url,
    loading,
    loaded,
    loadError,
    title,
    chapterCount: chapters.length,
    hasDuplicates: chapters.some((c) => c.language !== ''),
    hasGroupDuplicates: chapters.some(
      (c) => c.group !== '' && chapters.some((d) => d !== c && d.number === c.number && d.language === c.language),
    ),
    stale: loaded && !loading && (options.language !== applied.language || options.scanlator !== applied.scanlator),
    loadedLanguage: applied.language,
    loadedScanlator: applied.scanlator,
    languages: filterOf('language'),
    scanlators: filterOf('scanlator'),
    selectedCount: selected.size,
    running,
    cancelling,
    run: { chapters: runChapters, bundle: runBundle, volumes: runVolumes, rows: runRows },
    jobs: jobs.map((j) => ({ ...j })),
    summary: summary ? { ...summary, failures: [...summary.failures] } : null,
    options: { ...options },
    volumes: { text: volumesText, site: volumesSite },
    version: reportsRelease ? releaseVersion : 'v0.0.0-mock',
    releaseURL: reportsRelease ? `${REPO}/releases/tag/${releaseVersion}` : '',
    update: isReleaseBuild(scenario)
      ? {
          latest: shot ? UPDATE_VERSION_SHOT : UPDATE_VERSION_DEV,
          url: `${REPO}/releases/latest`,
          oneClick: !shot || prompt,
          ...(prompt && { prompt: true }),
        }
      : null,
    browserVisibleAuto: browserAuto,
    browserAlert,
    catalog: catalogInfo(),
    library: libraryInfo(),
    filenameTemplateDefault: TEMPLATE_DEFAULT,
    filenameTemplateWarning: options.packAs === 'chapters' ? templateWarning(options.filenameTemplate) : '',
  });
  const push = () => stateBus.emit(snapshot());

  function finishLoad(): void {
    loading = false;
    loaded = true;
    identified = true;
    fallbackFor('language', 'language');
    fallbackFor('scanlator', 'scanlation group');
    title = 'One Piece';
    chapters = mockChapters(chapterCount, dupes || url.includes('dupes') || options.language === 'all', !novolumes, locked);
    if (offers.scanlator !== null) {
      chapters = groupedChapters(chapters);
    }
    volumesSite = siteVolumes(chapters);
    volumesText = volumesSite;
    applied = { language: options.language, scanlator: options.scanlator };
    selected = new Set<number>();
    jobs = [];
    summary = null;
    pushLog('info', `Loaded ${title}: ${chapters.length} chapters`);
    seen.set(urlKey(url), chapters.length);
    const f = followedOf(url);
    if (f !== undefined) {
      f.title = title;
      f.chapters = chapters.length;
      f.latest = Math.max(...chapters.map((c) => c.number));
      baseline(f, chapters);
      f.new = newIndices(chapters, f).length;
      f.checkedAt = new Date().toISOString();
      f.error = '';
    }
    chaptersBus.emit(chapters);
    selectionBus.emit([]);
    push();
  }

  function failLoad(): void {
    loading = false;
    loaded = false;
    identified = true;
    applied = { language: options.language, scanlator: options.scanlator };
    chapters = [];
    selected = new Set<number>();
    loadError = "Couldn't fetch the chapter list: unexpected status 403 Forbidden";
    pushLog('error', loadError);
    chaptersBus.emit([]);
    selectionBus.emit([]);
    push();
  }

  function finishRun(): void {
    clearInterval(timer);
    timer = undefined;
    running = false;
    // a volume's chapters count as saved with it, as the backend counts them
    const saved = jobs
      .filter((j) => j.state === 'done')
      .reduce((n, j) => n + (j.volume ? (volumeChapters.get(j.title) ?? 0) : 1), 0);
    const volumes = jobs.filter((j) => j.volume && j.state === 'done').length;
    const failures = jobs.filter((j) => j.state === 'failed').map((j) => `${j.title}: ${j.reason}`);
    const skipped = jobs.filter((j) => j.state === 'skipped').length;
    const bundled = runBundle;
    summary = {
      dir: options.outputDir,
      cancelled: cancelling,
      saved: bundled && failures.length === 0 && !cancelling ? runChapters : saved,
      volumes,
      bundled,
      bundle: bundled && !cancelling ? `${title} 1-${runChapters}${EXTENSIONS[options.format]}` : '',
      skipped,
      failures,
      error: '',
    };
    pushLog('info', cancelling ? `Cancelled: ${saved} saved` : `Finished: ${saved} saved`);
    if (failures.length > 0) {
      pushLog('warn', `${failures.length} chapter(s) failed`);
    }
    // a run that saved something moves the library's mark, as the backend does
    const f = followedOf(url);
    const savedNumbers = bundled && !cancelling ? runNumbers : jobs.filter((j) => j.state === 'done' && !j.volume).map((j) => j.number);
    if (f !== undefined && savedNumbers.length > 0) {
      f.savedUpTo = Math.max(f.savedUpTo, ...savedNumbers);
      f.saved = [...new Set([...f.saved, ...savedNumbers])].sort((a, b) => a - b);
      f.everSaved = [...new Set([...f.everSaved, ...savedNumbers])].sort((a, b) => a - b);
      f.new = newIndices(chapters, f).length;
      f.lastSaved = new Date().toISOString();
    }
    push();
  }

  // the chapters of the run that haven't started; the backend only creates a
  // job when a chapter starts, so the mock does too
  let pending: Chapter[] = [];
  // the numbers of the chapters the run was started with, for the library's mark
  let runNumbers: number[] = [];
  // how many chapters each volume row of the run holds, by title
  let volumeChapters = new Map<string, number>();
  // with &fetching, the ticks a chapter's row has left fetching its page list
  const fetchLeft = new Map<string, number>();
  const FETCH_TICKS = 4;

  /** A tick's steps: the pace it had when a page was two steps, scaled to STEPS_PER_PAGE. */
  function ticks(n: number): number {
    return Math.round((n * STEPS_PER_PAGE) / 2);
  }

  /** The step at which a bar of total steps switches from downloading to archiving. */
  function downloadEnd(total: number): number {
    return (total * DOWNLOAD_WEIGHT) / STEPS_PER_PAGE;
  }

  /** One step of the simulated run. Returns false once it's over. */
  function step(): boolean {
    if (runBundle) {
      const j = jobs[0];
      const done = Math.min(j.total, j.done + ticks(4));
      const fetched = done < downloadEnd(j.total);
      jobs = [
        {
          ...j,
          done,
          state: done >= j.total ? 'done' : fetched ? 'downloading' : 'archiving',
          status:
            done >= j.total
              ? ''
              : fetched
                ? `Downloading chapter ${Math.floor((done / downloadEnd(j.total)) * runChapters) + 1}...`
                : 'Creating the bundle...',
        },
      ];
      return done < j.total;
    }

    // the volumes all have rows from the start; two move at a time, as the
    // backend downloads chapters in order
    let movingVolumes = 0;
    jobs = jobs.map((j) => {
      if (!j.volume || (j.state !== 'downloading' && j.state !== 'archiving') || movingVolumes >= 2) {
        return j;
      }
      movingVolumes++;
      const done = Math.min(j.total, j.done + ticks(5));
      if (done >= j.total) {
        return { ...j, done, state: 'done', status: '' };
      }
      const packing = done >= downloadEnd(j.total);
      return {
        ...j,
        done,
        state: packing ? 'archiving' : 'downloading',
        status: packing ? 'Packing...' : '',
      };
    });

    jobs = jobs.map((j) => {
      const left = fetchLeft.get(j.title);
      if (j.state !== 'fetching' || left === undefined) {
        return j;
      }
      if (left > 1) {
        fetchLeft.set(j.title, left - 1);
        return j;
      }
      fetchLeft.delete(j.title);
      return { ...j, state: 'downloading', total: PAGES * STEPS_PER_PAGE, status: '' };
    });

    jobs = jobs.map((j, idx) => {
      if (j.volume || (j.state !== 'downloading' && j.state !== 'archiving')) {
        return j;
      }
      if (fail && idx === 3 && j.done >= downloadEnd(j.total) / 2) {
        return { ...j, state: 'failed', reason: 'download: page 5 returned 404 Not Found' };
      }
      // uneven speeds, so the rows don't move in lockstep
      const done = Math.min(j.total, j.done + ticks(1 + ((j.number * 7) % 4)));
      const state = done >= j.total ? 'done' : done >= downloadEnd(j.total) ? 'archiving' : 'downloading';
      return { ...j, done, state };
    });

    const active = jobs.filter(
      (j) => !j.volume && (j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving'),
    ).length;
    if (!cancelling && active < 3 && pending.length > 0) {
      const c = pending.shift()!;
      if (c.locked) {
        // as the backend's FetchFailed: a listed lock fails before any fetch,
        // with a row unless the run is a bundle
        if (!runBundle) {
          jobs = [
            ...jobs,
            {
              title: c.title,
              tags: '',
              number: c.number,
              total: 0,
              done: 0,
              state: 'failed',
              reason: LOCKED_REASON,
              status: '',
              volume: false,
            },
          ];
        }
        pushLog('error', `error fetching ${c.title}: ${LOCKED_REASON}`);
        return true;
      }
      if (skip && jobs.filter((j) => !j.volume).length === 5) {
        // as the backend's Skipped event: a row with nothing to download
        jobs = [
          ...jobs,
          {
            title: c.title,
            tags: '',
            number: c.number,
            total: 0,
            done: 0,
            state: 'skipped',
            reason: 'every page excluded by --skip-pages',
            status: '',
            volume: false,
          },
        ];
        pushLog('warn', `skipping ${c.title}: every page is excluded by --skip-pages`);
        return true;
      }
      if (fetching) {
        fetchLeft.set(c.title, FETCH_TICKS);
      }
      jobs = [
        ...jobs,
        {
          title: c.title,
          tags: '',
          number: c.number,
          total: fetching ? 0 : PAGES * STEPS_PER_PAGE,
          done: 0,
          state: fetching ? 'fetching' : 'downloading',
          reason: '',
          status: fetching ? 'Fetching pages...' : '',
          volume: false,
        },
      ];
    }

    return (
      jobs.some((j) => j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving') ||
      (!cancelling && pending.length > 0)
    );
  }

  function beginRun(list: Chapter[]): void {
    running = true;
    runNumbers = list.map((c) => c.number);
    cancelling = false;
    runChapters = list.length;
    runBundle = options.packAs === 'bundle';
    runVolumes = options.packAs === 'volumes';
    summary = null;
    pending = [...list];
    volumeChapters = new Map();
    fetchLeft.clear();
    runRows = runBundle ? 1 : list.length;
    if (runVolumes) {
      // the backend makes a row per volume up front, and runs the chapters
      // outside them one by one
      const grouping = groupChapters(list, parseVolumes(volumesText));
      pending = grouping.loose;
      runRows = grouping.volumes.length + grouping.loose.length;
      jobs = grouping.volumes.map((v) => {
        const first = v.chapters[0].number;
        const last = v.chapters[v.chapters.length - 1].number;
        const vol = `Vol. ${String(v.number).padStart(2, '0')}`;
        const title = first === last ? `${vol} · chapter ${first}` : `${vol} · chapters ${first}\u2013${last}`;
        volumeChapters.set(title, v.chapters.length);
        return {
          title,
          tags: '',
          number: first,
          total: v.chapters.length * PAGES * STEPS_PER_PAGE,
          done: 0,
          state: 'downloading' as const,
          reason: '',
          status: 'Waiting...',
          volume: true,
        };
      });
      return;
    }
    jobs = runBundle
      ? [
          {
            title: `Bundle: chapters 1-${list.length}`,
            tags: '',
            number: 0,
            total: list.length * PAGES * STEPS_PER_PAGE,
            done: 0,
            state: 'downloading',
            reason: '',
            status: 'Gathering info...',
            volume: false,
          },
        ]
      : [];
  }

  function tick(): void {
    if (!step()) {
      finishRun();
      return;
    }
    push();
  }

  function simulateRun(list: Chapter[]): void {
    beginRun(list);
    push();
    if (!freeze) {
      timer = setInterval(tick, TICK_MS);
    }
  }

  if (scenario === 'check') {
    url = 'https://check.example.com/manga/one-piece';
    loading = true;
    browserAuto = true;
    browserAlert = MOCK_ALERT;
    if (!freeze) {
      setTimeout(() => {
        browserAlert = '';
        finishLoad();
      }, 4000);
    }
  } else if (scenario === 'loading') {
    url = 'https://example.com/manga/one-piece';
    loading = true;
    if (!freeze) {
      setTimeout(finishLoad, 2500);
    }
  } else if (scenario === 'error') {
    url = 'https://unknown.example.org/manga/some-series';
    failLoad();
    log = [];
    pushLog('info', `Loading ${url}`);
    pushLog('error', loadError);
  } else if (scenario !== 'empty') {
    url = 'https://example.com/manga/one-piece';
    finishLoad();
    for (const c of [...chapters.slice(0, 12), ...chapters.slice(40, 52)]) {
      if (!c.locked) {
        selected.add(c.index);
      }
    }
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'reloading') {
    // a series opened again: its saved list up, locked, until the site answers
    loading = true;
    push();
    if (!freeze) {
      setTimeout(finishLoad, 4000);
    }
  }
  if (scenario === 'stale') {
    options.language = 'es';
  }
  if (scenario === 'update') {
    // A realistic site-check failure: selector timeout including the full series URL.
    // The URL is long enough to overflow the status bar without the CSS fix.
    if (!shot) {
      pushLog(
        'warn',
        'site check: timed out waiting for "li.list-group-item, img.PB0mN" at https://example.com/manga/the-cartographer-of-lost-harbours-collected-edition',
      );
    }
  }
  if (scenario === 'volumes' || scenario === 'volumes-running') {
    options.packAs = 'volumes';
    // three volumes and a half: the first 32 chapters (598-629) span volumes 61-64
    selected = new Set(
      chapters
        .slice(0, Math.min(32, chapters.length))
        .filter((c) => !c.locked)
        .map((c) => c.index),
    );
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'volumes-running') {
    const picked = chapters.filter((c) => selected.has(c.index));
    pushLog('info', `Downloading ${picked.length} chapter(s) of ${title}`);
    beginRun(picked);
    for (let i = 0; i < 12 && step(); i++) {
      // advancing
    }
    if (!freeze) {
      timer = setInterval(tick, TICK_MS);
    }
  }
  if (scenario === 'running' || scenario === 'cancelling' || scenario === 'bundle' || scenario === 'done') {
    options.packAs = scenario === 'bundle' ? 'bundle' : 'chapters';
    const picked = chapters.filter((c) => selected.has(c.index)).slice(0, 12);
    pushLog('info', `Downloading ${picked.length} chapter(s) of ${title}`);
    beginRun(picked);
    // run the simulation forward to an interesting point
    const steps = scenario === 'done' ? Infinity : scenario === 'bundle' ? 8 : 14;
    for (let i = 0; i < steps && step(); i++) {
      // advancing
    }
    if (scenario === 'cancelling') {
      cancelling = true;
      pushLog('info', 'Cancelling: chapters already downloading will finish, no new ones start');
    }
    if (scenario === 'done') {
      // a conversion warning on a chapter of the run (the second, 599 by default)
      const warned = picked[1] ?? picked[0];
      if (warned !== undefined) {
        pushLog('warn', `Chapter ${warned.number}: page 7 is AVIF but could not be converted; kept as is`);
      }
      finishRun();
    } else if (!freeze) {
      timer = setInterval(tick, TICK_MS);
    }
  }
  push();

  // a load of next's URL: the body of the load action, shared with setOptionsAndLoad
  async function startLoad(next: string): Promise<void> {
    const trimmed = next.trim();
    if (trimmed === '') {
      throw 'enter the URL of a series first';
    }
    if (loading || running) {
      throw 'a series is loading or a download is running';
    }
    if (urlKey(trimmed) !== urlKey(url)) {
      // another series: the loaded one goes now, as the backend does, so
      // the window shows the new one loading
      loaded = false;
      title = '';
      chapters = [];
      selected = new Set<number>();
      chaptersBus.emit([]);
      selectionBus.emit([]);
    }
    url = trimmed;
    loadError = '';
    loading = true;
    browserAuto = trimmed.includes('check');
    browserAlert = browserAuto ? MOCK_ALERT : '';
    push();
    // a link with "slow" in it takes 3 s, to see what the window does meanwhile
    await new Promise((resolve) => setTimeout(resolve, browserAuto || trimmed.includes('slow') ? 3000 : 400));
    browserAlert = '';
    if (trimmed.includes('unknown')) {
      failLoad();
      return;
    }
    finishLoad();
  }

  const api: Api = {
    async state() {
      return snapshot();
    },
    async chapters() {
      return chapters.map((c) => ({ ...c }));
    },
    async selection() {
      return selectionOf();
    },
    async log() {
      return log.slice();
    },

    async load(next) {
      await startLoad(next);
    },
    async setOptionsAndLoad(next, target) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      Object.assign(options, next);
      await startLoad(target);
    },
    async clear() {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      url = '';
      loaded = false;
      loadError = '';
      identified = false;
      browserAuto = false;
      browserAlert = '';
      applied = { language: '', scanlator: '' };
      title = '';
      chapters = [];
      selected = new Set<number>();
      volumesText = '';
      volumesSite = '';
      jobs = [];
      summary = null;
      chaptersBus.emit([]);
      selectionBus.emit([]);
      push();
    },
    async setSelected(index, on) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      if (index < 0 || index >= chapters.length) {
        return;
      }
      if (on && !chapters[index].locked) {
        selected.add(index);
      } else if (!on) {
        selected.delete(index);
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setSelectedMany(indices, on) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      if (indices.some((index) => index < 0 || index >= chapters.length)) {
        throw `chapter ${indices.find((index) => index < 0 || index >= chapters.length)} is not in the list`;
      }
      for (const index of indices) {
        if (on) {
          if (!chapters[index].locked) {
            selected.add(index);
          }
        } else {
          selected.delete(index);
        }
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setAll(on) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      selected = new Set(on ? chapters.filter((c) => !c.locked).map((c) => c.index) : []);
      selectionBus.emit(selectionOf());
      push();
    },
    async selectRange(text) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      const matched: number[] = [];
      for (const part of text
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean)) {
        const [a, b] = part.split('-').map(Number);
        if (Number.isNaN(a)) {
          throw `invalid range: ${part}`;
        }
        const hi = b ?? a;
        for (const c of chapters) {
          if (c.number >= a && c.number <= hi && !c.locked) {
            matched.push(c.index);
          }
        }
      }
      for (const i of matched) {
        selected.add(i);
      }
      selectionBus.emit(selectionOf());
      push();
      return matched.length;
    },
    async setOptions(next) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      Object.assign(options, next);
      push();
    },
    async setVolumes(text) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      try {
        parseVolumes(text);
      } catch (e) {
        throw (e as Error).message;
      }
      volumesText = text.trim();
      push();
    },
    async chooseFolder() {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      const picked = window.prompt('Output folder (mock dialog)', options.outputDir) ?? '';
      if (picked !== '') {
        options.outputDir = picked;
        push();
      }
      return picked;
    },
    async openFolder() {
      pushLog('info', `would open ${options.outputDir} (mock)`);
    },
    async revealSaved(url) {
      pushLog('info', `would show the latest saved chapter of ${url} in ${options.outputDir} (mock)`);
    },
    openURL(url) {
      window.open(url, '_blank', 'noopener');
    },
    async readClipboard() {
      return 'https://example.com/manga/one-piece';
    },
    async start() {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      if (!loaded) {
        throw 'load a series first';
      }
      if (options.language !== applied.language || options.scanlator !== applied.scanlator) {
        throw 'the language or scanlator changed: reload the series first';
      }
      const picked = chapters.filter((c) => selected.has(c.index));
      if (picked.length === 0) {
        throw 'select at least one chapter';
      }
      if (
        options.packAs === 'volumes' &&
        groupChapters(picked, parseVolumes(volumesText)).volumes.length === 0 &&
        volumesSite === '' &&
        volumesText === ''
      ) {
        throw `the site doesn't say which volume these chapters are in; give the volumes as chapter ranges instead, e.g. "1-7/8-16"`;
      }
      pushLog('info', `Downloading ${picked.length} chapter(s) of ${title}`);
      simulateRun(picked);
    },
    async update() {
      pushLog('info', 'Downloading the update ' + UPDATE_VERSION_DEV + '... (mock)');
      await new Promise((resolve) => setTimeout(resolve, 1500));
      pushLog('error', `couldn't download the update: the mock has no installer`);
      throw new Error('the mock has no installer');
    },
    async showBrowser() {
      pushLog('info', 'would bring the browser window to the front (mock)');
    },
    async dismissBrowserAlert() {
      browserAlert = '';
      push();
    },
    async cancel() {
      if (!running || cancelling) {
        return;
      }
      cancelling = true;
      pushLog('info', 'Cancelling: chapters already downloading will finish, no new ones start');
      push();
    },

    async countChapters(target) {
      await new Promise((resolve) => setTimeout(resolve, freeze ? 0 : 700));
      if (target.includes('kingofshojo')) {
        throw 'the site needs the browser window';
      }
      let h = 0;
      for (const ch of target) {
        h = (h * 31 + ch.charCodeAt(0)) % 997;
      }
      return 20 + (h % 300);
    },
    async search(query, site) {
      const words = query.toLowerCase().split(/\s+/).filter((w) => w !== '');
      if (words.length === 0) {
        return [];
      }
      const hits: Hit[] = [];
      for (const s of catalogSites) {
        if (site !== '' && s.site !== site) {
          continue;
        }
        for (const title of s.entries) {
          const folded = title.toLowerCase();
          if (words.every((w) => folded.includes(w))) {
            const hitURL = `https://${s.site}/manga/${slug(title)}/`;
            const entry = followedOf(hitURL);
            hits.push({
              site: s.site,
              title,
              url: hitURL,
              language: s.language,
              downloadable: s.downloadable,
              nsfw: s.nsfw,
              chapters: entry?.chapters ?? seen.get(urlKey(hitURL)) ?? mockLatest(s.site, title),
              approximate: entry === undefined && !seen.has(urlKey(hitURL)) && mockLatest(s.site, title) > 0,
              followed: entry !== undefined,
            });
          }
        }
      }
      hits.sort((a, b) => a.title.length - b.title.length || a.site.localeCompare(b.site));
      return hits.slice(0, 50);
    },
    async sites() {
      const have = new Map(catalogSites.map((s) => [s.site, s]));
      return MOCK_SITES.map((s): SiteInfo => {
        const c = have.get(s.site);
        return { ...s, hasCatalog: c !== undefined, entries: c?.entries.length ?? 0, nsfw: c?.nsfw ?? false };
      });
    },
    async catalogStatus() {
      return catalogInfo();
    },
    async copyText(text) {
      pushLog('info', `copied ${text} (mock)`);
    },
    async updateCatalog() {
      if (catalogUpdating) {
        throw new Error('the catalog is already updating');
      }
      catalogUpdating = true;
      push();
      setTimeout(() => {
        catalogUpdating = false;
        catalogSites = MOCK_CATALOG;
        catalogUpdatedAt = new Date().toISOString();
        catalogGeneration++;
        pushLog('info', `Catalog updated: ${catalogSites.length} sites`);
        push();
      }, 600);
    },

    async follow(f) {
      const target = f.url.trim();
      if (target === '') {
        throw 'a followed series needs a link';
      }
      const entry: Followed = {
        ...f,
        url: target,
        site: hostOfURL(target),
        followedAt: f.followedAt !== '' ? f.followedAt : new Date().toISOString(),
      };
      if (loaded && urlKey(url) === urlKey(target)) {
        entry.title = title;
        entry.chapters = chapters.length;
        entry.latest = Math.max(...chapters.map((c) => c.number));
        // what's listed now is known: only what comes later is new
        entry.known = [...new Set(chapters.map((c) => c.number))].sort((a, b) => a - b);
        entry.baselined = true;
        entry.new = 0;
        entry.checkedAt = new Date().toISOString();
        entry.downloadable = true;
      } else if (entry.chapters === 0) {
        entry.chapters = seen.get(urlKey(target)) ?? 0;
      }
      const at = followed.findIndex((g) => urlKey(g.url) === urlKey(target));
      if (at >= 0) {
        if (entry.savedUpTo === 0) {
          entry.savedUpTo = followed[at].savedUpTo;
          entry.saved = followed[at].saved;
        }
        if (!entry.baselined) {
          entry.known = followed[at].known;
          entry.baselined = followed[at].baselined;
          entry.new = followed[at].new;
        }
        followed[at] = entry;
      } else {
        followed = [...followed, entry];
      }
      pushLog('info', `Following ${entry.title || target}`);
      push();
      // the cover arrives a moment later, as the app's background fetch does
      if (entry.cover === '' && !freeze) {
        setTimeout(() => {
          if (followed.includes(entry)) {
            entry.cover = mockCover('#7c2d12', '#fdba74');
            push();
          }
        }, 900);
      }
    },
    async unfollow(target) {
      const entry = followedOf(target);
      followed = followed.filter((f) => f !== entry);
      if (entry !== undefined) {
        pushLog('info', `Unfollowed ${entry.title}`);
      }
      push();
    },
    async checkLibrary() {
      if (checking) {
        throw 'a check is already running';
      }
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      const todo = followed.filter((f) => f.downloadable);
      if (todo.length === 0) {
        throw 'nothing to check: no followed series the downloader can read';
      }
      checking = true;
      checkDone = 0;
      checkTotal = todo.length;
      pushLog('info', `Checking ${todo.length} followed series for new chapters`);
      push();
      for (const f of todo) {
        if (!checking) {
          break;
        }
        await new Promise((resolve) => setTimeout(resolve, freeze ? 0 : 600));
        f.checkedAt = new Date().toISOString();
        if (urlKey(f.url) === urlKey('https://example.com/manga/one-piece')) {
          f.chapters = CHAPTERS;
          f.latest = 1194;
          f.new = OP_CHAPTERS.filter(([number]) => f.baselined && !f.known.includes(number) && !f.saved.includes(number)).length;
        }
        f.error = '';
        checkDone++;
        push();
      }
      checking = false;
      pushLog('info', 'Check done');
      push();
    },
    async cancelCheck() {
      checking = false;
      push();
    },
    async downloadNew(target) {
      const f = followedOf(target);
      if (f === undefined) {
        throw "that series isn't followed";
      }
      if (!f.downloadable) {
        throw "the downloader can't read this site yet";
      }
      await startLoad(f.url);
      if (!loaded) {
        return;
      }
      const freshIdx = new Set(newIndices(chapters, f));
      const fresh = chapters.filter((c) => freshIdx.has(c.index));
      if (fresh.length === 0) {
        pushLog('info', `${f.title}: nothing new to download`);
        return;
      }
      selected = new Set(fresh.map((c) => c.index));
      selectionBus.emit(selectionOf());
      pushLog('info', `Downloading ${fresh.length} chapter(s) of ${title}`);
      simulateRun(fresh);
    },
    async downloadAllNew() {
      if (loading || running || checking) {
        throw 'a series is loading or a download is running';
      }
      if (batch.active) {
        throw 'a download of every new chapter is already running';
      }
      const todo = followed.filter((f) => f.downloadable && f.new > 0);
      if (todo.length === 0) {
        throw 'nothing new in the library';
      }
      batch = { active: true, done: 0, total: todo.length, current: '', currentURL: '' };
      batchStop = false;
      push();
      void (async () => {
        for (const [i, f] of todo.entries()) {
          if (batchStop) {
            break;
          }
          batch = { ...batch, current: f.title, currentURL: f.url, done: i };
          push();
          await api.downloadNew(f.url);
          while (running) {
            await new Promise((resolve) => setTimeout(resolve, 50));
          }
        }
        batch = { active: false, done: 0, total: 0, current: '', currentURL: '' };
        push();
      })();
    },
    async cancelBatch() {
      batchStop = true;
      await api.cancel();
    },

    onState: (cb) => stateBus.subscribe(cb),
    onChapters: (cb) => chaptersBus.subscribe(cb),
    onSelection: (cb) => selectionBus.subscribe(cb),
    onLog: (cb) => logBus.subscribe(cb),
  };

  return api;
}

export { scenarioFrom };

/** The host of a URL without "www.", "" when it doesn't parse */
function hostOfURL(raw: string): string {
  try {
    return new URL(raw).hostname.toLowerCase().replace(/^www\./, '');
  } catch {
    return '';
  }
}

/** A URL slug of a title, the way most sites make one */
function slug(title: string): string {
  return title
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');
}

/**
 * The latest chapter number the nightly catalog lists for a series, the way
 * the Madara crawler fills it: mangaread.org has one for every title (from
 * the title's letters, steady between searches), the other sites none
 */
function mockLatest(site: string, title: string): number {
  if (site !== 'mangaread.org') {
    return 0;
  }
  let n = 0;
  for (const ch of title) {
    n = (n * 31 + ch.codePointAt(0)!) % 1000;
  }

  return 120 + n;
}

/** The sites of the mock's picker: five with a catalog (two of them search only), two without */
const MOCK_SITES: Omit<SiteInfo, 'hasCatalog' | 'entries' | 'nsfw'>[] = [
  { site: 'arenascan.com', label: '', language: 'en', crawlable: true, downloadable: true },
  { site: 'kingofshojo.com', label: '', language: 'id', crawlable: true, downloadable: true },
  { site: 'mangadex.org', label: 'MangaDex', language: 'en', crawlable: false, downloadable: true },
  { site: 'mangaread.org', label: '', language: 'en', crawlable: true, downloadable: true },
  { site: 'weebcentral.com', label: '', language: 'en', crawlable: false, downloadable: true },
  { site: 'manhwaclan.example', label: 'Manhwa Clan', language: 'en', crawlable: true, downloadable: false },
  { site: 'toonily.example', label: 'Toonily', language: 'en', crawlable: true, downloadable: false },
];

/** The mock catalog: titles per site */
const MOCK_CATALOG: { site: string; language: string; downloadable: boolean; nsfw: boolean; entries: string[] }[] = [
  {
    site: 'manhwaclan.example',
    language: 'en',
    downloadable: false,
    nsfw: false,
    entries: ['One Piece', 'Solo Leveling', 'The Breaker', 'Lookism', 'Eleceed', 'Viral Hit', 'Wind Breaker', 'Noblesse', 'Tower of God', 'Hardcore Leveling Warrior'],
  },
  {
    site: 'toonily.example',
    language: 'en',
    downloadable: false,
    nsfw: true,
    entries: ['Secret Class', 'Queen Bee', 'Stepmother Friends', 'My Landlady Noona', 'Keep It a Secret from Your Mother', 'Boarding Diary', 'Fitness', 'Touch to Unlock', 'Sports Girl', 'Excuse Me, This Is My Room'],
  },
  {
    site: 'mangaread.org',
    language: 'en',
    downloadable: true,
    nsfw: false,
    entries: ['One Piece', 'One Piece: Ace\u2019s Story', 'Bleach', 'Naruto', 'Chainsaw Man', 'Jujutsu Kaisen', 'Spy x Family', 'Dandadan', 'Frieren: Beyond Journey\u2019s End', 'Blue Lock'],
  },
  {
    site: 'kingofshojo.com',
    language: 'id',
    downloadable: true,
    nsfw: false,
    entries: ['One Piece', 'Kimi ni Todoke', 'Ao Haru Ride', 'Fruits Basket', 'Ouran High School Host Club', 'Skip Beat!', 'Horimiya', 'Kaguya-sama: Love Is War', 'Wotakoi', 'My Love Story!!'],
  },
  {
    site: 'arenascan.com',
    language: 'en',
    downloadable: true,
    nsfw: false,
    entries: ['Solo Leveling', 'The Beginning After the End', 'Omniscient Reader\u2019s Viewpoint', 'Tower of God', 'Return of the Mount Hua Sect', 'Nano Machine', 'One Punch Man', 'Overgeared', 'Second Life Ranker', 'The Greatest Estate Developer'],
  },
];
