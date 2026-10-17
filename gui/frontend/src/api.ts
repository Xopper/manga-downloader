// The one module the UI talks to the backend through. createWailsApi wraps the
// bindings Wails generates into wailsjs/; createMockApi (mock.ts) stands in for
// them in a plain browser, so the UI can be worked on without the Go app.

import * as app from '../wailsjs/go/main/App';
import { BrowserOpenURL, ClipboardGetText, ClipboardSetText, EventsOn } from '../wailsjs/runtime/runtime';
import type { CatalogInfo, Chapter, Followed, Hit, LogLine, Options, SiteInfo, State } from './types';

export type Unsubscribe = () => void;

export interface Api {
  // Snapshots, for the first render. The events below keep them current.
  state(): Promise<State>;
  chapters(): Promise<Chapter[]>;
  selection(): Promise<number[]>;
  log(): Promise<LogLine[]>;

  // Actions. A rejected promise carries the reason as a string (or Error).
  load(url: string): Promise<void>;
  /** Forgets the loaded series (URL, chapters, selection, load error); refused while one loads or a download runs */
  clear(): Promise<void>;
  setSelected(index: number, on: boolean): Promise<void>;
  /** Selects or deselects every index at once; rejects, changing nothing, if one is out of range */
  setSelectedMany(indices: number[], on: boolean): Promise<void>;
  setAll(on: boolean): Promise<void>;
  /** Selects the chapters in the range text; resolves with how many matched */
  selectRange(text: string): Promise<number>;
  setOptions(options: Options): Promise<void>;
  /** setOptions and then load(url), in one step: the options a selector picked are never seen unapplied */
  setOptionsAndLoad(options: Options, url: string): Promise<void>;
  /** Sets the Volumes box ("1-7/8-16", "" for the site's own); rejects a value that doesn't parse */
  setVolumes(text: string): Promise<void>;
  /** Native folder picker; resolves with "" when cancelled */
  chooseFolder(): Promise<string>;
  openFolder(): Promise<void>;
  /** Shows a followed series' latest chapter saved in the chosen folder, selected in the file manager */
  revealSaved(url: string): Promise<void>;
  /** Opens the URL in the system's browser, never in the window */
  openURL(url: string): void;
  /** The clipboard's text, for the Paste button; rejects when the runtime isn't there */
  readClipboard(): Promise<string>;
  start(): Promise<void>;
  cancel(): Promise<void>;
  /** Downloads and runs the installer, then quits; rejects with the reason (also logged) when it can't */
  update(): Promise<void>;
  /** Brings the browser window to the front again */
  showBrowser(): Promise<void>;
  /** Hides browserAlert; the window and the load waiting for it go on */
  dismissBrowserAlert(): Promise<void>;

  // Search. The catalogs are on disk; the backend keeps them fresh.
  /** Finds series in the catalogs, best match first; site "" is every site */
  search(query: string, site: string): Promise<Hit[]>;
  /** How many chapters a result lists, fetched (and remembered) when the app hasn't seen it; refused for a site that needs the browser window */
  countChapters(url: string): Promise<number>;
  /** Every site of the table, for the picker */
  sites(): Promise<SiteInfo[]>;
  catalogStatus(): Promise<CatalogInfo>;
  /** Downloads the catalogs that changed, in the background; state.catalog follows. Rejects while one update runs */
  updateCatalog(): Promise<void>;
  /** Puts text on the system clipboard (through the runtime: the webview's own clipboard API needs a secure context) */
  copyText(text: string): Promise<void>;

  // The library. state.library follows every change.
  /** Follows a series (or updates its entry); the loaded series fills in its own count */
  follow(f: Followed): Promise<void>;
  unfollow(url: string): Promise<void>;
  /** Fetches every followed series' chapter list in the background; refused while busy or already checking */
  checkLibrary(): Promise<void>;
  cancelCheck(): Promise<void>;
  /** Loads a followed series, selects the chapters above its saved mark and starts; refused while busy */
  downloadNew(url: string): Promise<void>;
  /** The same for every followed series with new chapters, one after the other; progress in state.library.batch */
  downloadAllNew(): Promise<void>;
  cancelBatch(): Promise<void>;

  // Pushes. Each returns a function that stops the callback.
  onState(cb: (s: State) => void): Unsubscribe;
  onChapters(cb: (c: Chapter[]) => void): Unsubscribe;
  onSelection(cb: (indices: number[]) => void): Unsubscribe;
  onLog(cb: (lines: LogLine[]) => void): Unsubscribe;
}

/** Events are registered once each; callbacks are fanned out from a set */
function fanOut<T>(event: string): { subscribe(cb: (v: T) => void): Unsubscribe } {
  const callbacks = new Set<(v: T) => void>();
  let registered = false;

  return {
    subscribe(cb) {
      callbacks.add(cb);
      if (!registered) {
        registered = true;
        EventsOn(event, (v: T) => {
          for (const fn of callbacks) {
            fn(v);
          }
        });
      }

      return () => {
        callbacks.delete(cb);
      };
    },
  };
}

export function createWailsApi(): Api {
  const state = fanOut<State>('state');
  const chapters = fanOut<Chapter[]>('chapters');
  const selection = fanOut<number[]>('selection');
  const log = fanOut<LogLine[]>('log');

  return {
    // The bindings' types are structurally the same as ours; the cast only
    // narrows the string-typed enums the Go side fills in.
    state: () => app.State() as unknown as Promise<State>,
    chapters: () => app.Chapters() as unknown as Promise<Chapter[]>,
    selection: () => app.Selection(),
    log: () => app.Log() as unknown as Promise<LogLine[]>,

    load: (url) => app.Load(url),
    clear: () => app.Clear(),
    setSelected: (index, on) => app.SetSelected(index, on),
    setSelectedMany: (indices, on) => app.SetSelectedMany(indices, on),
    setAll: (on) => app.SetAll(on),
    selectRange: (text) => app.SelectRange(text),
    setOptions: (options) => app.SetOptions(options as unknown as Parameters<typeof app.SetOptions>[0]),
    setOptionsAndLoad: (options, url) =>
      app.SetOptionsAndLoad(options as unknown as Parameters<typeof app.SetOptionsAndLoad>[0], url),
    setVolumes: (text) => app.SetVolumes(text),
    chooseFolder: () => app.ChooseFolder(),
    openFolder: () => app.OpenFolder(),
    revealSaved: (url) => app.RevealSaved(url),
    openURL: (url) => BrowserOpenURL(url),
    // async, so a missing runtime rejects rather than throwing into the click
    readClipboard: async () => ClipboardGetText(),
    start: () => app.Start(),
    cancel: () => app.Cancel(),
    update: () => app.Update(),
    showBrowser: () => app.ShowBrowser(),
    dismissBrowserAlert: () => app.DismissBrowserAlert(),
    search: (query, site) => app.Search(query, site),
    countChapters: (url) => app.CountChapters(url),
    sites: () => app.Sites(),
    catalogStatus: () => app.CatalogStatus(),
    updateCatalog: () => app.UpdateCatalog(),
    copyText: async (text) => {
      if (!(await ClipboardSetText(text))) {
        throw new Error('could not copy to the clipboard');
      }
    },
    follow: (f) => app.Follow(f as unknown as Parameters<typeof app.Follow>[0]),
    unfollow: (url) => app.Unfollow(url),
    checkLibrary: () => app.CheckLibrary(),
    cancelCheck: () => app.CancelCheck(),
    downloadNew: (url) => app.DownloadNew(url),
    downloadAllNew: () => app.DownloadAllNew(),
    cancelBatch: () => app.CancelBatch(),

    onState: (cb) => state.subscribe(cb),
    onChapters: (cb) => chapters.subscribe(cb),
    onSelection: (cb) => selection.subscribe(cb),
    onLog: (cb) => log.subscribe(cb),
  };
}
