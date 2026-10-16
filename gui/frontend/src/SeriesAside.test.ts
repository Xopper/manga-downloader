import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { followedOf } from './library';
import { createMockApi } from './mock';
import { SeriesAside } from './SeriesAside';
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
    expect(markup).toContain('Saved up to chapter <strong class="num">1182</strong>');
    expect(markup).toContain('597 chapters · latest 1194');
    expect(markup).toContain('role="progressbar"');
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
