import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { LibraryPanel } from './LibraryPanel';
import { createMockApi } from './mock';
import type { State } from './types';

function render(state: State, over: Partial<Parameters<typeof LibraryPanel>[0]> = {}): string {
  return renderToStaticMarkup(
    createElement(LibraryPanel, {
      state,
      busy: false,
      sort: 'new',
      onSort: () => undefined,
      onOpen: () => undefined,
      onDownloadNew: () => undefined,
      onDownloadAllNew: () => undefined,
      onCancelBatch: () => undefined,
      onUnfollow: () => undefined,
      onOpenURL: () => undefined,
      ...over,
    }),
  );
}

describe('LibraryPanel', () => {
  it('shows a card per followed series with its status and one button', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = render(state);
    expect(markup.match(/<article class="card/g)?.length).toBe(3);
    expect(markup).toContain('12 new · 585 of 597 saved');
    expect(markup).toContain('Download 12 new');
    expect(markup).toContain('Nothing new · all 379 saved');
    expect(markup).toContain('>Nothing new<');
    expect(markup).toContain('Open on the site');
    expect(markup).toContain('Download all new · 12');
    expect(markup).toContain('3 series you follow · 12 new chapters');
    expect(markup).toContain('aria-label="Following One Piece, click to unfollow"');
  });
  it('shows the cover when the entry has one, the first letter otherwise', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = render(state);
    expect(markup.match(/<img class="cover-img" src="data:image\/svg\+xml,[^"]+" alt=""\/>/g)?.length).toBe(2);
    expect(markup.match(/<span class="cover-letter">K<\/span>/g)?.length).toBe(1);
  });
  it('makes the cover a button that opens the series, out of the tab order', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = render(state);
    const covers = markup.match(/<button type="button" class="cover cover-open[^"]*"[^>]*>/g) ?? [];
    expect(covers.length).toBe(3);
    for (const c of covers) {
      expect(c).toContain('tabindex="-1"');
      expect(c).toContain('aria-hidden="true"');
    }
  });
  it('puts the series with new chapters first', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = render(state);
    expect(markup.indexOf('One Piece')).toBeLessThan(markup.indexOf('Berserk'));
    const byTitle = render(state, { sort: 'title' });
    expect(byTitle.indexOf('Berserk')).toBeLessThan(byTitle.indexOf('Kingdom'));
    expect(byTitle.indexOf('Kingdom')).toBeLessThan(byTitle.indexOf('One Piece'));
  });
  it('shows the empty state without a library', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true, nolibrary: true }).state();
    const markup = render(state);
    expect(markup).toContain('Nothing followed yet');
    expect(markup).not.toContain('class="card');
  });
  it('shows the batch line while every new chapter downloads', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const state: State = { ...base, library: { ...base.library, batch: { active: true, done: 0, total: 2, current: 'One Piece', currentURL: 'https://example.com/manga/one-piece' } } };
    const markup = render(state);
    expect(markup).toContain('Downloading 1 of 2');
    expect(markup).toContain('Downloading…</button>');
    expect(markup).not.toContain('Download all new');
  });
});
