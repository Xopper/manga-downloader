// The right side of the series page: what belongs to this series. Its
// facts, Follow, how it saves (the Save options), and the Download dock.

import type { CSSProperties } from 'react';
import { DownloadDock } from './DownloadDock';
import { formatNumber, hueOf } from './format';
import { LinkIcon } from './icons';
import { newIndices } from './library';
import { SaveOptions } from './OptionsPanel';
import { BookmarkIcon } from './ResultsPanel';
import { hostOf } from './run';
import { ago } from './search';
import type { Chapter, Followed, Options, State } from './types';

interface Props {
  state: State;
  chapters: Chapter[];
  selected: number[];
  /** The library's entry for the loaded series, null when it isn't followed */
  followed: Followed | null;
  /** The backend's refusal of the last Download or Cancel */
  actionError: string;
  onFollow: () => void;
  onUnfollow: () => void;
  onOpenURL: (url: string) => void;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onChooseFolder: () => Promise<void>;
  onError: (message: string) => void;
  onStart: () => void;
  /** Selects these chapters (the new ones) and starts, from a dock with nothing picked */
  onStartNew: (indices: number[]) => void;
  onCancel: () => void;
  /** Opens the per-chapter activity of the running download */
  onDetails: () => void;
}

export function SeriesAside(props: Props) {
  const { state, chapters, selected, followed, actionError } = props;
  const { onFollow, onUnfollow, onOpenURL, onOptions, onChooseFolder, onError, onStart, onStartNew, onCancel, onDetails } = props;
  const host = hostOf(state.url);
  const title = state.title !== '' ? state.title : followed?.title ?? host;
  const letter = (title.trim()[0] ?? '?').toUpperCase();
  const cover = followed?.cover ?? '';
  const now = new Date();
  // what the dock offers when nothing is picked: the chapters that came out since it was followed
  const fresh = newIndices(chapters, followed);

  return (
    <aside className="side series-aside" aria-label="This series">
      <div className="aside-head">
        <div
          className={`cover aside-cover${cover === '' ? ' tinted' : ''}`}
          style={cover === '' ? ({ '--hue': hueOf(title) } as CSSProperties) : undefined}
          aria-hidden="true"
        >
          {cover !== '' ? <img className="cover-img" src={cover} alt="" /> : <span className="cover-letter">{letter}</span>}
        </div>
        <div className="aside-id">
          <h2 className="series-title" title={title}>
            {title}
          </h2>
          {host !== '' && (
            <a
              className="aside-host"
              href={state.url}
              onClick={(e) => {
                e.preventDefault();
                onOpenURL(state.url);
              }}
              title="Open on the site, in your browser"
            >
              <LinkIcon size={12} />
              <span>{host}</span>
            </a>
          )}
          {state.loaded && (
            <button
              type="button"
              className={`btn small follow${followed !== null ? ' on' : ''}`}
              aria-pressed={followed !== null}
              onClick={() => (followed !== null ? onUnfollow() : onFollow())}
              title={followed !== null ? 'Following · click to unfollow' : 'Keep it in your library'}
            >
              <BookmarkIcon filled={followed !== null} size={13} /> {followed !== null ? 'Following' : 'Follow'}
            </button>
          )}
        </div>
      </div>

      {followed !== null && followed.downloadable && (state.loaded || followed.chapters > 0) && (
        <Progress count={state.loaded ? state.chapterCount : followed.chapters} followed={followed} now={now} />
      )}

      <div className="side-body">
        <SaveOptions
          state={state}
          onOptions={onOptions}
          onChooseFolder={onChooseFolder}
          onError={onError}
          heading="Save as"
        />
      </div>

      <DownloadDock
        state={state}
        error={actionError}
        chapters={chapters}
        selected={selected}
        fresh={fresh}
        onStart={onStart}
        onStartNew={() => onStartNew(fresh)}
        onCancel={onCancel}
        onDetails={onDetails}
      />
    </aside>
  );
}

/**
 * How far the user is with a followed series, at a glance: the last chapter
 * saved against the latest one, what's new, and when it was last saved.
 */
function Progress({ count, followed, now }: { count: number; followed: Followed; now: Date }) {
  const saved = followed.savedUpTo;
  const latest = followed.latest;
  const fraction = saved > 0 && latest > 0 ? Math.min(1, saved / latest) : 0;
  const fresh = followed.new;
  const meta = [`${count.toLocaleString('en-US')} ${count === 1 ? 'chapter' : 'chapters'}`];
  if (latest > 0) {
    meta.push(`latest ${formatNumber(latest)}`);
  }
  if (followed.lastSaved !== '') {
    meta.push(`saved ${ago(followed.lastSaved, now)}`);
  }
  return (
    <section className="progress-card" aria-label="Your progress">
      <div className="progress-top">
        <span className="progress-title">
          {saved > 0 ? (
            <>
              Saved up to chapter <strong className="num">{formatNumber(saved)}</strong>
            </>
          ) : (
            'Nothing saved yet'
          )}
        </span>
        {fresh > 0 ? (
          <span className="pill-new">{fresh.toLocaleString('en-US')} new</span>
        ) : (
          <span className="pill-done">Up to date</span>
        )}
      </div>
      <div
        className="bar progress-bar"
        role="progressbar"
        aria-label="Saved so far"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(fraction * 100)}
      >
        <span style={{ transform: `scaleX(${fraction})` }} />
      </div>
      <p className="progress-meta">{meta.join(' · ')}</p>
    </section>
  );
}
