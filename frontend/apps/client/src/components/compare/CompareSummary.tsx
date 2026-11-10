// Owner: U4 (compare). "+1 −2 ~0" or "No changes" in the picker bar.
import type { DiffCounts } from '@/types/contracts';
import { CHANGE_TEXT } from './changeStyles';

/**
 * specChanged: channels or platforms differ, so the versions are not the
 * same even when no requested package changed (the counts are shown then).
 */
export const CompareSummary = ({
  counts,
  specChanged = false,
}: {
  counts: DiffCounts;
  specChanged?: boolean;
}) => {
  const changed =
    specChanged || counts.added + counts.removed + counts.changed > 0;
  return (
    <output
      className="ml-auto flex items-center gap-2 font-medium text-sm leading-5"
      aria-label="Summary of changes"
    >
      {changed ? (
        <>
          <span className={CHANGE_TEXT.added} title="Added">
            +{counts.added}
          </span>
          <span className={CHANGE_TEXT.removed} title="Removed">
            −{counts.removed}
          </span>
          <span className={CHANGE_TEXT.changed} title="Changed">
            ~{counts.changed}
          </span>
        </>
      ) : (
        <span className="text-muted-foreground">No changes</span>
      )}
    </output>
  );
};
