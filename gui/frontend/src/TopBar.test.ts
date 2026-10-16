import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import { TopBar } from './TopBar';
import type { State } from './types';

function render(state: State, over: Partial<Parameters<typeof TopBar>[0]> = {}): string {
  return renderToStaticMarkup(
    createElement(TopBar, {
      state,
      query: '',
      onQuery: () => undefined,
      onLoad: () => undefined,
      onHome: () => undefined,
      onCheck: () => undefined,
      onCancelCheck: () => undefined,
      onSettings: () => undefined,
      settingsOpen: false,
      problem: '',
      onDismiss: () => undefined,
      ...over,
    }),
  );
}

describe('TopBar', () => {
  it('offers the check when something is followed, with the catalog count in the box', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = render(state);
    expect(markup).toContain('Check for new chapters');
    expect(markup).toContain('Search 5 sites, or paste a link');
    expect(markup).toContain('aria-label="Settings"');
  });
  it('hides the check without a library, and asks for a link without catalogs', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true, nolibrary: true, nocatalog: true }).state();
    const markup = render(state);
    expect(markup).not.toContain('Check for new chapters');
    expect(markup).toContain('Paste the link to a series');
  });
  it('shows the progress of a check with a way to stop it', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const state: State = { ...base, library: { ...base.library, checking: true, checkDone: 1, checkTotal: 2 } };
    const markup = render(state);
    expect(markup).toContain('Checking 2 of 2');
    expect(markup).toContain('>Stop<');
    expect(markup).not.toContain('Check for new chapters');
  });
  it('shows a refused load under the bar', async () => {
    const state = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = render(state, { problem: 'site not recognised', query: 'https://x.test/manga/a' });
    expect(markup).toContain('site not recognised');
    expect(markup).toContain('url-box invalid');
    expect(markup).toContain('aria-label="Clear"');
  });
});
