import { describe, expect, it, vi } from 'vitest';
import { catalogSummary, debounce, resultRows, siteGroups } from './search';
import type { CatalogInfo, Hit, SiteInfo } from './types';

const hits: Hit[] = [
  { site: 'mangaread.org', title: 'One Piece', url: 'https://www.mangaread.org/manga/one-piece/', language: 'en', downloadable: true, nsfw: false },
  { site: 'kingofshojo.com', title: 'One Piece', url: 'https://kingofshojo.com/manga/one-piece/', language: 'id', downloadable: true, nsfw: false },
];

describe('resultRows', () => {
  it('keeps the backend order and keys rows by site and URL', () => {
    const rows = resultRows(hits);
    expect(rows.map((r) => r.site)).toEqual(['mangaread.org', 'kingofshojo.com']);
    expect(rows[0].key).not.toBe(rows[1].key);
    expect(rows[0].url).toBe(hits[0].url);
  });
});

describe('siteGroups', () => {
  const sites: SiteInfo[] = [
    { site: 'zzz.test', label: '', language: 'en', hasCatalog: true, crawlable: true, entries: 3, downloadable: true, nsfw: false },
    { site: 'mangadex.org', label: 'MangaDex', language: 'en', hasCatalog: false, crawlable: false, entries: 0, downloadable: true, nsfw: false },
    { site: 'aaa.test', label: 'Aaa Scans', language: 'es', hasCatalog: true, crawlable: true, entries: 5, downloadable: true, nsfw: false },
    { site: 'only.test', label: 'Only Source', language: 'en', hasCatalog: true, crawlable: true, entries: 9, downloadable: false, nsfw: true },
    { site: 'kkk.test', label: '', language: 'en', hasCatalog: false, crawlable: true, entries: 0, downloadable: true, nsfw: false },
  ];
  it('offers all sites, then the downloadable ones, then the search-only ones, and no site without a catalog', () => {
    const groups = siteGroups(sites);
    expect(groups[0]).toEqual({ label: '', options: [{ value: '', label: 'All sites' }] });
    expect(groups[1].label).toBe('Can download');
    expect(groups[1].options.map((o) => o.value)).toEqual(['aaa.test', 'zzz.test']);
    expect(groups[1].options[0].label).toBe('Aaa Scans (aaa.test)');
    expect(groups[2].label).toBe('Search only');
    expect(groups[2].options.map((o) => o.value)).toEqual(['only.test']);
    expect(groups.flatMap((g) => g.options).some((o) => o.value === 'mangadex.org' || o.value === 'kkk.test')).toBe(false);
  });
  it('leaves an empty group out', () => {
    expect(siteGroups(sites.filter((s) => s.downloadable)).map((g) => g.label)).toEqual(['', 'Can download']);
  });
});

describe('catalogSummary', () => {
  const now = new Date('2026-10-15T12:00:00Z');
  const base: CatalogInfo = { sites: 12, entries: 48210, searchOnly: 0, updatedAt: '2026-10-15T09:00:00Z', updating: false, error: '', generation: 3 };
  it('counts sites and titles and says when it was updated', () => {
    expect(catalogSummary(base, now)).toBe('12 sites, 48,210 titles · updated 3 h ago');
  });
  it('counts the search-only sites', () => {
    expect(catalogSummary({ ...base, searchOnly: 3 }, now)).toBe('12 sites, 48,210 titles (3 search only) · updated 3 h ago');
  });
  it('says so while updating, and shows the error after a failed update', () => {
    expect(catalogSummary({ ...base, updating: true }, now)).toBe('12 sites, 48,210 titles · updating…');
    expect(catalogSummary({ ...base, error: 'catalog index: boom' }, now)).toBe('12 sites, 48,210 titles · update failed: catalog index: boom');
  });
  it('has no catalog', () => {
    expect(catalogSummary({ ...base, sites: 0, entries: 0, updatedAt: '' }, now)).toBe('No catalog yet');
    expect(catalogSummary({ ...base, sites: 0, entries: 0, updatedAt: '', updating: true }, now)).toBe('Downloading the catalog…');
  });
});

describe('debounce', () => {
  it('fires once after the pause, with the last arguments', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const d = debounce(fn, 200);
    d.call('a');
    d.call('ab');
    vi.advanceTimersByTime(150);
    expect(fn).not.toHaveBeenCalled();
    d.call('abc');
    vi.advanceTimersByTime(199);
    expect(fn).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenCalledWith('abc');
    d.call('x');
    d.cancel();
    vi.advanceTimersByTime(500);
    expect(fn).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });
});
