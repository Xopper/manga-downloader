import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { followedOf } from './library';
import { createMockApi } from './mock';
import { alreadyHere, chapterMarks, MAP_PAGE, SeriesAside, startPage } from './SeriesAside';
import type { Chapter, Followed, State } from './types';

function render(state: State, followed: Followed | null, chapters: Chapter[] = []): string {
  return renderToStaticMarkup(
    createElement(SeriesAside, {
      state,
      chapters,
      selected: [],
      followed,
      actionError: '',
      onFollow: () => undefined,
      onUnfollow: () => undefined,
      onOpenURL: () => undefined,
      onOptions: () => Promise.resolve(),
      onChooseFolder: () => Promise.resolve(),
      onError: () => undefined,
      onStart: () => undefined,
      onStartNew: () => undefined,
      onCancel: () => undefined,
      onDetails: () => undefined,
    }),
  );
}

describe('SeriesAside', () => {
  it('shows the progress card and Following for a followed series', async () => {
    const state = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const followed = followedOf(state.library, state.url);
    expect(followed).not.toBeNull();
    const markup = render(state, followed);
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain(' Following</button>');
    expect(markup).toContain('class="progress-card"');
    expect(markup).toMatch(/<strong class="num">[\d,]+<\/strong> of 597 saved/);
    expect(markup).toContain('597 chapters · latest 1194');
    // without the chapter list there are no squares, only the bar
    expect(markup).toContain('role="progressbar"');
    expect(markup).not.toContain('map-grid');
    expect(markup).toContain('Save as');
    expect(markup).toContain('Browse…');
    expect(markup).toContain('Download');
    expect(markup).toContain('<img class="cover-img" src="data:image/svg+xml,');
    expect(markup).not.toContain('cover-letter');
  });
  it('offers Follow without the progress card for a series not followed', async () => {
    const state = await createMockApi({ scenario: 'loaded', freeze: true, nolibrary: true }).state();
    const markup = render(state, null);
    expect(markup).toContain('aria-pressed="false"');
    expect(markup).toContain(' Follow</button>');
    expect(markup).not.toContain('progress-card');
    expect(markup).toContain('example.com');
  });
  it('names the site while the series loads', async () => {
    const state = await createMockApi({ scenario: 'loading', freeze: true }).state();
    const markup = render(state, null);
    expect(markup).toContain('example.com');
    expect(markup).not.toContain(' Follow</button>');
  });
  it('offers the new chapters when nothing is picked', async () => {
    const api = createMockApi({ scenario: 'loaded', freeze: true });
    const state = { ...(await api.state()), selectedCount: 0 };
    const chapters = await api.chapters();
    const markup = render(state, followedOf(state.library, state.url), chapters);
    expect(markup).toContain('Download 12 new chapters</button>');
    expect(markup).toContain('this gets the chapters that came out since you followed it');
    expect(markup).toContain('>12 CBZ files</p>');
    expect(markup).not.toContain('Select at least one chapter');
  });
  it('asks for a pick, without a dead hint, when nothing is new', async () => {
    const state = { ...(await createMockApi({ scenario: 'loaded', freeze: true, nolibrary: true }).state()), selectedCount: 0 };
    const markup = render(state, null);
    expect(markup).toContain('Pick chapters to download</button>');
    expect(markup).toContain('disabled=""');
    expect(markup).not.toContain('Select at least one chapter');
    expect(markup).not.toContain('dock-dest');
  });
  it('shows the folder once, from the home folder', async () => {
    const state = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const markup = render({ ...state, options: { ...state.options, outputDir: '/Users/reader/Downloads' } }, null);
    expect(markup).toContain('<bdi>~/Downloads</bdi>');
    expect(markup.match(/Users\/reader\/Downloads/g)?.length).toBe(2); // the two tooltips
  });
});

/** chapters 1..n, listed newest first as sites list them */
function listOf(n: number): Chapter[] {
  return Array.from({ length: n }, (_, i) => ({ index: i, number: n - i }) as Chapter);
}

function entry(over: Partial<Followed>): Followed {
  return {
    url: 'https://example.com/manga/x',
    title: 'X',
    site: 'example.com',
    downloadable: true,
    chapters: 50,
    latest: 50,
    savedUpTo: 0,
    saved: [],
    everSaved: [],
    known: Array.from({ length: 50 }, (_, i) => i + 1),
    baselined: true,
    new: 0,
    followedAt: '',
    checkedAt: '',
    lastSaved: '',
    error: '',
    cover: '',
    ...over,
  };
}

describe('the chapter map', () => {
  it('marks only the chapter saved, in chapter order', () => {
    const marks = chapterMarks(listOf(50), entry({ saved: [20], everSaved: [20] }));
    expect(marks.map((m) => m.number).slice(0, 3)).toEqual([1, 2, 3]);
    expect(marks.filter((m) => m.mark === 'saved').map((m) => m.number)).toEqual([20]);
    expect(marks.filter((m) => m.mark === 'new')).toEqual([]);
  });
  it('marks what came out since the follow as new', () => {
    const f = entry({ known: Array.from({ length: 50 }, (_, i) => i + 1), new: 3 });
    const marks = chapterMarks(listOf(53), f);
    expect(marks.filter((m) => m.mark === 'new').map((m) => m.number)).toEqual([51, 52, 53]);
  });
  it('opens on the page with the first new chapter, else the first not saved, else the last', () => {
    const all = Array.from({ length: 1000 }, (_, i) => i + 1);
    const known = all.slice(0, 990);
    expect(startPage(chapterMarks(listOf(1000), entry({ known, saved: all.slice(0, 500) })))).toBe(9);
    expect(startPage(chapterMarks(listOf(1000), entry({ known: all, saved: all.slice(0, 250) })))).toBe(2);
    expect(startPage(chapterMarks(listOf(1000), entry({ known: all, saved: all })))).toBe(9);
    expect(MAP_PAGE).toBe(100);
  });
  it('draws a page of squares with a pager for a long series', async () => {
    const state = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const f = entry({ url: state.url, saved: [20], everSaved: [20] });
    const markup = render(state, f, listOf(250));
    expect(markup).toContain('<strong class="num">1</strong> of');
    expect((markup.match(/class="map-sq/g) ?? []).length).toBe(100 + 3);
    expect(markup).toContain('Chapters 1–100');
    expect(markup).toContain('1 of 3');
    expect(markup).toContain('title="Chapter 20 · saved"');
    expect(markup).not.toContain('role="progressbar"');
  });
  it('counts the picked chapters already saved here', () => {
    const f = entry({ saved: [1, 2, 3] });
    const list = listOf(10); // index 0 is chapter 10, index 9 chapter 1
    expect(alreadyHere(list, [9, 8, 0], f)).toBe(2);
    expect(alreadyHere(list, [9], null)).toBe(0);
  });
});
