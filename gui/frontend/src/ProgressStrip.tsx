// The strip above the status bar: what's happening now. A running download
// (its title, how far it is, Cancel), then how it ended (with Open folder)
// until dismissed or the next run.

import { summaryHeadline } from './format';
import { CheckIcon, DownloadIcon, ErrorIcon, FolderIcon, Spinner, XIcon } from './icons';
import { overallProgress, summaryTone } from './run';
import type { State } from './types';

interface Props {
  state: State;
  /** The finished run's line was dismissed */
  dismissed: boolean;
  onDetails: () => void;
  onCancel: () => void;
  onOpenFolder: () => void;
  onDismiss: () => void;
}

export function ProgressStrip({ state, dismissed, onDetails, onCancel, onOpenFolder, onDismiss }: Props) {
  if (state.running) {
    const overall = overallProgress(state);
    const pct = Math.round(overall.fraction * 100);
    const unit = state.run.volumes ? 'files' : 'chapters';
    return (
      <div className="strip running" role="status" aria-label="Download in progress">
        <Spinner />
        <span className="strip-title">
          {state.cancelling ? 'Finishing the current chapters' : `Downloading ${state.title}`}
        </span>
        <span className="muted">
          {state.run.bundle ? 'one file' : `${overall.finished} of ${overall.total} ${unit} done`}
        </span>
        <div className="bar strip-bar" role="progressbar" aria-label="Overall progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct}>
          <span style={{ transform: `scaleX(${overall.fraction})` }} />
        </div>
        <span className="strip-pct num">{pct}%</span>
        <span className="spacer" />
        <button type="button" className="btn small ghost" onClick={onDetails}>
          Details
        </button>
        <button type="button" className="btn small danger-outline" aria-disabled={state.cancelling || undefined} onClick={() => !state.cancelling && onCancel()}>
          <XIcon size={13} /> {state.cancelling ? 'Cancelling…' : 'Cancel'}
        </button>
      </div>
    );
  }

  const summary = state.summary;
  if (summary === null || dismissed) {
    return null;
  }
  const tone = summaryTone(summary);
  return (
    <div className={`strip done tone-${tone}`} role="status" aria-label="Last download">
      {tone === 'success' ? <CheckIcon size={15} /> : tone === 'danger' ? <ErrorIcon size={15} /> : <DownloadIcon size={15} />}
      <span className="strip-title">{state.title !== '' ? `${state.title}: ` : ''}{summaryHeadline(summary)}</span>
      <span className="spacer" />
      <button type="button" className="btn small" onClick={onOpenFolder}>
        <FolderIcon size={13} /> Open folder
      </button>
      <button type="button" className="btn small ghost" onClick={onDetails}>
        Details
      </button>
      <button type="button" className="btn small ghost icon-only" aria-label="Dismiss" onClick={onDismiss}>
        <XIcon size={13} />
      </button>
    </div>
  );
}
