import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ResultsPanel, type ResultsPanelProps } from './ResultsPanel';
import type { CatalogInfo, Hit, SiteInfo } from './types';

const catalog: CatalogInfo = { sites: 3, entries: 30, searchOnly: 1, updatedAt: '2026-10-15T09:00:00Z', updating: false, error: '', generation: 1 };
const sites: SiteInfo[] = [
  { site: 'mangaread.org', label: '', language: 'en', hasCatalog: true, crawlable: true, entries: 10, downloadable: true, nsfw: false },
  { site: 'only.test', label: 'Only Source', language: 'en', hasCatalog: true, crawlable: true, entries: 20, downloadable: false, nsfw: true },
  { site: 'mangadex.org', label: 'MangaDex', language: 'en', hasCatalog: false, crawlable: false, entries: 0, downloadable: true, nsfw: false },
];
const hits: Hit[] = [
  { site: 'mangaread.org', title: 'One Piece', url: 'https://www.mangaread.org/manga/one-piece/', language: 'en', downloadable: true, nsfw: false, chapters: 1120, approximate: false, followed: true },
  { site: 'mangaread.org', title: 'One Piece: Ace', url: 'https://www.mangaread.org/manga/one-piece-ace/', language: 'en', downloadable: true, nsfw: false, chapters: 0, approximate: false, followed: false },
  { site: 'only.test', title: 'One Piece (scan)', url: 'https://only.test/series/one-piece', language: 'en', downloadable: false, nsfw: true, chapters: 0, approximate: false, followed: false },
  { site: 'mangaread.org', title: 'One Piece: Omake', url: 'https://www.mangaread.org/manga/one-piece-omake/', language: 'en', downloadable: true, nsfw: false, chapters: 1120, approximate: true, followed: false },
];

function render(over: Partial<ResultsPanelProps>): string {
  const props: ResultsPanelProps = {
    catalog,
    sites,
    query: 'one piece',
    site: '',
    hits,
    searching: false,
    busy: false,
    blocked: null,
    copied: false,
    onSite: () => undefined,
    onPick: () => undefined,
    onFollow: () => undefined,
    onUnfollow: () => undefined,
    onUpdate: () => undefined,
    onCopy: () => undefined,
    onOpen: () => undefined,
    onDismissBlocked: () => undefined,
    ...over,
  };

  return renderToStaticMarkup(createElement(ResultsPanel, props));
}

describe('ResultsPanel', () => {
  it('lists the hits with their site, chapters and follow mark', () => {
    const markup = render({});
    expect(markup).toContain('4 results for “one piece”');
    expect(markup.match(/class="hit-site"/g)?.length).toBe(4);
    expect(markup).toContain('data-url="https://www.mangaread.org/manga/one-piece/"');
    expect(markup).toContain('>1,120<');
    // with no count: one readable (open it to see), one search only (can't be counted)
    expect(markup.match(/title="Open it to see how many chapters it has"/g)?.length).toBe(1);
    expect(markup.match(/title="Search only: /g)?.length).toBe(1);
    // a series never opened shows the catalog's latest chapter as about that many
    expect(markup.match(/class="hit-chapters num approx" title="latest chapter the site listed"/g)?.length).toBe(1);
    expect(markup).toContain('≈ </span>');
    expect(markup).toContain('aria-label="Following One Piece, click to unfollow"');
    expect(markup).toContain('aria-label="Follow One Piece: Ace"');
    expect(markup.match(/class="star on"/g)?.length).toBe(1);
  });
  it('offers the site picker, closed, naming the site picked', () => {
    const markup = render({});
    expect(markup).toContain('aria-haspopup="listbox"');
    expect(markup).toContain('aria-expanded="false"');
    expect(markup).toContain('>All sites</span>');
    expect(markup).not.toContain('role="listbox"');
  });
  it('shows the counts the window fetched, a spinner while it does, and why a dash', () => {
    const ace = 'https://www.mangaread.org/manga/one-piece-ace/';
    expect(render({ counts: { [ace]: 42 } })).toContain('<span class="hit-chapters num">42</span>');
    expect(render({ counts: { [ace]: 'counting' } })).toContain('<span class="sr-only">counting</span>');
    expect(render({ counts: { [ace]: 'failed' } })).toContain("Couldn&#x27;t count its chapters");
    const markup = render({});
    expect(markup).toContain('Search only: the app can&#x27;t read this site, so it can&#x27;t count its chapters');
    expect(markup).toContain('Open it to see how many chapters it has');
  });
  it('tags a hit from a search-only site, and an adult one', () => {
    const markup = render({});
    expect(markup.match(/class="hit-tag search-only"/g)?.length).toBe(1);
    expect(markup.match(/class="hit-tag nsfw"/g)?.length).toBe(1);
  });
  it('explains a picked search-only hit, until the search changes', () => {
    const picked = { hit: hits[2], query: 'one piece', site: '' };
    expect(render({ blocked: picked })).toContain('supported for download yet');
    expect(render({ blocked: picked, copied: true })).toContain('>Copied<');
    expect(render({ query: 'one pie', blocked: picked })).not.toContain('supported for download yet');
    expect(render({ site: 'mangaread.org', blocked: picked })).not.toContain('supported for download yet');
  });
  it('keeps following possible while a download runs, but not picking', () => {
    const markup = render({ busy: true });
    expect(markup.match(/class="hit-pick"[^>]*disabled/g)?.length).toBe(4);
    expect(markup).not.toMatch(/class="star[^"]*"[^>]*disabled/);
  });
  it('says when nothing matched', () => {
    expect(render({ hits: [] })).toContain('No series called “one piece”');
  });
  it('offers the catalog download when there is none', () => {
    const markup = render({ catalog: { ...catalog, sites: 0, entries: 0 }, hits: [] });
    expect(markup).toContain('No catalog yet');
    expect(markup).toContain('Download the catalogs');
  });
});
