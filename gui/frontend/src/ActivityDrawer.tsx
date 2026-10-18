// The run's details, as a drawer over the window: the Activity panel (the
// summary and one row per chapter), then the downloads waiting their turn,
// opened from the progress strip.

import { ActivityPanel } from './ActivityPanel';
import { plural } from './format';
import { XIcon } from './icons';
import { QueueList } from './ProgressStrip';
import type { State } from './types';

interface Props {
  state: State;
  onOpenFolder: () => void;
  /** Drops a waiting download */
  onRemoveQueued: (id: number) => void;
  onClose: () => void;
}

export function ActivityDrawer({ state, onOpenFolder, onRemoveQueued, onClose }: Props) {
  const title = state.run.title !== '' ? ` · ${state.run.title}` : '';
  return (
    <section
      className="log-drawer drawer activity-drawer"
      role="dialog"
      aria-modal="true"
      aria-label="Download details"
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          onClose();
        }
      }}
    >
      <div className="log-head">
        <h2>{state.running ? `This download${title}` : `Last download${title}`}</h2>
        <button type="button" className="btn ghost small icon-only" aria-label="Close the details" onClick={onClose} autoFocus>
          <XIcon size={14} />
        </button>
      </div>
      <div className="drawer-body activity-body">
        <ActivityPanel state={state} onOpenFolder={onOpenFolder} />
        {state.queue.length > 0 && (
          <section className="activity-queue" aria-label="Waiting downloads">
            <h3 className="jobs-title">
              Waiting <span className="muted"> · {plural(state.queue.length, 'download', 'downloads')}, first to start first</span>
            </h3>
            <QueueList queue={state.queue} onRemove={onRemoveQueued} />
          </section>
        )}
      </div>
    </section>
  );
}
