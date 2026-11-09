// Owner: U2 (detail). Guided first-run tour of the project page: the
// "New to Nebi projects?" prompt, then one card per step next to the
// highlighted element. Start it again from Help ("Take the page tour").
import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { cn } from '@nebi/ui/lib/utils';
import { X } from 'lucide-react';
import {
  type CSSProperties,
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { PROJECT_TOUR_STEPS, type TourStep, tourTarget } from './tourSteps';
import { useTourStore } from './tourStore';

const CARD_WIDTH = 320;
const GAP = 12;
const MARGIN = 16;

/** Mount once on the project page: prompt for new viewers, then the tour. */
export const ProjectTour = () => {
  const active = useTourStore((s) => s.active);
  const seen = useTourStore((s) => s.seen);
  const start = useTourStore((s) => s.start);
  const stop = useTourStore((s) => s.stop);
  const dismissPrompt = useTourStore((s) => s.dismissPrompt);

  // Leaving the page ends a running tour.
  const activeRef = useRef(active);
  useEffect(() => {
    activeRef.current = active;
  }, [active]);
  useEffect(
    () => () => {
      if (activeRef.current) stop();
    },
    [stop],
  );

  if (active) return <TourSteps onClose={stop} />;
  if (seen) return null;
  return <TourPrompt onStart={start} onDismiss={dismissPrompt} />;
};

const TourPrompt = ({
  onStart,
  onDismiss,
}: {
  onStart: () => void;
  onDismiss: () => void;
}) => (
  // Not modal: the prompt is an invitation, the page behind it stays
  // readable and usable (and announced) until the viewer decides.
  <Dialog
    open
    modal={false}
    onOpenChange={(open) => {
      if (!open) onDismiss();
    }}
  >
    <DialogContent className="max-w-[400px]" finalFocus={false}>
      <DialogHeader>
        <DialogTitle>New to Nebi projects?</DialogTitle>
        <DialogDescription>
          Take a 1-minute tour of this page: versions, installing, sharing and
          publishing.
        </DialogDescription>
      </DialogHeader>
      <p className="text-muted-foreground text-sm">
        Not now? You can start it any time from Help.
      </p>
      <DialogFooter>
        <DialogClose render={<Button variant="secondary" />}>Skip</DialogClose>
        <Button onClick={onStart}>Start tour</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
);

const TourSteps = ({ onClose }: { onClose: () => void }) => {
  // Steps whose target is on the page when the tour starts.
  const steps = useMemo(
    () => PROJECT_TOUR_STEPS.filter((s) => tourTarget(s) !== null),
    [],
  );
  const counted = steps.filter((s) => !s.finish).length;
  const [index, setIndex] = useState(0);
  const step = steps[index];
  const returnFocus = useRef<Element | null>(document.activeElement);

  const close = useCallback(() => {
    onClose();
    const prev = returnFocus.current;
    const target =
      prev instanceof HTMLElement && prev.isConnected
        ? prev
        : document.querySelector<HTMLElement>('[data-tour="help"]');
    target?.focus();
  }, [onClose]);

  useEffect(() => {
    if (steps.length === 0) close();
  }, [steps.length, close]);

  if (!step) return null;
  return (
    <TourCard
      key={step.id}
      step={step}
      counter={step.finish ? null : `${index + 1} of ${counted}`}
      canGoBack={index > 0}
      onNext={() => (index < steps.length - 1 ? setIndex(index + 1) : close())}
      onBack={() => setIndex(Math.max(0, index - 1))}
      onClose={close}
    />
  );
};

interface Rect {
  top: number;
  left: number;
  width: number;
  height: number;
}

const TourCard = ({
  step,
  counter,
  canGoBack,
  onNext,
  onBack,
  onClose,
}: {
  step: TourStep;
  counter: string | null;
  canGoBack: boolean;
  onNext: () => void;
  onBack: () => void;
  onClose: () => void;
}) => {
  const titleId = useId();
  const bodyId = useId();
  const cardRef = useRef<HTMLDivElement>(null);
  const nextRef = useRef<HTMLButtonElement>(null);
  const [highlight, setHighlight] = useState<Rect | null>(null);
  const [position, setPosition] = useState<{
    top: number;
    left: number;
  } | null>(null);

  const measure = useCallback(() => {
    const target = tourTarget(step);
    const card = cardRef.current;
    if (!target || !card) return;
    const r = target.getBoundingClientRect();
    const pad = step.pad ?? 6;
    const h: Rect = {
      top: r.top - pad,
      left: r.left - pad,
      width: r.width + pad * 2,
      height: r.height + pad * 2,
    };
    setHighlight(h);
    const cardHeight = card.offsetHeight;
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let top: number;
    let left: number;
    if (step.placement === 'right') {
      left = h.left + h.width + GAP;
      top = h.top;
    } else {
      left =
        step.placement === 'bottom-end'
          ? h.left + h.width - CARD_WIDTH
          : h.left;
      top = h.top + h.height + GAP;
      if (top + cardHeight > vh - MARGIN) top = h.top - GAP - cardHeight;
    }
    setPosition({
      left: Math.min(Math.max(left, MARGIN), vw - CARD_WIDTH - MARGIN),
      top: Math.min(Math.max(top, MARGIN), vh - cardHeight - MARGIN),
    });
  }, [step]);

  useLayoutEffect(() => {
    tourTarget(step)?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
    measure();
    let frame = 0;
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(measure);
    };
    window.addEventListener('resize', schedule);
    window.addEventListener('scroll', schedule, true);
    const observer =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver(schedule);
    observer?.observe(document.body);
    if (cardRef.current) observer?.observe(cardRef.current);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener('resize', schedule);
      window.removeEventListener('scroll', schedule, true);
      observer?.disconnect();
    };
  }, [measure, step]);

  const placed = position !== null;
  useEffect(() => {
    if (placed) nextRef.current?.focus();
  }, [placed]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      else if (e.key === 'ArrowRight') onNext();
      else if (e.key === 'ArrowLeft' && canGoBack) onBack();
      else return;
      e.preventDefault();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose, onNext, onBack, canGoBack]);

  const trapFocus = (e: React.KeyboardEvent) => {
    if (e.key !== 'Tab' || !cardRef.current) return;
    const buttons = [
      ...cardRef.current.querySelectorAll<HTMLButtonElement>(
        'button:not([disabled])',
      ),
    ];
    const first = buttons[0];
    const last = buttons[buttons.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last?.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first?.focus();
    }
  };

  return (
    <>
      <div className="fixed inset-0 z-50" aria-hidden />
      {highlight && (
        <div
          aria-hidden
          data-testid="tour-highlight"
          className="pointer-events-none fixed z-50 rounded-lg shadow-[0_0_0_9999px_var(--scrim)] ring-2 ring-primary motion-safe:animate-fade-in"
          style={highlight as CSSProperties}
        />
      )}
      <div
        ref={cardRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={bodyId}
        onKeyDown={trapFocus}
        className={cn(
          'fixed z-50 w-80 rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10',
          placed ? 'motion-safe:animate-slide-up-fade' : 'invisible',
        )}
        style={position ?? { top: 0, left: 0 }}
      >
        <div className="flex flex-col gap-2 p-3">
          <div className="flex items-center justify-between gap-2">
            <h2 id={titleId} className="font-medium text-foreground text-sm">
              {step.title}
            </h2>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onClose}
              aria-label="End tour"
            >
              <X />
            </Button>
          </div>
          <p id={bodyId} className="text-muted-foreground-strong text-sm">
            {step.body}
          </p>
          <div className="flex items-center justify-between gap-2 pt-1">
            {counter && (
              <span className="text-muted-foreground text-xs">{counter}</span>
            )}
            <div className="ml-auto flex gap-2">
              {canGoBack && (
                <Button variant="secondary" size="xs" onClick={onBack}>
                  Back
                </Button>
              )}
              <Button ref={nextRef} size="xs" onClick={onNext}>
                {step.finish ? 'Done' : 'Next'}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};
