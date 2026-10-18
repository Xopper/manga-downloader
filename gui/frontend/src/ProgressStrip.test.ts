import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import { ProgressStrip, STRIP_QUEUE_ROWS } from './ProgressStrip';
import type { QueuedJob, State } from './types';

function render(state: State, dismissed = false, runShownElsewhere = false): string {
  return renderToStaticMarkup(
    createElement(ProgressStrip, {
      state,
      dismissed,
      runShownElsewhere,
      onDetails: () => undefined,
      onCancel: () => undefined,
      onRemoveQueued: () => undefined,
      onOpenFolder: () => undefined,
      onDismiss: () => undefined,
    }),
  );
}

function waiting(n: number): QueuedJob[] {
  return Array.from({ length: n }, (_, i) => ({ id: i + 2, url: `https://example.com/manga/s${i}`, title: `Series ${i + 1}`, chapters: i + 1, fresh: i === 0 }));
}

describe('ProgressStrip', () => {
  it('shows nothing before a run', async () => {
    const state = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    expect(render(state)).toBe('');
  });
  it('shows the running download with its progress and Cancel', async () => {
    const state = await createMockApi({ scenario: 'running', freeze: true }).state();
    const markup = render(state);
    expect(markup).toContain('Downloading One Piece');
    expect(markup).toMatch(/\d+ of 12 chapters done/);
    expect(markup).toContain('role="progressbar"');
    expect(markup).toContain('>Details<');
    expect(markup).toContain('Stop');
    expect(markup).not.toContain('strip-queue');
  });
  it('lists the downloads waiting their turn, each with a remove button, and folds the rest', async () => {
    const base = await createMockApi({ scenario: 'running', freeze: true }).state();
    const markup = render({ ...base, queue: waiting(2) });
    expect(markup).toContain('aria-label="Waiting downloads"');
    expect(markup).toContain('Series 1');
    expect(markup).toContain('1 new chapter');
    expect(markup).toContain('2 chapters');
    expect(markup).toContain('aria-label="Remove Series 2 from the queue"');
    const many = render({ ...base, queue: waiting(STRIP_QUEUE_ROWS + 2) });
    expect(many).toContain(`Series ${STRIP_QUEUE_ROWS}`);
    expect(many).not.toContain(`Series ${STRIP_QUEUE_ROWS + 1}<`);
    expect(many).toContain('and 2 more downloads');
  });
  it('shows only the queue when the page shows the run already', async () => {
    const base = await createMockApi({ scenario: 'running', freeze: true }).state();
    const markup = render({ ...base, queue: waiting(1) }, false, true);
    expect(markup).toContain('Series 1');
    expect(markup).not.toContain('role="progressbar"');
  });
  it('says a fresh job is fetching its list', async () => {
    const base = await createMockApi({ scenario: 'running', freeze: true }).state();
    const markup = render({ ...base, run: { ...base.run, fresh: true, fetching: true, title: 'Berserk' } });
    expect(markup).toContain('Fetching the chapter list of Berserk');
    expect(markup).not.toContain('role="progressbar"');
  });
  it('leaves the running download out when the page shows it already, but not how it ended', async () => {
    const running = await createMockApi({ scenario: 'running', freeze: true }).state();
    expect(render(running, false, true)).toBe('');
    const done = await createMockApi({ scenario: 'done', freeze: true }).state();
    expect(render(done, false, true)).toContain('Open folder');
  });
  it('says the run is cancelling', async () => {
    const state = await createMockApi({ scenario: 'cancelling', freeze: true }).state();
    expect(render(state)).toContain('Stopping…');
  });
  it('shows how the run ended, until dismissed', async () => {
    const state = await createMockApi({ scenario: 'done', freeze: true }).state();
    const markup = render(state);
    expect(markup).toContain('12 saved');
    expect(markup).toContain('Open folder');
    expect(markup).toContain('tone-success');
    expect(render(state, true)).toBe('');
  });
});
