// The foot of the side panel, always in view: the one big action. Download
// when idle (with the reason it can't start, if any), the run's overall
// progress and Cancel while it runs.

import { useEffect, useMemo, useRef } from 'react';
import { plural } from './format';
import { DownloadIcon, Spinner, XIcon } from './icons';
import { downloadBlocker, downloadLabel, overallProgress } from './run';
import type { Chapter, Options, PackAs, State } from './types';

interface Props {
  state: State;
  /** The backend's refusal of the last Download or Cancel */
  error: string;
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

export function DownloadDock({ state, error, chapters, selected, fresh, already = 0, onStart, onStartNew, onCancel, onDetails }: Props) {
  // Download and Cancel are different buttons, so the focus moves to the other
  // one when the run starts or ends, rather than falling to the body
  const focused = useRef<'start' | 'cancel' | null>(null);
  const startButton = useRef<HTMLButtonElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (state.running && focused.current === 'start') {
      cancelButton.current?.focus();
    } else if (!state.running && focused.current === 'cancel') {
      if (startButton.current?.disabled) {
        // the run ended with nothing to download: focus the chapter list instead of the body
        document.querySelector<HTMLElement>('[role="listbox"]')?.focus();
      } else {
        startButton.current?.focus();
      }
    }
  }, [state.running]);

  // the label regroups the chapters, so it's kept across the progress ticks
  const label = useMemo(
    () => downloadLabel(state, chapters, selected),
    [state.selectedCount, state.options.packAs, state.volumes.text, chapters, selected],
  );

  if (state.running) {
    const overall = overallProgress(state);
    const pct = Math.round(overall.fraction * 100);
    return (
      <div className="dock running">
        <div className="dock-progress">
          <div className="dock-line">
            <span className="dock-title">
              <Spinner />
              {state.cancelling
                ? 'Finishing the current chapters'
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
        {/* aria-disabled, not disabled: a disabled button drops its focus, and Cancel's focus must survive to the run's end */}
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
          <XIcon /> {state.cancelling ? 'Cancelling…' : 'Cancel'}
        </button>
        </div>
        {error !== '' && (
          <p className="error-text small" role="alert">
            {error}
          </p>
        )}
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
