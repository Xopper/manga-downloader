// The home page: the library. One card per followed series with what's new
// and one button, and a button at the top that downloads everything new.

import type { CSSProperties } from 'react';
import { hueOf } from './format';
import { DownloadIcon, FolderIcon, LinkIcon, Spinner } from './icons';
import { cardAction, cardBadges, librarySummary, sortFollowed, totalNew, urlKey, type Sort } from './library';
import { Mascot } from './Mascot';
import { BookmarkIcon } from './ResultsPanel';
import type { Followed, State } from './types';

interface Props {
  state: State;
  /** A load or a check runs: the buttons wait (a download doesn't: more join the queue) */
  busy: boolean;
  sort: Sort;
  onSort: (sort: Sort) => void;
  /** The series page of a followed series (loading it when it isn't the loaded one) */
  onOpen: (f: Followed) => void;
  onDownloadNew: (url: string) => void;
  onDownloadAllNew: () => void;
  onUnfollow: (f: Followed) => void;
  onOpenURL: (url: string) => void;
  /** Shows a series' latest chapter saved in the chosen folder, in the file manager */
  onReveal: (url: string) => void;
}

export function LibraryPanel({ state, busy, sort, onSort, onOpen, onDownloadNew, onDownloadAllNew, onUnfollow, onOpenURL, onReveal }: Props) {
  const library = state.library;
  const fresh = totalNew(library);
  // the series a job of the queue is about: downloading, or waiting its turn
  const downloading = (url: string) => state.running && urlKey(state.run.url) === urlKey(url);
  const waiting = (url: string) => state.queue.some((job) => urlKey(job.url) === urlKey(url));
  // every series with new chapters is downloading or waiting already
  const allQueued = library.followed.filter((f) => f.downloadable && f.new > 0).every((f) => downloading(f.url) || waiting(f.url));

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
        <button
          type="button"
          className="btn primary"
          disabled={busy || fresh === 0 || allQueued}
          title={fresh > 0 && allQueued ? 'Every new chapter is downloading or waiting its turn' : undefined}
          onClick={onDownloadAllNew}
        >
          <DownloadIcon /> {fresh > 0 ? `Download all new · ${fresh}` : 'Nothing new to download'}
        </button>
      </div>

      <div className="lib-grid">
        {sortFollowed(library.followed, sort).map((f) => (
          <Card
            key={f.url}
            f={f}
            busy={busy}
            current={downloading(f.url)}
            waiting={waiting(f.url)}
            onOpen={() => onOpen(f)}
            onDownloadNew={() => onDownloadNew(f.url)}
            onUnfollow={() => onUnfollow(f)}
            onOpenURL={() => onOpenURL(f.url)}
            onReveal={() => onReveal(f.url)}
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
  /** The running download is this series */
  current: boolean;
  /** A download of this series waits its turn */
  waiting: boolean;
  onOpen: () => void;
  onDownloadNew: () => void;
  onUnfollow: () => void;
  onOpenURL: () => void;
  /** Shows its latest chapter saved in the chosen folder, in the file manager */
  onReveal: () => void;
}

function Card({ f, busy, current, waiting, onOpen, onDownloadNew, onUnfollow, onOpenURL, onReveal }: CardProps) {
  const badges = cardBadges(f);
  const action = cardAction(f);
  const letter = (f.title.trim()[0] ?? f.site[0] ?? '?').toUpperCase();
  const canReveal = f.downloadable && f.saved.length > 0;

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
        <span className="hit-tag card-site" title={f.url}>
          <LinkIcon size={11} />
          <span>{f.site}</span>
        </span>
        {'note' in badges ? (
          <span className={`hit-tag card-note${badges.note.tone !== '' ? ` ${badges.note.tone}` : ''}`} title={badges.note.title}>
            {badges.note.text}
          </span>
        ) : (
          <div className="card-slots">
            <span className={`hit-tag${badges.saved.tone !== '' ? ` ${badges.saved.tone}` : ''}`} title={badges.saved.title}>
              Saved <b className="num">{badges.saved.text}</b>
            </span>
            <span className={`hit-tag${badges.fresh.tone !== '' ? ` ${badges.fresh.tone}` : ''}`} title={badges.fresh.title}>
              New <b className="num">{badges.fresh.text}</b>
            </span>
          </div>
        )}
      </div>
      <div className="card-foot">
        <button
          type="button"
          className="btn small icon-only"
          disabled={!canReveal}
          aria-label="Show in its folder"
          title={canReveal ? 'Show the latest chapter saved, in its folder' : 'Nothing saved in this folder yet'}
          onClick={onReveal}
        >
          <FolderIcon size={14} />
        </button>
        {action.kind === 'new' ? (
          <button type="button" className="btn primary small block" disabled={busy || current || waiting} onClick={onDownloadNew}>
            {current ? (
              <>
                <Spinner size={12} /> Downloading…
              </>
            ) : waiting ? (
              <>Waiting its turn…</>
            ) : (
              <>
                <DownloadIcon size={13} /> {action.label}
              </>
            )}
          </button>
        ) : action.kind === 'open' ? (
          <button type="button" className="btn small block" onClick={onOpenURL}>
            <LinkIcon size={13} /> {action.label}
          </button>
        ) : (
          <button type="button" className="btn small block" onClick={onOpen}>
            {action.label}
          </button>
        )}
      </div>
    </article>
  );
}
