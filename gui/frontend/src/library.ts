// The library's pure parts: what a card says and offers, the home's header
// line, the sort, which chapters are new, and the URL key two spellings of
// one series share (as the backend's urlKey).

import { isSeriesUrl } from './autoLoad';
import { plural } from './format';
import { ago } from './search';
import type { Chapter, Followed, LibraryInfo, State } from './types';

/** The form two spellings of one series URL share: lower-case scheme and host, no "www.", no trailing slash */
export function urlKey(raw: string): string {
  const text = raw.trim();
  try {
    const u = new URL(text);
    const host = u.hostname.toLowerCase().replace(/^www\./, '');
    const port = u.port !== '' ? `:${u.port}` : '';
    return `${u.protocol.toLowerCase()}//${host}${port}${u.pathname.replace(/\/+$/, '')}${u.search}`;
  } catch {
    return text.toLowerCase().replace(/\/+$/, '');
  }
}

/** The followed entry of url, null when it isn't followed */
export function followedOf(library: Pick<LibraryInfo, 'followed'>, url: string): Followed | null {
  if (url.trim() === '') {
    return null;
  }
  const key = urlKey(url);
  return library.followed.find((f) => urlKey(f.url) === key) ?? null;
}

/** Whether the top box holds a link to load rather than a title to search */
export function isLinkQuery(text: string): boolean {
  const t = text.trim();
  return /^https?:\/\//i.test(t) || isSeriesUrl(t);
}

export type Tone = 'new' | 'ok' | 'muted' | 'warn' | 'danger';

/** The status line of a card */
export function cardStatus(f: Followed): { text: string; tone: Tone } {
  if (!f.downloadable) {
    return { text: "search only · can't download from this site yet", tone: 'warn' };
  }
  if (f.error !== '') {
    return { text: `Check failed: ${f.error}`, tone: 'danger' };
  }
  if (f.chapters === 0) {
    return { text: 'Not checked yet', tone: 'muted' };
  }
  const saved = ` · ${savedCount(f)}`;
  if (f.new > 0) {
    return { text: `${f.new} new${saved}`, tone: 'new' };
  }
  return { text: `Nothing new${saved}`, tone: 'ok' };
}

/** How many chapters are saved in the folder and format chosen now: "12 of 53 saved", "all 42 saved", "none saved" */
export function savedCount(f: Followed): string {
  const n = Math.min(f.saved.length, f.chapters);
  if (n === 0) {
    return 'none saved';
  }
  if (n === f.chapters) {
    return `all ${f.chapters.toLocaleString('en-US')} saved`;
  }
  return `${n.toLocaleString('en-US')} of ${f.chapters.toLocaleString('en-US')} saved`;
}

/** The one button of a card */
export function cardAction(f: Followed): { label: string; kind: 'new' | 'none' | 'open' } {
  if (!f.downloadable) {
    return { label: 'Open on the site', kind: 'open' };
  }
  if (f.new > 0) {
    return { label: `Download ${f.new} new`, kind: 'new' };
  }
  return { label: f.chapters === 0 ? 'Not checked yet' : 'Nothing new', kind: 'none' };
}

export type Sort = 'new' | 'title' | 'saved';

/** The cards in order: new chapters first (then title), by title, or by last download */
export function sortFollowed(list: Followed[], by: Sort): Followed[] {
  const byTitle = (a: Followed, b: Followed) => a.title.localeCompare(b.title);
  const out = [...list];
  switch (by) {
    case 'new':
      return out.sort((a, b) => b.new - a.new || byTitle(a, b));
    case 'title':
      return out.sort(byTitle);
    case 'saved':
      return out.sort((a, b) => b.lastSaved.localeCompare(a.lastSaved) || byTitle(a, b));
  }
}

/** How many new chapters the library has in all, over the series the downloader can read */
export function totalNew(library: Pick<LibraryInfo, 'followed'>): number {
  return library.followed.filter((f) => f.downloadable).reduce((n, f) => n + f.new, 0);
}

/** The home's header line: "5 series you follow · 3 new chapters · checked 5 min ago" */
export function librarySummary(library: Pick<LibraryInfo, 'followed' | 'checking' | 'checkDone' | 'checkTotal'>, now: Date): string {
  const n = library.followed.length;
  if (n === 0) {
    return 'Nothing followed yet';
  }
  const parts = [`${plural(n, 'series you follow', 'series you follow')}`];
  const fresh = totalNew(library);
  parts.push(fresh > 0 ? plural(fresh, 'new chapter', 'new chapters') : 'nothing new');
  if (library.checking) {
    parts.push(`checking ${library.checkDone + 1} of ${library.checkTotal}…`);
  } else {
    const checked = library.followed.map((f) => f.checkedAt).filter((t) => t !== '').sort();
    if (checked.length > 0) {
      parts.push(`checked ${ago(checked[checked.length - 1], now)}`);
    }
  }
  return parts.join(' · ');
}

/** Whether a chapter numbered number is new: listed since the series was followed, and not saved */
function isNew(number: number, f: Followed): boolean {
  return f.baselined && !f.known.includes(number) && !f.everSaved.includes(number);
}

/** The indices of a followed series' new chapters: listed since it was followed, not saved since */
export function newIndices(chapters: Chapter[], f: Followed | null): number[] {
  if (f === null || !f.downloadable || !f.baselined) {
    return [];
  }
  const known = new Set(f.known);
  const saved = new Set(f.everSaved);
  return chapters.filter((c) => !known.has(c.number) && !saved.has(c.number)).map((c) => c.index);
}

/**
 * The tag a chapter row shows for a followed series: saved on the chapters
 * the app saved, new on those listed since the series was followed and not
 * saved, none on the others; none for a series not followed
 */
export function chapterMark(number: number, f: Followed | null): 'new' | 'saved' | '' {
  if (f === null || !f.downloadable) {
    return '';
  }
  if (f.saved.includes(number)) {
    return 'saved';
  }
  return isNew(number, f) ? 'new' : '';
}

/** The followed entry of the loaded series */
export function loadedFollowed(state: Pick<State, 'url' | 'library'>): Followed | null {
  return followedOf(state.library, state.url);
}
