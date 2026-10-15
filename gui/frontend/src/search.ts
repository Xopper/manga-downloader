// The Search mode's pure parts: the rows the panel lists, the site picker's
// options, the catalog line, and the pause before a typed query searches.

import type { CatalogInfo, Hit, SiteInfo } from './types';

/** How long typing pauses before the query runs */
export const SEARCH_DELAY_MS = 250;

export interface ResultRow {
  key: string;
  title: string;
  site: string;
  language: string;
  url: string;
  downloadable: boolean;
  nsfw: boolean;
}

/** The rows of the hits, in the backend's order (best first) */
export function resultRows(hits: Hit[]): ResultRow[] {
  return hits.map((h) => ({
    key: `${h.site} ${h.url}`,
    title: h.title,
    site: h.site,
    language: h.language,
    url: h.url,
    downloadable: h.downloadable,
    nsfw: h.nsfw,
  }));
}

export interface SiteOption {
  value: string;
  label: string;
}

/** A group of the site picker: "" is the ungrouped head (All sites) */
export interface SiteGroup {
  label: string;
  options: SiteOption[];
}

/** What a site is called in the picker: its label and domain, or the domain */
function siteName(s: SiteInfo): string {
  return s.label !== '' ? `${s.label} (${s.site})` : s.site;
}

/**
 * The picker's groups: every site first, then the searchable sites the
 * downloader can download from, then the search-only ones, each by name. A
 * site without a catalog isn't offered (the status command lists them).
 */
export function siteGroups(sites: SiteInfo[]): SiteGroup[] {
  const byName = (a: SiteInfo, b: SiteInfo) => siteName(a).localeCompare(siteName(b));
  const option = (s: SiteInfo): SiteOption => ({ value: s.site, label: siteName(s) });
  const searchable = sites.filter((s) => s.hasCatalog);
  const groups: SiteGroup[] = [
    { label: '', options: [{ value: '', label: 'All sites' }] },
    { label: 'Can download', options: searchable.filter((s) => s.downloadable).sort(byName).map(option) },
    { label: 'Search only', options: searchable.filter((s) => !s.downloadable).sort(byName).map(option) },
  ];

  return groups.filter((g) => g.options.length > 0);
}

/** "3 h ago", "12 min ago", "just now", "4 days ago" */
export function ago(iso: string, now: Date): string {
  const ms = now.getTime() - new Date(iso).getTime();
  if (!Number.isFinite(ms) || ms < 60_000) {
    return 'just now';
  }
  const min = Math.floor(ms / 60_000);
  if (min < 60) {
    return `${min} min ago`;
  }
  const h = Math.floor(min / 60);
  if (h < 48) {
    return `${h} h ago`;
  }

  return `${Math.floor(h / 24)} days ago`;
}

/** The catalog line of the panel's head */
export function catalogSummary(info: CatalogInfo, now: Date): string {
  if (info.sites === 0) {
    return info.updating ? 'Downloading the catalog…' : 'No catalog yet';
  }
  let counts = `${info.sites} ${info.sites === 1 ? 'site' : 'sites'}, ${info.entries.toLocaleString('en-US')} titles`;
  if (info.searchOnly > 0) {
    counts += ` (${info.searchOnly} search only)`;
  }
  if (info.updating) {
    return `${counts} · updating…`;
  }
  if (info.error !== '') {
    return `${counts} · update failed: ${info.error}`;
  }
  if (info.updatedAt === '') {
    return counts;
  }

  return `${counts} · updated ${ago(info.updatedAt, now)}`;
}

/** Calls fn once the calls pause for ms, with the last arguments */
export function debounce<A extends unknown[]>(fn: (...args: A) => void, ms: number): { call: (...args: A) => void; cancel: () => void } {
  let timer: ReturnType<typeof setTimeout> | undefined;

  return {
    call(...args: A) {
      clearTimeout(timer);
      timer = setTimeout(() => fn(...args), ms);
    },
    cancel() {
      clearTimeout(timer);
    },
  };
}
