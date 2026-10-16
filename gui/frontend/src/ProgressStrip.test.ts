import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import { ProgressStrip } from './ProgressStrip';
import type { State } from './types';

function render(state: State, dismissed = false, runShownElsewhere = false): string {
  return renderToStaticMarkup(
    createElement(ProgressStrip, {
      state,
      dismissed,
      runShownElsewhere,
      onDetails: () => undefined,
      onCancel: () => undefined,
      onOpenFolder: () => undefined,
      onDismiss: () => undefined,
    }),
  );
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
    expect(markup).toContain('Cancel');
  });
  it('leaves the running download out when the page shows it already, but not how it ended', async () => {
    const running = await createMockApi({ scenario: 'running', freeze: true }).state();
    expect(render(running, false, true)).toBe('');
    const done = await createMockApi({ scenario: 'done', freeze: true }).state();
    expect(render(done, false, true)).toContain('Open folder');
  });
  it('says the run is cancelling', async () => {
    const state = await createMockApi({ scenario: 'cancelling', freeze: true }).state();
    expect(render(state)).toContain('Cancelling…');
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
