import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import { SearchPanel, type SearchPanelProps } from './SearchPanel';
import type { CatalogInfo, Hit, SiteInfo } from './types';

const catalog: CatalogInfo = { sites: 3, entries: 30, updatedAt: '2026-10-15T09:00:00Z', updating: false, error: '', generation: 1 };
const sites: SiteInfo[] = [
  { site: 'mangaread.org', label: '', language: 'en', hasCatalog: true, crawlable: true, entries: 10 },
  { site: 'mangadex.org', label: 'MangaDex', language: 'en', hasCatalog: false, crawlable: false, entries: 0 },
];
const hits: Hit[] = [
  { site: 'mangaread.org', title: 'One Piece', url: 'https://www.mangaread.org/manga/one-piece/', language: 'en' },
  { site: 'mangaread.org', title: 'One Piece: Ace', url: 'https://www.mangaread.org/manga/one-piece-ace/', language: 'en' },
];

function render(over: Partial<SearchPanelProps>): string {
  const props: SearchPanelProps = {
    catalog,
    sites,
    query: '',
    site: '',
    hits: [],
    searching: false,
    busy: false,
    onQuery: () => undefined,
    onSite: () => undefined,
    onPick: () => undefined,
    onUpdate: () => undefined,
    ...over,
  };

  return renderToStaticMarkup(createElement(SearchPanel, props));
}

describe('SearchPanel', () => {
  it('asks for a query when there is none', () => {
    const markup = render({});
    expect(markup).toContain('Type to search');
    expect(markup).toContain('All sites');
    expect(markup).toContain('mangaread.org');
    expect(markup).toMatch(/<option[^>]*disabled[^>]*>MangaDex/);
  });
  it('lists the hits with their site, as buttons that load them', () => {
    const markup = render({ query: 'one piece', hits });
    expect(markup).toContain('One Piece: Ace');
    expect(markup.match(/class="hit-site"/g)?.length).toBe(2);
    expect(markup).toContain('data-url="https://www.mangaread.org/manga/one-piece/"');
    expect(markup).toContain('2 results');
  });
  it('says when nothing matched', () => {
    expect(render({ query: 'zzz', hits: [] })).toContain('No results for “zzz”');
  });
  it('offers the download when there is no catalog yet', () => {
    const markup = render({ catalog: { ...catalog, sites: 0, entries: 0, updatedAt: '' }, sites: [] });
    expect(markup).toContain('No catalog yet');
    expect(markup).toContain('Update now');
    expect(markup).not.toContain('Type to search');
  });
  it('disables the field while a download runs', () => {
    expect(render({ busy: true })).toMatch(/<input[^>]*readonly[^>]*\/>/i);
  });
});

describe('mock search', () => {
  it('finds titles in the mock catalog and knows its sites', async () => {
    const api = createMockApi('empty');
    const found = await api.search('one piece', '');
    expect(found.length).toBeGreaterThan(0);
    expect(found[0].title.toLowerCase()).toContain('one piece');
    const only = await api.search('one piece', found[0].site);
    expect(only.every((h) => h.site === found[0].site)).toBe(true);
    expect((await api.sites()).some((s) => s.hasCatalog)).toBe(true);
    expect((await api.state()).catalog.sites).toBeGreaterThan(0);
  });
  it('starts without a catalog with &nocatalog', async () => {
    const api = createMockApi({ scenario: 'empty', nocatalog: true });
    expect((await api.state()).catalog.sites).toBe(0);
    expect(await api.search('one', '')).toEqual([]);
  });
});
