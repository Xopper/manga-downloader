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
    expect(markup).toContain('>Select<');
    expect(markup).not.toContain('no language choice');
    expect(markup).not.toContain('no scanlation groups');
    expect(markup).not.toContain('class="filter-row"');
  });
  it('shows the selectors beside the chips when the site lists values', async () => {
    const api = createMockApi({ scenario: 'loaded', freeze: true });
    const state = await api.state();
    const chapters = await api.chapters();
    const markup = render(state, chapters, followedOf(state.library, state.url));
    const chips = markup.indexOf('class="chip-row"');
    const filters = markup.indexOf('class="filter-row"');
    expect(filters).toBeGreaterThan(chips);
    expect(markup).toContain('<label for=');
    expect(markup).not.toContain('placeholder="any"');
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
