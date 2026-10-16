// The home page: the library. One card per followed series with what's new
// and one button, and a button at the top that downloads everything new.

import type { CSSProperties } from 'react';
import { hueOf } from './format';
import { DownloadIcon, Spinner, XIcon } from './icons';
import { cardAction, cardStatus, librarySummary, sortFollowed, totalNew, urlKey, type Sort } from './library';
import { Mascot } from './Mascot';
import { BookmarkIcon } from './ResultsPanel';
import type { Followed, State } from './types';

interface Props {
  state: State;
  /** A load, a download or a check runs: the buttons wait */
  busy: boolean;
  sort: Sort;
  onSort: (sort: Sort) => void;
  /** The series page of a followed series (loading it when it isn't the loaded one) */
  onOpen: (f: Followed) => void;
  onDownloadNew: (url: string) => void;
  onDownloadAllNew: () => void;
  onCancelBatch: () => void;
  onUnfollow: (f: Followed) => void;
  onOpenURL: (url: string) => void;
}

export function LibraryPanel({ state, busy, sort, onSort, onOpen, onDownloadNew, onDownloadAllNew, onCancelBatch, onUnfollow, onOpenURL }: Props) {
  const library = state.library;
  const fresh = totalNew(library);
  const batch = library.batch;

  if (library.followed.length === 0) {
    return (
      <section className="library empty-state" aria-label="Library">
        <Mascot expression="happy" size={112} />
        <h2>Nothing followed yet</h2>
        <p>Search a title in the box above, or paste a link, and press Follow on it.</p>
        <p className="muted">
          Followed series show up here with their new chapters, ready to download in one click. The list stays on this
          computer.
        </p>
      </section>
    );
  }

  return (
    <section className="library" aria-label="Library">
      <div className="lib-head">
        <div className="lib-title">
          <h1>Library</h1>
          <p className="muted">{librarySummary(library, new Date())}</p>
        </div>
        <label className="lib-sort">
          <span className="muted small">Sort</span>
          <select className="input select-input small" aria-label="Sort" value={sort} onChange={(e) => onSort(e.target.value as Sort)}>
            <option value="new">New chapters first</option>
            <option value="title">Title</option>
            <option value="saved">Last downloaded</option>
          </select>
        </label>
        {batch.active ? (
          <div className="batch-line" role="status">
            <Spinner />
            <span>
              Downloading {Math.min(batch.done + 1, batch.total)} of {batch.total}
              {batch.current !== '' && <> · {batch.current}</>}
            </span>
            <button type="button" className="btn small" onClick={onCancelBatch}>
              <XIcon size={13} /> Stop
            </button>
          </div>
        ) : (
          <button type="button" className="btn primary" disabled={busy || fresh === 0} onClick={onDownloadAllNew}>
            <DownloadIcon /> {fresh > 0 ? `Download all new · ${fresh}` : 'Nothing new to download'}
          </button>
        )}
      </div>

      <div className="lib-grid">
        {sortFollowed(library.followed, sort).map((f) => (
          <Card
            key={f.url}
            f={f}
            busy={busy || batch.active}
            current={batch.active && urlKey(batch.currentURL) === urlKey(f.url)}
            onOpen={() => onOpen(f)}
            onDownloadNew={() => onDownloadNew(f.url)}
            onUnfollow={() => onUnfollow(f)}
            onOpenURL={() => onOpenURL(f.url)}
          />
        ))}
      </div>

      <p className="muted small lib-foot">
        A series joins the library when you press Follow on it. The list is saved on this computer only.
      </p>
    </section>
  );
}

interface CardProps {
  f: Followed;
  busy: boolean;
  /** The batch is downloading this one */
  current: boolean;
  onOpen: () => void;
  onDownloadNew: () => void;
  onUnfollow: () => void;
  onOpenURL: () => void;
}

function Card({ f, busy, current, onOpen, onDownloadNew, onUnfollow, onOpenURL }: CardProps) {
  const status = cardStatus(f);
  const action = cardAction(f);
  const letter = (f.title.trim()[0] ?? f.site[0] ?? '?').toUpperCase();

  return (
    <article className={`card${f.new > 0 && f.downloadable ? ' fresh' : ''}`} aria-label={f.title}>
      {/* the cover opens the series too, a bigger target than the title; the
          title stays the one keyboard and screen reader stop */}
      <button
        type="button"
        className={`cover cover-open${f.cover === '' ? ' tinted' : ''}`}
        style={f.cover === '' ? ({ '--hue': hueOf(f.title) } as CSSProperties) : undefined}
        tabIndex={-1}
        aria-hidden="true"
        title={f.url}
        onClick={onOpen}
      >
        {f.cover !== '' ? <img className="cover-img" src={f.cover} alt="" /> : <span className="cover-letter">{letter}</span>}
      </button>
      <button
        type="button"
        className="pin on"
        aria-pressed="true"
        aria-label={`Following ${f.title}, click to unfollow`}
        title="Following · click to unfollow"
        onClick={onUnfollow}
      >
        <BookmarkIcon filled />
      </button>
      <div className="card-body">
        <h3 className="card-title">
          <button type="button" className="card-link" onClick={onOpen} title={f.url}>
            {f.title !== '' ? f.title : f.url}
          </button>
        </h3>
        <p className="card-meta">
          {f.site}
          {f.chapters > 0 && <> · {f.chapters.toLocaleString('en-US')} chapters</>}
        </p>
        <p className={`card-status tone-${status.tone}`}>{status.text}</p>
      </div>
      <div className="card-foot">
        {action.kind === 'new' ? (
          <button type="button" className="btn primary small block" disabled={busy} onClick={onDownloadNew}>
            {current ? (
              <>
                <Spinner size={12} /> Downloading…
              </>
            ) : (
              action.label
            )}
          </button>
        ) : action.kind === 'open' ? (
          <button type="button" className="btn small block" onClick={onOpenURL}>
            {action.label}
          </button>
        ) : (
          <button type="button" className="btn small block" disabled>
            {action.label}
          </button>
        )}
      </div>
    </article>
  );
}
