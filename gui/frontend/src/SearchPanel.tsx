// The Search mode's panel, in the chapter list's place: a query, a site
// picker, and the matching series. Picking one loads it as a pasted URL would.
// The state lives in App (useSearch), so this stays a plain render.

import { AlertIcon, Spinner } from './icons';
import { Mascot } from './Mascot';
import { catalogSummary, resultRows, siteGroups } from './search';
import type { CatalogInfo, Hit, SiteInfo } from './types';

export interface SearchPanelProps {
  catalog: CatalogInfo;
  sites: SiteInfo[];
  query: string;
  site: string;
  hits: Hit[];
  searching: boolean;
  /** A load or a download runs: the field is read-only, a pick waits */
  busy: boolean;
  /** A picked hit from a search-only site, explained instead of loaded; null when none */
  blocked: Hit | null;
  /** The blocked hit's link was just copied */
  copied: boolean;
  onQuery: (query: string) => void;
  onSite: (site: string) => void;
  /** A picked hit: loaded when its site is downloadable, else shown as blocked */
  onPick: (hit: Hit) => void;
  onUpdate: () => void;
  /** The blocked hit's link to the clipboard */
  onCopy: (url: string) => void;
  /** The blocked hit's link in the system's browser */
  onOpen: (url: string) => void;
  onDismissBlocked: () => void;
}

export function SearchPanel({ catalog, sites, query, site, hits, searching, busy, blocked, copied, onQuery, onSite, onPick, onUpdate, onCopy, onOpen, onDismissBlocked }: SearchPanelProps) {
  const summary = catalogSummary(catalog, new Date());
  if (catalog.sites === 0) {
    return (
      <section className="panel chapters empty-state search" aria-label="Search">
        <Mascot expression="soft" size={112} />
        <h2>{summary}</h2>
        <p>The catalogs list every series of the supported sites, so you can search them here without a link.</p>
        <button type="button" className="btn primary" disabled={catalog.updating} onClick={onUpdate}>
          {catalog.updating ? (
            <>
              <Spinner /> Downloading…
            </>
          ) : (
            'Update now'
          )}
        </button>
        {catalog.error !== '' && (
          <p className="error-text" role="alert">
            {catalog.error}
          </p>
        )}
      </section>
    );
  }

  const rows = resultRows(hits);
  const typed = query.trim() !== '';

  return (
    <section className="panel chapters search" aria-label="Search">
      <form
        className="search-form"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <div className="url-box search-box">
          <input
            type="search"
            className="url-input"
            aria-label="Search the catalogs"
            placeholder="Title, or a few words of it"
            value={query}
            readOnly={busy}
            spellCheck={false}
            autoComplete="off"
            autoFocus
            onChange={(e) => onQuery(e.target.value)}
          />
          {searching && <Spinner />}
        </div>
        <select className="input select-input search-site" aria-label="Site" value={site} onChange={(e) => onSite(e.target.value)}>
          {siteGroups(sites).map((g) =>
            g.label === '' ? (
              g.options.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))
            ) : (
              <optgroup key={g.label} label={g.label}>
                {g.options.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </optgroup>
            ),
          )}
        </select>
      </form>
      <div className="list-toolbar search-head">
        <span className="muted">{summary}</span>
        {catalog.error !== '' && <AlertIcon />}
        <span className="spacer" />
        {typed && !searching && (
          <span className="muted">
            {rows.length === 0 ? 'Nothing found' : `${rows.length} ${rows.length === 1 ? 'result' : 'results'}`}
          </span>
        )}
        <button type="button" className="btn small" disabled={catalog.updating} onClick={onUpdate} title="Download the catalogs that changed">
          {catalog.updating ? <Spinner size={11} /> : 'Update'}
        </button>
      </div>
      {blocked !== null && (
        <div className="banner search-only-note" role="status">
          <AlertIcon />
          <span>
            <strong>{blocked.site}</strong> isn't supported for download yet. The title is <em>{blocked.title}</em>; you can
            take its link elsewhere.
          </span>
          <button type="button" className="btn small" onClick={() => onCopy(blocked.url)}>
            {copied ? 'Copied' : 'Copy link'}
          </button>
          <button type="button" className="btn small" onClick={() => onOpen(blocked.url)}>
            Open in browser
          </button>
          <button type="button" className="btn small" aria-label="Dismiss" onClick={onDismissBlocked}>
            ×
          </button>
        </div>
      )}
      {!typed ? (
        <div className="search-empty">
          <p className="muted">Type to search {catalog.entries.toLocaleString('en-US')} titles.</p>
        </div>
      ) : rows.length === 0 && !searching ? (
        <div className="search-empty">
          <p className="muted">No results for “{query.trim()}”</p>
        </div>
      ) : (
        <ul className="hits" aria-label="Results">
          {rows.map((r, i) => (
            <li key={r.key}>
              <button type="button" className="hit" data-url={r.url} disabled={busy} onClick={() => onPick(hits[i])}>
                <span className="hit-title">{r.title}</span>
                <span className="hit-site">
                  {r.site}
                  {r.language !== '' && <span className="hit-lang">{r.language}</span>}
                  {r.nsfw && <span className="hit-tag nsfw">18+</span>}
                  {!r.downloadable && <span className="hit-tag search-only">search only</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
