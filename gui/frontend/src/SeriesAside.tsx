// The right side of the series page: what belongs to this series. Its
// facts, Follow, how it saves (the Save options), and the Download dock.

import { DownloadDock } from './DownloadDock';
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
  onCancel: () => void;
}

export function SeriesAside(props: Props) {
  const { state, chapters, selected, followed, actionError } = props;
  const { onFollow, onUnfollow, onOpenURL, onOptions, onChooseFolder, onError, onStart, onCancel } = props;
  const host = hostOf(state.url);
  const title = state.title !== '' ? state.title : followed?.title ?? host;
  const letter = (title.trim()[0] ?? '?').toUpperCase();
  const cover = followed?.cover ?? '';
  const now = new Date();

  return (
    <aside className="side series-aside" aria-label="This series">
      <div className="aside-head">
        <div className="cover aside-cover" aria-hidden="true">
          {cover !== '' ? <img className="cover-img" src={cover} alt="" /> : <span className="cover-letter">{letter}</span>}
        </div>
        <div className="aside-id">
          <h2 className="series-title" title={title}>
            {title}
          </h2>
          {host !== '' && (
            <p className="series-meta">
              <a
                href={state.url}
                onClick={(e) => {
                  e.preventDefault();
                  onOpenURL(state.url);
                }}
                title="Open on the site, in your browser"
              >
                {host}
              </a>
            </p>
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

      {followed !== null && followed.downloadable && (
        <dl className="facts" aria-label="Facts">
          <div className="fact">
            <dt>Chapters on the site</dt>
            <dd className="num">{(state.loaded ? state.chapterCount : followed.chapters).toLocaleString('en-US')}</dd>
          </div>
          <div className="fact">
            <dt>New since your last download</dt>
            <dd className={`num${followed.new > 0 ? ' fresh' : ''}`}>{followed.savedUpTo > 0 ? followed.new : 'all of them'}</dd>
          </div>
          <div className="fact">
            <dt>Saved up to</dt>
            <dd className="num">{followed.savedUpTo > 0 ? `chapter ${followed.savedUpTo}` : 'nothing yet'}</dd>
          </div>
          <div className="fact">
            <dt>Last downloaded</dt>
            <dd>{followed.lastSaved !== '' ? ago(followed.lastSaved, now) : 'never'}</dd>
          </div>
        </dl>
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

      <DownloadDock state={state} error={actionError} chapters={chapters} selected={selected} onStart={onStart} onCancel={onCancel} />
    </aside>
  );
}
