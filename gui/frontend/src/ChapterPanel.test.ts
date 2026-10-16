import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ChapterPanel } from './ChapterPanel';
import { followedOf } from './library';
import { createMockApi } from './mock';
import type { Chapter, Followed, State } from './types';

function render(state: State, chapters: Chapter[], followed: Followed | null): string {
  return renderToStaticMarkup(
    createElement(ChapterPanel, {
      state,
      chapters,
      selected: [],
      disabled: false,
      onToggle: () => Promise.resolve(),
      onToggleMany: () => Promise.resolve(),
      onSetAll: () => Promise.resolve(),
      onRange: () => Promise.resolve(0),
      onVolumes: () => Promise.resolve(),
      onReload: () => undefined,
      onError: () => undefined,
      followed,
      onPick: () => Promise.resolve(),
      onOptions: () => Promise.resolve(),
    }),
  );
}

describe('ChapterPanel', () => {
  it('leaves out a filter the site has no choice for', async () => {
    const api = createMockApi({ scenario: 'loaded', freeze: true, site: 'none' });
    const state = await api.state();
    const chapters = await api.chapters();
    const markup = render(state, chapters, followedOf(state.library, state.url));
    expect(markup).not.toContain('no language choice');
    expect(markup).not.toContain('no scanlation groups');
    expect(markup).not.toContain('class="filters-bar"');
    expect(markup).not.toContain('class="filter-row"');
  });
  it('shows the selectors above the list when the site lists values', async () => {
    const api = createMockApi({ scenario: 'loaded', freeze: true });
    const state = await api.state();
    const chapters = await api.chapters();
    const markup = render(state, chapters, followedOf(state.library, state.url));
    const filters = markup.indexOf('class="filters-bar"');
    const toolbar = markup.indexOf('class="list-toolbar"');
    expect(filters).toBeGreaterThan(-1);
    expect(filters).toBeLessThan(toolbar);
    expect(markup).toContain('<label for=');
    expect(markup).not.toContain('placeholder="any"');
  });
  it('selects with one set of chips in the toolbar', async () => {
    const api = createMockApi({ scenario: 'loaded', freeze: true });
    const state = await api.state();
    const chapters = await api.chapters();
    const markup = render(state, chapters, followedOf(state.library, state.url));
    expect(markup.match(/class="chip[^"]*"/g)?.length).toBe(3);
    expect(markup).toContain('New · 12');
    expect(markup).toContain('All · 597');
    expect(markup).toContain('>None</button>');
    expect(markup).not.toContain('btn ghost small');
    expect(markup).not.toContain('>Select<');
  });
  it('shows a plain "Chapter N" title once, as the number', async () => {
    const api = createMockApi({ scenario: 'loaded', freeze: true });
    const state = await api.state();
    const chapters: Chapter[] = [
      { index: 0, number: 3, title: 'Chapter 3', language: '', group: '', volume: 0 },
      { index: 1, number: 4, title: 'Chapter 4 - The fight', language: '', group: '', volume: 0 },
    ];
    const markup = render({ ...state, chapterCount: 2, selectedCount: 0, options: { ...state.options, packAs: 'chapters' } }, chapters, null);
    expect(markup).toContain('<span class="num">3</span><span class="title"></span>');
    expect(markup).toContain('<span class="num">4</span><span class="title">The fight</span>');
  });
});
