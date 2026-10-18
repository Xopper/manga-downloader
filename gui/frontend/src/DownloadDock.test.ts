import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { DownloadDock } from './DownloadDock';
import { createMockApi } from './mock';
import type { State } from './types';

function render(state: State, over: Partial<Parameters<typeof DownloadDock>[0]> = {}): string {
  return renderToStaticMarkup(
    createElement(DownloadDock, {
      state,
      error: '',
      chapters: [],
      selected: [],
      fresh: [],
      onStart: () => undefined,
      onStartNew: () => undefined,
      onCancel: () => undefined,
      onDetails: () => undefined,
      ...over,
    }),
  );
}

describe('DownloadDock', () => {
  it('shows the progress and Stop while the loaded series downloads', async () => {
    const state = await createMockApi({ scenario: 'running', freeze: true }).state();
    const markup = render(state);
    expect(markup).toContain('role="progressbar"');
    expect(markup).toContain('Stop');
    expect(markup).not.toContain('Download 12 chapters');
  });
  it('offers to queue more chapters of the running series', async () => {
    const base = await createMockApi({ scenario: 'running', freeze: true }).state();
    const markup = render({ ...base, selectedCount: 3 });
    expect(markup).toContain('role="progressbar"');
    expect(markup).toContain('Download 3 more chapters after this');
  });
  it('keeps Download while another series downloads, saying these wait their turn', async () => {
    const base = await createMockApi({ scenario: 'running', freeze: true }).state();
    const state: State = { ...base, url: 'https://example.com/manga/other', selectedCount: 2 };
    const markup = render(state);
    expect(markup).not.toContain('role="progressbar"');
    expect(markup).toContain('Download 2 chapters');
    expect(markup).toContain('One Piece is downloading: these start once it finishes.');
    const queued = render({ ...state, queue: [{ id: 2, url: 'https://example.com/manga/x', title: 'X', chapters: 1, fresh: false }] });
    expect(queued).toContain('with 1 download waiting: these start after them.');
  });
  it('shows the note about the last Download for a moment', async () => {
    const base = await createMockApi({ scenario: 'running', freeze: true }).state();
    const state: State = { ...base, url: 'https://example.com/manga/other', selectedCount: 2 };
    expect(render(state, { note: 'Added to the queue' })).toContain('Added to the queue');
    expect(render(state, { note: 'Added to the queue', error: 'nope' })).not.toContain('Added to the queue');
  });
});
