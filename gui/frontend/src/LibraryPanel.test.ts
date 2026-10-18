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
      onUnfollow: () => undefined,
      onOpenURL: () => undefined,
      onReveal: () => undefined,
      ...over,
    }),
  );
}

describe('LibraryPanel', () => {
  it('shows a card per followed series with its status and one button', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = render(state);
    expect(markup.match(/<article class="card/g)?.length).toBe(3);
    expect(markup).toContain('Saved <b class="num">585/597</b>');
    expect(markup).toContain('New <b class="num">12</b>');
    expect(markup).toContain('Download 12 new');
    expect(markup).toContain('Saved <b class="num">379/379</b>');
    expect(markup).toContain('>Open series</button>');
    expect(markup).toContain('search only');
    expect(markup).toContain('Open on the site');
    // the folder button of each card; the search-only one has nothing saved
    expect(markup.match(/aria-label="Show in its folder"/g)?.length).toBe(3);
    expect(markup.match(/disabled="" aria-label="Show in its folder"/g)?.length).toBe(1);
    // no fact twice: the chapter count only in the Saved badge, no status line
    expect(markup).not.toContain('card-meta');
    expect(markup).not.toContain('card-status');
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
  it('says which card is downloading and which waits its turn', async () => {
    const api = createMockApi({ scenario: 'empty', freeze: true });
    const base = await api.state();
    const state: State = {
      ...base,
      running: true,
      run: { ...base.run, id: 1, url: 'https://example.com/manga/one-piece', title: 'One Piece', fresh: true },
      queue: [{ id: 2, url: 'https://www.mangaread.org/manga/berserk/', title: 'Berserk', chapters: 3, fresh: true }],
    };
    const markup = render(state);
    expect(markup).toContain('Downloading…</button>');
    // every series with new chapters is downloading already: nothing left to queue
    expect(markup).toContain('Every new chapter is downloading or waiting its turn');
    expect(markup).toContain('Download all new · 12');
    // Berserk has nothing new, so its card keeps Open series; a waiting card says so
    const waiting: State = { ...state, library: { ...state.library, followed: state.library.followed.map((f) => (f.title === 'Berserk' ? { ...f, new: 3 } : f)) } };
    expect(render(waiting)).toContain('Waiting its turn…');
  });
  it('lets Download all new queue while a download of another series runs', async () => {
    const api = createMockApi({ scenario: 'running', freeze: true });
    const base = await api.state();
    const state: State = { ...base, run: { ...base.run, url: 'https://example.com/manga/other' } };
    const markup = render(state);
    expect(markup).toContain('<button type="button" class="btn primary">');
    expect(markup).toContain('Download all new · 12');
  });
});
