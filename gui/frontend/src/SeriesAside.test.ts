import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { followedOf } from './library';
import { createMockApi } from './mock';
import { SeriesAside } from './SeriesAside';
import type { Followed, State } from './types';

function render(state: State, followed: Followed | null): string {
  return renderToStaticMarkup(
    createElement(SeriesAside, {
      state,
      chapters: [],
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
      onCancel: () => undefined,
    }),
  );
}

describe('SeriesAside', () => {
  it('shows the facts and Following for a followed series', async () => {
    const state = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const followed = followedOf(state.library, state.url);
    expect(followed).not.toBeNull();
    const markup = render(state, followed);
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain(' Following</button>');
    expect(markup).toContain('Chapters on the site');
    expect(markup).toContain('chapter 1182');
    expect(markup).toContain('Save as');
    expect(markup).toContain('Browse…');
    expect(markup).toContain('Download');
    expect(markup).toContain('<img class="cover-img" src="data:image/svg+xml,');
    expect(markup).not.toContain('cover-letter');
  });
  it('offers Follow without the facts for a series not followed', async () => {
    const state = await createMockApi({ scenario: 'loaded', freeze: true, nolibrary: true }).state();
    const markup = render(state, null);
    expect(markup).toContain('aria-pressed="false"');
    expect(markup).toContain(' Follow</button>');
    expect(markup).not.toContain('Chapters on the site');
    expect(markup).toContain('example.com');
  });
  it('names the site while the series loads', async () => {
    const state = await createMockApi({ scenario: 'loading', freeze: true }).state();
    const markup = render(state, null);
    expect(markup).toContain('example.com');
    expect(markup).not.toContain(' Follow</button>');
  });
});
