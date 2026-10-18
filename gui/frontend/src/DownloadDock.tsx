// The foot of the side panel, always in view: the one big action. Download
// when idle (with the reason it can't start, if any); while this series
// downloads, the run's overall progress and Stop. While another series
// downloads, Download still works: the chapters join the queue and start
// when their turn comes.

import { useEffect, useMemo, useRef } from 'react';
import { plural } from './format';
import { DownloadIcon, Spinner, XIcon } from './icons';
import { downloadBlocker, downloadLabel, overallProgress, runningHere } from './run';
import type { Chapter, Options, PackAs, State } from './types';

interface Props {
  state: State;
  /** The backend's refusal of the last Download or Stop */
  error: string;
  /** A word about the last Download, for a moment: "Added to the queue" */
  note?: string;
  /** The loaded chapters and the selected indices, for the Download label's volumes */
  chapters: Chapter[];
  selected: number[];
  /** The followed series' new chapters, which the button offers when nothing is picked */
  fresh: number[];
  /** How many picked chapters are already in the folder, in the format, chosen: the run skips them */
  already?: number;
  onStart: () => void;
  /** Picks the new chapters and starts */
  onStartNew: () => void;
  onCancel: () => void;
  /** Opens the per-chapter activity of the running download */
  onDetails?: () => void;
}

export function DownloadDock({ state, error, note = '', chapters, selected, fresh, already = 0, onStart, onStartNew, onCancel, onDetails }: Props) {
  const here = runningHere(state);
  // Download and Stop are different buttons, so the focus moves to the other
  // one when the run starts or ends, rather than falling to the body
  const focused = useRef<'start' | 'cancel' | null>(null);
  const startButton = useRef<HTMLButtonElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (here && focused.current === 'start') {
      cancelButton.current?.focus();
    } else if (!here && focused.current === 'cancel') {
      if (startButton.current?.disabled) {
        // the run ended with nothing to download: focus the chapter list instead of the body
        document.querySelector<HTMLElement>('[role="listbox"]')?.focus();
      } else {
        startButton.current?.focus();
      }
    }
  }, [here]);

  // the label regroups the chapters, so it's kept across the progress ticks
  const label = useMemo(
    () => downloadLabel(state, chapters, selected),
    [state.selectedCount, state.options.packAs, state.volumes.text, chapters, selected],
  );

  if (here) {
    const overall = overallProgress(state);
    const pct = Math.round(overall.fraction * 100);
    // more chapters picked meanwhile join the queue after this run
    const more = !state.loading && !state.stale && state.selectedCount > 0;
    return (
      <div className="dock running">
        <div className="dock-progress">
          <div className="dock-line">
            <span className="dock-title">
              <Spinner />
              {state.cancelling
                ? 'Finishing the current chapters'
                : state.run.fetching
                  ? 'Fetching the chapter list'
                  : state.run.bundle
                    ? 'Building the bundle'
                    : `${overall.finished} of ${overall.total} ${state.run.volumes ? 'files' : 'chapters'} done`}
            </span>
            <span className="dock-pct">{pct}%</span>
          </div>
          <div
            className="bar big"
            role="progressbar"
            aria-label="Overall progress"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={pct}
          >
            <span style={{ transform: `scaleX(${overall.fraction})` }} />
          </div>
          {state.cancelling && <p className="hint">Chapters already downloading will finish; no new ones start.</p>}
        </div>
        <div className="dock-actions">
        {onDetails && (
          <button type="button" className="btn ghost" onClick={onDetails}>
            Details
          </button>
        )}
        {/* aria-disabled, not disabled: a disabled button drops its focus, and Stop's focus must survive to the run's end */}
        <button
          type="button"
          ref={cancelButton}
          className="btn danger-outline block"
          aria-disabled={state.cancelling || undefined}
          onFocus={() => (focused.current = 'cancel')}
          onBlur={() => (focused.current = null)}
          onClick={() => {
            if (!state.cancelling) {
              onCancel();
            }
          }}
        >
          <XIcon /> {state.cancelling ? 'Stopping…' : 'Stop'}
        </button>
        </div>
        {more && (
          <button type="button" className="btn block" onClick={onStart}>
            <DownloadIcon size={14} /> Download {plural(state.selectedCount, 'more chapter', 'more chapters')} after this
          </button>
        )}
        <DockMessage error={error} note={note} />
      </div>
    );
  }

  // nothing picked but new chapters waiting: the button gets them, rather
  // than sitting there dead
  const nothingPicked = state.loaded && !state.loading && !state.stale && state.selectedCount === 0;
  const quick = nothingPicked && fresh.length > 0 && downloadBlocker({ ...state, selectedCount: fresh.length }) === '';
  // the picked chapters already in this folder, as this format, are skipped
  const skipping = quick || nothingPicked ? 0 : Math.min(already, state.selectedCount);
  const allThere = skipping > 0 && skipping === state.selectedCount;
  let blocker = quick ? '' : downloadBlocker(state);
  if (blocker === '' && allThere) {
    blocker = `${skipping === 1 ? "It's" : "They're all"} in this folder already. Pick others, or choose another folder or format to save ${skipping === 1 ? 'it' : 'them'} again.`;
  }
  const count = (quick ? fresh.length : state.selectedCount) - skipping;
  const o = state.options;
  let text = label;
  if (quick) {
    text = `Download ${plural(fresh.length, 'new chapter', 'new chapters')}`;
  } else if (nothingPicked) {
    text = 'Pick chapters to download';
  } else if (allThere) {
    text = 'Already in this folder';
  } else if (skipping > 0) {
    text = `Download ${plural(count, 'chapter', 'chapters')}`;
  }
  // the label says it already: no hint repeating it underneath
  let hint = quick ? 'Nothing picked: this gets the chapters that came out since you followed it.' : nothingPicked ? '' : blocker;
  if (hint === '' && skipping > 0) {
    hint = `${plural(skipping, 'picked chapter is', 'picked chapters are')} already in this folder, so ${skipping === 1 ? "it's" : "they're"} skipped.`;
  }
  // another series downloads: these wait their turn
  if (hint === '' && state.running && count > 0) {
    hint =
      state.queue.length > 0
        ? `${state.run.title} is downloading, with ${plural(state.queue.length, 'download', 'downloads')} waiting: these start after them.`
        : `${state.run.title} is downloading: these start once it finishes.`;
  }
  return (
    <div className="dock">
      {count > 0 && (
        <p className="dock-dest" title={`Saved in ${o.outputDir}`}>
          {destination(o, count)}
        </p>
      )}
      <button
        type="button"
        ref={startButton}
        className="btn primary block big"
        disabled={blocker !== ''}
        aria-describedby={hint !== '' ? 'dock-blocker' : undefined}
        onFocus={() => (focused.current = 'start')}
        onBlur={() => (focused.current = null)}
        onClick={quick ? onStartNew : onStart}
      >
        {(!nothingPicked || quick) && !allThere ? <DownloadIcon /> : null} {text}
      </button>
      {error !== '' ? (
        <p className="error-text small" role="alert">
          Can't download: {error}
        </p>
      ) : note !== '' ? (
        <p className="hint center dock-note" role="status">
          {note}
        </p>
      ) : (
        hint !== '' && (
          <p className="hint center" id="dock-blocker">
            {hint}
          </p>
        )
      )}
    </div>
  );
}

/** The refusal, else the note, under the running dock's buttons */
function DockMessage({ error, note }: { error: string; note: string }) {
  if (error !== '') {
    return (
      <p className="error-text small" role="alert">
        {error}
      </p>
    );
  }
  if (note !== '') {
    return (
      <p className="hint center dock-note" role="status">
        {note}
      </p>
    );
  }
  return null;
}

/** What the run writes, by format: one file of a chapter, several, a volume's, the bundle */
const DESTINATIONS: Record<Options['format'], { one: string; many: string; volume: string; bundle: string }> = {
  cbz: { one: 'CBZ file', many: 'CBZ files', volume: 'A CBZ per volume', bundle: 'One CBZ' },
  epub: { one: 'EPUB file', many: 'EPUB files', volume: 'An EPUB per volume', bundle: 'One EPUB' },
  raw: { one: 'image folder', many: 'image folders', volume: 'A folder per volume', bundle: 'One image folder' },
};

/** What the run of n chapters writes, over the button; the folder is in the Save options above */
function destination(o: Options, n: number): string {
  const d = DESTINATIONS[o.format] ?? DESTINATIONS.cbz;
  const packAs: PackAs = o.packAs;
  if (packAs === 'volumes') {
    return d.volume;
  }
  if (packAs === 'bundle' && n > 1) {
    return `${d.bundle}, ${n} chapters`;
  }
  return plural(n, d.one, d.many);
}
