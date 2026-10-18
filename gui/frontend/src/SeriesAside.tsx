// The right side of the series page: what belongs to this series. Its
// facts, Follow, how it saves (the Save options), and the Download dock.

import { useMemo, useState, type CSSProperties } from 'react';
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
  /** The backend's refusal of the last Download or Stop */
  actionError: string;
  /** A word about the last Download, for a moment ("Added to the queue") */
  note?: string;
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
  const { state, chapters, selected, followed, actionError, note = '' } = props;
  const { onFollow, onUnfollow, onOpenURL, onOptions, onChooseFolder, onError, onStart, onStartNew, onCancel, onDetails } = props;
  const host = hostOf(state.url);
  const title = state.title !== '' ? state.title : followed?.title ?? host;
  const letter = (title.trim()[0] ?? '?').toUpperCase();
  // a followed series' own cover, else the one fetched for the page (a series picked from the search)
  const cover = followed?.cover || state.cover;
  const now = new Date();
  // what the dock offers when nothing is picked: the chapters that came out since it was followed
  const fresh = newIndices(chapters, followed);
  // the picked chapters already saved in the chosen folder and format, which a
  // run of separate chapters skips (a bundle or volumes pack them all)
  const already = state.options.packAs === 'chapters' ? alreadyHere(chapters, selected, followed) : 0;

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
        <Progress
          count={state.loaded ? state.chapterCount : followed.chapters}
          chapters={state.loaded ? chapters : []}
          followed={followed}
          now={now}
        />
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
        note={note}
        chapters={chapters}
        selected={selected}
        fresh={fresh}
        already={already}
        onStart={onStart}
        onStartNew={() => onStartNew(fresh)}
        onCancel={onCancel}
        onDetails={onDetails}
      />
    </aside>
  );
}

/** How many of the selected chapters (indices) are saved here, as followed.saved says */
export function alreadyHere(chapters: Chapter[], selected: number[], followed: Followed | null): number {
  if (followed === null || followed.saved.length === 0) {
    return 0;
  }
  const saved = new Set(followed.saved);
  const picked = new Set(selected);
  return chapters.filter((c) => picked.has(c.index) && saved.has(c.number)).length;
}

/**
 * How far the user is with a followed series, at a glance: how many chapters
 * are saved in the folder chosen now, what's new, and a square per chapter.
 */
function Progress({ count, chapters, followed, now }: { count: number; chapters: Chapter[]; followed: Followed; now: Date }) {
  const listed = chapters.length > 0;
  const savedSet = new Set(followed.saved);
  const saved = listed ? chapters.filter((c) => savedSet.has(c.number)).length : followed.saved.length;
  const fresh = followed.new;
  const meta = [`${count.toLocaleString('en-US')} ${count === 1 ? 'chapter' : 'chapters'}`];
  if (followed.latest > 0) {
    meta.push(`latest ${formatNumber(followed.latest)}`);
  }
  if (followed.lastSaved !== '') {
    meta.push(`saved ${ago(followed.lastSaved, now)}`);
  }
  let pill: { text: string; tone: 'new' | 'done' } | null = null;
  if (fresh > 0) {
    pill = { text: `${fresh.toLocaleString('en-US')} new`, tone: 'new' };
  } else if (count > 0 && saved >= count) {
    pill = { text: 'All saved', tone: 'done' };
  } else if (saved > 0) {
    pill = { text: `${(count - saved).toLocaleString('en-US')} to go`, tone: 'done' };
  }
  const fraction = count > 0 ? Math.min(1, saved / count) : 0;
  return (
    <section className="progress-card" aria-label="Your progress">
      <div className="progress-top">
        <span className="progress-title">
          <strong className="num">{saved.toLocaleString('en-US')}</strong> of {count.toLocaleString('en-US')} saved
          {saved === 0 && <span className="progress-where"> in this folder</span>}
        </span>
        {pill !== null && <span className={pill.tone === 'new' ? 'pill-new' : 'pill-done'}>{pill.text}</span>}
      </div>
      {listed ? (
        <ChapterMap key={followed.url} chapters={chapters} followed={followed} />
      ) : (
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
      )}
      <p className="progress-meta">{meta.join(' · ')}</p>
    </section>
  );
}

/** How many squares a page of the chapter map holds: 4 rows of 25 */
export const MAP_PAGE = 100;

type Mark = 'saved' | 'new' | '';

/** Each chapter's mark, in chapter order: saved here, new, or neither */
export function chapterMarks(chapters: Chapter[], followed: Followed): { number: number; mark: Mark }[] {
  const saved = new Set(followed.saved);
  const fresh = new Set(newIndices(chapters, followed));
  return [...chapters]
    .sort((a, b) => a.number - b.number)
    .map((c) => ({ number: c.number, mark: saved.has(c.number) ? 'saved' : fresh.has(c.index) ? 'new' : '' }));
}

/** The page the map opens on: the first with a new chapter, else the first with one not saved, else the last */
export function startPage(marks: { mark: Mark }[]): number {
  const pages = Math.max(1, Math.ceil(marks.length / MAP_PAGE));
  const pageOf = (i: number) => (i < 0 ? -1 : Math.floor(i / MAP_PAGE));
  const fresh = pageOf(marks.findIndex((m) => m.mark === 'new'));
  if (fresh >= 0) {
    return fresh;
  }
  const todo = pageOf(marks.findIndex((m) => m.mark !== 'saved'));
  return todo >= 0 ? todo : pages - 1;
}

const MARK_NAMES: Record<Mark, string> = { saved: 'saved', new: 'new', '': 'not saved' };

/** A square per chapter, a page of MAP_PAGE at a time: saved, new or not saved */
function ChapterMap({ chapters, followed }: { chapters: Chapter[]; followed: Followed }) {
  const marks = useMemo(() => chapterMarks(chapters, followed), [chapters, followed]);
  const pages = Math.max(1, Math.ceil(marks.length / MAP_PAGE));
  const [page, setPage] = useState(() => startPage(marks));
  const at = Math.min(page, pages - 1);
  const shown = marks.slice(at * MAP_PAGE, (at + 1) * MAP_PAGE);
  const first = shown[0]?.number ?? 0;
  const last = shown[shown.length - 1]?.number ?? 0;
  return (
    <div className="chapter-map">
      {pages > 1 && (
        <div className="map-pager">
          <button type="button" className="btn ghost small icon-only" aria-label="Earlier chapters" disabled={at === 0} onClick={() => setPage(at - 1)}>
            <ChevronIcon dir="left" />
          </button>
          <span className="map-range">
            Chapters {formatNumber(first)}–{formatNumber(last)}
            <span className="map-page">
              {' '}
              · {at + 1} of {pages}
            </span>
          </span>
          <button
            type="button"
            className="btn ghost small icon-only"
            aria-label="Later chapters"
            disabled={at === pages - 1}
            onClick={() => setPage(at + 1)}
          >
            <ChevronIcon dir="right" />
          </button>
        </div>
      )}
      <div className="map-grid" role="img" aria-label={`Chapters ${formatNumber(first)} to ${formatNumber(last)}: saved, new and not saved`}>
        {shown.map((m) => (
          <span
            key={m.number}
            className={`map-sq${m.mark !== '' ? ` ${m.mark}` : ''}`}
            title={`Chapter ${formatNumber(m.number)} · ${MARK_NAMES[m.mark]}`}
          />
        ))}
      </div>
      <div className="map-key" aria-hidden="true">
        <span>
          <i className="map-sq saved" /> saved
        </span>
        <span>
          <i className="map-sq new" /> new
        </span>
        <span>
          <i className="map-sq" /> not saved
        </span>
      </div>
    </div>
  );
}

/** A small chevron pointing left or right */
function ChevronIcon({ dir }: { dir: 'left' | 'right' }) {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={dir === 'left' ? 'M15 6l-6 6 6 6' : 'M9 6l6 6-6 6'} />
    </svg>
  );
}
