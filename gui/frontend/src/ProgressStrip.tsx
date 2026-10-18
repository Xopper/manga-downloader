// The strip above the status bar: what's happening now. A running download
// (its title, how far it is, Stop), the downloads waiting their turn under
// it (each with a remove button), then how the last one ended (with Open
// folder) until dismissed or the next run.

import { plural, summaryHeadline } from './format';
import { CheckIcon, DownloadIcon, ErrorIcon, FolderIcon, Spinner, XIcon } from './icons';
import { overallProgress, summaryTone } from './run';
import type { QueuedJob, State } from './types';

interface Props {
  state: State;
  /** The finished run's line was dismissed */
  dismissed: boolean;
  /** The page shows the running download already (the series page's dock): the strip leaves it out, and shows only the queue */
  runShownElsewhere?: boolean;
  onDetails: () => void;
  onCancel: () => void;
  /** Drops a waiting download */
  onRemoveQueued: (id: number) => void;
  onOpenFolder: () => void;
  onDismiss: () => void;
}

/** How many waiting downloads the strip lists before "and N more" */
export const STRIP_QUEUE_ROWS = 3;

export function ProgressStrip({ state, dismissed, runShownElsewhere = false, onDetails, onCancel, onRemoveQueued, onOpenFolder, onDismiss }: Props) {
  if (state.running) {
    const queue = state.queue.length > 0 && (
      <QueueList queue={state.queue} limit={STRIP_QUEUE_ROWS} onRemove={onRemoveQueued} onMore={onDetails} />
    );
    if (runShownElsewhere) {
      return queue ? <div className="strip-block">{queue}</div> : null;
    }
    const overall = overallProgress(state);
    const pct = Math.round(overall.fraction * 100);
    const unit = state.run.volumes ? 'files' : 'chapters';
    return (
      <div className="strip-block">
        <div className="strip running" role="status" aria-label="Download in progress">
          <Spinner />
          <span className="strip-title">
            {state.cancelling
              ? 'Finishing the current chapters'
              : state.run.fetching
                ? `Fetching the chapter list of ${state.run.title}`
                : `Downloading ${state.run.title}`}
          </span>
          {!state.run.fetching && (
            <>
              <span className="muted">
                {state.run.bundle ? 'one file' : `${overall.finished} of ${overall.total} ${unit} done`}
              </span>
              <div className="bar strip-bar" role="progressbar" aria-label="Overall progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct}>
                <span style={{ transform: `scaleX(${overall.fraction})` }} />
              </div>
              <span className="strip-pct num">{pct}%</span>
            </>
          )}
          <span className="spacer" />
          <button type="button" className="btn small ghost" onClick={onDetails}>
            Details
          </button>
          <button type="button" className="btn small danger-outline" aria-disabled={state.cancelling || undefined} onClick={() => !state.cancelling && onCancel()}>
            <XIcon size={13} /> {state.cancelling ? 'Stopping…' : 'Stop'}
          </button>
        </div>
        {queue}
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
      <span className="strip-title">{state.run.title !== '' ? `${state.run.title}: ` : ''}{summaryHeadline(summary)}</span>
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

interface QueueProps {
  queue: QueuedJob[];
  /** How many rows to list; the rest is "and N more" (opening onMore) */
  limit?: number;
  onRemove: (id: number) => void;
  onMore?: () => void;
}

/** The downloads waiting their turn, in order, each with a remove button */
export function QueueList({ queue, limit = Infinity, onRemove, onMore }: QueueProps) {
  if (queue.length === 0) {
    return null;
  }
  const shown = queue.slice(0, limit);
  const more = queue.length - shown.length;
  return (
    <ol className="strip-queue" aria-label="Waiting downloads">
      {shown.map((job, i) => (
        <li key={job.id}>
          <span className="queue-pos num" aria-hidden="true">
            {i + 1}
          </span>
          <span className="queue-title">{job.title}</span>
          <span className="muted">{queuedCount(job)}</span>
          <button type="button" className="btn small ghost icon-only" aria-label={`Remove ${job.title} from the queue`} title="Remove from the queue" onClick={() => onRemove(job.id)}>
            <XIcon size={12} />
          </button>
        </li>
      ))}
      {more > 0 && (
        <li className="queue-more">
          <span className="queue-pos" aria-hidden="true" />
          {onMore ? (
            <button type="button" className="btn small ghost" onClick={onMore}>
              and {plural(more, 'more download', 'more downloads')}
            </button>
          ) : (
            <span className="muted">and {plural(more, 'more download', 'more downloads')}</span>
          )}
        </li>
      )}
    </ol>
  );
}

/** What a waiting job downloads: its chapters, or the new ones of a followed series */
export function queuedCount(job: QueuedJob): string {
  if (job.fresh) {
    return plural(job.chapters, 'new chapter', 'new chapters');
  }
  return plural(job.chapters, 'chapter', 'chapters');
}
