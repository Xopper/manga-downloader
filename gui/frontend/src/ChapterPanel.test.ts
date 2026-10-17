import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ChapterBox, ChapterPanel, ChapterTitle, LOCKED_HINT } from './ChapterPanel';
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
  it('says the list is still loading over a saved list, and only then', async () => {
    const api = createMockApi({ scenario: 'reloading', freeze: true });
    const state = await api.state();
    const chapters = await api.chapters();
    expect(state.loading && state.loaded).toBe(true);
    const markup = render(state, chapters, null);
    expect(markup).toContain('class="reload-note" role="status"');
    expect(markup).toContain('Checking example.com for new chapters…');
    expect(markup).toContain('aria-busy="true"');
    expect(markup.indexOf('class="reload-note"')).toBeLessThan(markup.indexOf('class="list-toolbar"'));
    const done = render({ ...state, loading: false }, chapters, null);
    expect(done).not.toContain('reload-note');
    expect(done).not.toContain('aria-busy');
  });
  it('shows a plain "Chapter N" title once, as the number', async () => {
    const api = createMockApi({ scenario: 'loaded', freeze: true });
    const state = await api.state();
    const chapters: Chapter[] = [
      { index: 0, number: 3, title: 'Chapter 3', language: '', group: '', volume: 0, locked: false },
      { index: 1, number: 4, title: 'Chapter 4 - The fight', language: '', group: '', volume: 0, locked: false },
    ];
    const markup = render({ ...state, chapterCount: 2, selectedCount: 0, options: { ...state.options, packAs: 'chapters' } }, chapters, null);
    expect(markup).toContain('<span class="num">3</span><span class="title"></span>');
    expect(markup).toContain('<span class="num">4</span><span class="title">The fight</span>');
  });
});

function title(locked: boolean): string {
  return renderToStaticMarkup(createElement(ChapterTitle, { title: 'Chapter 186', locked }));
}

describe('ChapterTitle', () => {
  it('marks a locked chapter with a lock that names why, in the title', () => {
    const html = title(true);
    expect(html).toContain(`class="lock" role="img" title="${LOCKED_HINT}" aria-label="${LOCKED_HINT}"`);
    expect(html).toContain('class="title"');
    expect(html.indexOf('class="lock"')).toBeLessThan(html.indexOf('Chapter 186'));
  });

  it('shows a free chapter with no lock at all', () => {
    const html = title(false);
    expect(html).not.toContain('lock');
    expect(html).toBe('<span class="title">Chapter 186</span>');
  });
});

describe('ChapterBox', () => {
  const box = (on: boolean, locked: boolean) => renderToStaticMarkup(createElement(ChapterBox, { on, locked }));

  it('greys out the box of a locked chapter', () => {
    expect(box(false, true)).toBe('<span class="box disabled" aria-hidden="true"></span>');
  });

  it('leaves a free chapter\'s box as it was', () => {
    expect(box(false, false)).toBe('<span class="box" aria-hidden="true"></span>');
    expect(box(true, false)).toBe('<span class="box on" aria-hidden="true"></span>');
  });
});
