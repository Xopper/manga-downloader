// The results page: what the top box's query found in the catalogs, as a
// table (title, site, language, chapters, follow). Picking a row loads it;
// a search-only row is explained instead, with its link to take elsewhere.
// The query lives in App (useSearch); this is a plain render.

import { AlertIcon, Spinner } from './icons';
import { Mascot } from './Mascot';
import { catalogSummary, resultRows, siteGroups } from './search';
import { SitePicker } from './SitePicker';
import type { CatalogInfo, ChapterCount, Hit, SiteInfo } from './types';

/** A search-only hit the user picked, and the query and site it was picked under */
export interface BlockedHit {
  hit: Hit;
  query: string;
  site: string;
}

export interface ResultsPanelProps {
  catalog: CatalogInfo;
  sites: SiteInfo[];
  query: string;
  site: string;
  hits: Hit[];
  /** Chapter counts the window fetched for hits the catalogs had none for, by URL */
  counts?: Record<string, ChapterCount>;
  searching: boolean;
  /** A load or a download runs: a pick waits, following doesn't */
  busy: boolean;
  /** A picked hit from a search-only site, explained instead of loaded, with the search it was picked under: the note only shows while that search is still the one on screen */
  blocked: BlockedHit | null;
  /** The blocked hit's link was just copied */
  copied: boolean;
  onSite: (site: string) => void;
  /** A picked hit: loaded when its site is downloadable, else shown as blocked */
  onPick: (hit: Hit) => void;
  onFollow: (hit: Hit) => void;
  onUnfollow: (url: string) => void;
  onUpdate: () => void;
  /** The blocked hit's link to the clipboard */
  onCopy: (url: string) => void;
  /** The blocked hit's link in the system's browser */
  onOpen: (url: string) => void;
  onDismissBlocked: () => void;
}

export function ResultsPanel(props: ResultsPanelProps) {
  const { catalog, sites, query, site, hits, counts = {}, searching, busy, blocked, copied } = props;
  const { onSite, onPick, onFollow, onUnfollow, onUpdate, onCopy, onOpen, onDismissBlocked } = props;
  const summary = catalogSummary(catalog, new Date());
  if (catalog.sites === 0) {
    return (
      <section className="panel results empty-state" aria-label="Results">
        <Mascot expression="soft" size={112} />
        <h2>{summary}</h2>
        <p>The catalogs list every series of the supported sites, so you can search them here without a link.</p>
        <button type="button" className="btn primary" disabled={catalog.updating} onClick={onUpdate}>
          {catalog.updating ? (
            <>
              <Spinner /> Downloading…
            </>
          ) : (
            'Download the catalogs'
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
  // the note belongs to the search it came from: a new query or site clears it
  const note = blocked !== null && blocked.query === query && blocked.site === site ? blocked.hit : null;

  return (
    <section className="panel results" aria-label="Results">
      <div className="list-toolbar search-head">
        <span className="results-count">
          {searching ? (
            <>
              <Spinner size={12} /> Searching…
            </>
          ) : rows.length === 0 ? (
            `Nothing found for “${query.trim()}”`
          ) : (
            `${rows.length} ${rows.length === 1 ? 'result' : 'results'} for “${query.trim()}”`
          )}
        </span>
        <SitePicker groups={siteGroups(sites)} value={site} onChange={onSite} />
        <span className="spacer" />
        <span className="muted">{summary}</span>
        {catalog.error !== '' && <AlertIcon />}
        <button type="button" className="btn small" disabled={catalog.updating} onClick={onUpdate} title="Download the catalogs that changed">
          {catalog.updating ? <Spinner size={11} /> : 'Update'}
        </button>
      </div>
      {note !== null && (
        <div className="banner search-only-note" role="status">
          <AlertIcon />
          <span>
            <strong>{note.site}</strong> isn't supported for download yet. The title is <em>{note.title}</em>; you can take
            its link elsewhere.
          </span>
          <button type="button" className="btn small" onClick={() => onCopy(note.url)}>
            {copied ? 'Copied' : 'Copy link'}
          </button>
          <button type="button" className="btn small" onClick={() => onOpen(note.url)}>
            Open in browser
          </button>
          <button type="button" className="btn small" aria-label="Dismiss" onClick={onDismissBlocked}>
            ×
          </button>
        </div>
      )}
      {rows.length === 0 && !searching ? (
        <div className="search-empty">
          <p className="muted">No series called “{query.trim()}” in the catalogs. Try fewer words, or paste its link.</p>
        </div>
      ) : (
        <>
          <div className="hit-head" aria-hidden="true">
            <span>Title</span>
            <span className="hit-chapters">Chapters</span>
            <span className="hit-follow">Follow</span>
          </div>
          <ul className="hits" aria-label="Results">
            {rows.map((r, i) => {
              const hit = hits[i];
              return (
                <li key={r.key} className="hit">
                  <button type="button" className="hit-pick" data-url={r.url} disabled={busy} onClick={() => onPick(hit)}>
                    <span className="hit-title">{r.title}</span>
                    <span className="hit-site">
                      {r.site}
                      {r.language !== '' && <span className="hit-lang">{r.language}</span>}
                      {r.nsfw && <span className="hit-tag nsfw">18+</span>}
                      {!r.downloadable && <span className="hit-tag search-only">search only</span>}
                    </span>
                  </button>
                  {hit.chapters === 0 ? (
                    <FetchedCount hit={hit} count={counts[hit.url]} />
                  ) : hit.approximate ? (
                    <span className="hit-chapters num approx" title="latest chapter the site listed">
                      <span aria-hidden="true">≈ </span>
                      <span className="sr-only">about </span>
                      {hit.chapters.toLocaleString('en-US')}
                    </span>
                  ) : (
                    <span className="hit-chapters num">{hit.chapters.toLocaleString('en-US')}</span>
                  )}
                  <button
                    type="button"
                    className={`star${hit.followed ? ' on' : ''}`}
                    aria-pressed={hit.followed}
                    aria-label={hit.followed ? `Following ${r.title}, click to unfollow` : `Follow ${r.title}`}
                    title={hit.followed ? 'Following · click to unfollow' : 'Follow: keep it in your library'}
                    onClick={() => (hit.followed ? onUnfollow(hit.url) : onFollow(hit))}
                  >
                    <BookmarkIcon filled={hit.followed} />
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </section>
  );
}

/** The follow mark: a bookmark, filled when following */
export function BookmarkIcon({ filled, size = 15 }: { filled: boolean; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={filled ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M6 3h12v18l-6-4-6 4z" />
    </svg>
  );
}

/**
 * The count of a hit the catalogs had no number for: the one the window
 * fetched, a spinner while it does, or a dash that says why there's none
 */
function FetchedCount({ hit, count }: { hit: Hit; count: ChapterCount | undefined }) {
  if (typeof count === 'number' && count > 0) {
    return <span className="hit-chapters num">{count.toLocaleString('en-US')}</span>;
  }
  if (count === 'counting') {
    return (
      <span className="hit-chapters muted" title="Counting its chapters…">
        <Spinner size={12} />
        <span className="sr-only">counting</span>
      </span>
    );
  }
  const why = !hit.downloadable
    ? "Search only: the app can't read this site, so it can't count its chapters"
    : count === 'failed'
      ? "Couldn't count its chapters: the site didn't answer, or needs the browser window. Open it to see them"
      : 'Open it to see how many chapters it has';
  return (
    <span className="hit-chapters muted" title={why}>
      <span aria-hidden="true">—</span>
      <span className="sr-only">{why}</span>
    </span>
  );
}
