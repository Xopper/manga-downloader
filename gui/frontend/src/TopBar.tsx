// The top of the window, on every page: the brand (a way home), one box
// that takes a title to search or a link to load, Check for new chapters,
// and Settings. A link loads once the typing pauses (autoLoad.ts) or on
// Enter; anything else is a search, shown by App as the results page.

import { useEffect, useRef } from 'react';
import { AUTO_LOAD_DELAY_MS, shouldAutoLoad } from './autoLoad';
import { AlertIcon, RefreshIcon, SearchIcon, SlidersIcon, Spinner, XIcon } from './icons';
import { isLinkQuery } from './library';
import { Mascot, useExpression } from './Mascot';
import type { State } from './types';

interface Props {
  state: State;
  /** The box's text: a title being searched, or a link about to load */
  query: string;
  onQuery: (text: string) => void;
  onLoad: (url: string) => void;
  onHome: () => void;
  onCheck: () => void;
  onCancelCheck: () => void;
  onSettings: () => void;
  settingsOpen: boolean;
  /** A refused load or option, shown under the bar */
  problem: string;
  /** Closes the problem banner */
  onDismiss: () => void;
}

export function TopBar({ state, query, onQuery, onLoad, onHome, onCheck, onCancelCheck, onSettings, settingsOpen, problem, onDismiss }: Props) {
  const busy = state.loading || state.running || state.library.checking || state.library.batch.active;
  const expression = useExpression(state);
  const library = state.library;
  const canCheck = library.followed.some((f) => f.downloadable);
  // a pasted link loads once the typing pauses; the timer reads the latest
  // state, since it fires after the render that started it
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const latest = useRef({ url: state.url, busy });
  useEffect(() => {
    latest.current = { url: state.url, busy };
  }, [state.url, busy]);
  useEffect(() => () => clearTimeout(timer.current), []);

  const change = (value: string) => {
    onQuery(value);
    clearTimeout(timer.current);
    if (isLinkQuery(value)) {
      timer.current = setTimeout(() => {
        if (shouldAutoLoad(value, latest.current)) {
          onLoad(value.trim());
        }
      }, AUTO_LOAD_DELAY_MS);
    }
  };

  return (
    <header className="topbar">
      <div className="topbar-row">
        <button type="button" className="brand brand-home" title="Library" onClick={onHome}>
          <Mascot expression={expression} size={40} />
          <span className="brand-name">Manga Downloader</span>
        </button>
        <form
          className="topbar-search"
          role="search"
          onSubmit={(e) => {
            e.preventDefault();
            if (isLinkQuery(query)) {
              clearTimeout(timer.current);
              onLoad(query.trim());
            }
          }}
        >
          <div className={`url-box${problem !== '' ? ' invalid' : ''}`}>
            <SearchIcon className="url-icon" />
            <input
              type="search"
              className="url-input"
              aria-label="Search the catalogs, or paste a link"
              placeholder={
                state.catalog.sites > 0
                  ? `Search ${state.catalog.sites} sites, or paste a link`
                  : 'Paste the link to a series, or search once the catalogs are downloaded'
              }
              value={query}
              spellCheck={false}
              autoComplete="off"
              onChange={(e) => change(e.target.value)}
              onFocus={(e) => e.currentTarget.select()}
            />
            {query !== '' && (
              <button
                type="button"
                className="url-clear"
                aria-label="Clear"
                title="Clear"
                onClick={() => {
                  clearTimeout(timer.current);
                  onQuery('');
                }}
              >
                ×
              </button>
            )}
          </div>
        </form>
        {canCheck &&
          (library.checking ? (
            <div className="checking" role="status">
              <Spinner />
              <span>
                Checking {Math.min(library.checkDone + 1, library.checkTotal)} of {library.checkTotal}…
              </span>
              <button type="button" className="btn small ghost" onClick={onCancelCheck}>
                Stop
              </button>
            </div>
          ) : (
            <button type="button" className="btn" disabled={busy} onClick={onCheck} title="Ask every followed site for its chapter list">
              <RefreshIcon /> Check for new chapters
            </button>
          ))}
        <button
          type="button"
          className={`btn ghost icon-only${settingsOpen ? ' on' : ''}`}
          aria-label="Settings"
          aria-pressed={settingsOpen}
          title="Settings"
          onClick={onSettings}
        >
          <SlidersIcon size={16} />
        </button>
      </div>
      {problem !== '' && (
        <div className="banner danger" role="alert">
          <AlertIcon />
          <span>{problem}</span>
          <button type="button" className="banner-close" aria-label="Dismiss" title="Dismiss" onClick={onDismiss}>
            <XIcon size={13} />
          </button>
        </div>
      )}
    </header>
  );
}
