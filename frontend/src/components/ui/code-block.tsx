import { cva, type VariantProps } from 'class-variance-authority';
import { CheckIcon, CopyIcon } from 'lucide-react';
import type * as React from 'react';
import {
  createContext,
  Fragment,
  useCallback,
  useContext,
  useState,
} from 'react';
import { cn } from '@/lib/utils';

interface CodeBlockContextValue {
  /** The raw snippet, shared so descendants don't re-thread the text. */
  code: string;
  /** Whether the body renders a line-number gutter. */
  showLineNumbers: boolean;
  /**
   * Whether the root renders a floating copy button over the body. The body
   * reads this to reserve top-right clearance so code can't slide under it on
   * a narrow, header-less block.
   */
  hasFloatingCopyButton: boolean;
}

const CodeBlockContext = createContext<CodeBlockContextValue | null>(null);

function useCodeBlockContext(component: string): CodeBlockContextValue {
  const context = useContext(CodeBlockContext);
  if (!context) {
    throw new Error(`<${component}> must be used within a <CodeBlock>.`);
  }
  return context;
}

type CodeBlockProps = React.ComponentProps<'div'> & {
  /** Snippet rendered by {@link CodeBlockBody} and copied by the copy button. */
  code: string;
  /** Render a non-selectable, aria-hidden line-number gutter in the body. */
  showLineNumbers?: boolean;
  /**
   * Render a floating copy button in the top-right corner. On by default so a
   * bare block still offers copy. Set to `false` when composing your own
   * {@link CodeBlockCopyButton} inside a {@link CodeBlockHeader}, to avoid two.
   */
  showCopyButton?: boolean;
  /**
   * Force the dark palette for this block regardless of the surrounding theme.
   * Applies the theme's `.dark` class to the root so its semantic tokens
   * (`bg-card`, `text-foreground`, …) resolve against the dark collection.
   */
  dark?: boolean;
};

/**
 * CodeBlock frames a formatted, monospaced snippet. It shares the snippet with
 * its descendants via context, so {@link CodeBlockBody} and
 * {@link CodeBlockCopyButton} read the text without it being re-threaded.
 * Compose it with an optional {@link CodeBlockHeader} (language/filename label),
 * a {@link CodeBlockBody}, and a {@link CodeBlockCopyButton}.
 *
 * Syntax highlighting is intentionally out of scope — the body renders plain
 * monospaced text.
 */
function CodeBlock({
  className,
  code,
  showLineNumbers = false,
  showCopyButton = true,
  dark = false,
  children,
  ...props
}: CodeBlockProps) {
  return (
    <CodeBlockContext.Provider
      value={{ code, showLineNumbers, hasFloatingCopyButton: showCopyButton }}
    >
      <div
        data-slot="code-block"
        data-dark={dark || undefined}
        className={cn(
          // `dark` remaps the semantic tokens below to the dark palette for
          // this subtree, so the block can be dark on an otherwise light page.
          dark && 'dark',
          // `relative` anchors the floating copy button to this root.
          // `w-[clamp(...)]` makes the block grow with the viewport between a
          // 20rem floor and a 48rem ceiling, so typical-length lines fit
          // without a horizontal scrollbar; only genuinely long lines fall
          // back to the body's `overflow-x-auto`. Override via `className` for
          // a fixed (`w-[28rem]`) or fluid (`w-full`) width.
          // `overflow-hidden` also zeroes the automatic min-width in flex/grid
          // parents, so a narrow layout squeezes the block (and the body
          // scrolls) instead of the longest line blowing out the page.
          // `min-w-40` is the floor below which the frame stops being usable.
          'relative w-[clamp(20rem,80vw,48rem)] min-w-40 overflow-hidden rounded-md border border-border bg-card font-mono text-sm text-foreground',
          className,
        )}
        {...props}
      >
        {showCopyButton ? <CodeBlockCopyButton floating /> : null}
        {children}
      </div>
    </CodeBlockContext.Provider>
  );
}

/**
 * Header bar for a {@link CodeBlock}, typically holding a language/filename
 * label and a {@link CodeBlockCopyButton}.
 */
function CodeBlockHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="code-block-header"
      className={cn(
        'flex items-center justify-between gap-2 border-b border-border bg-muted px-4 py-2 text-xs text-muted-foreground-strong',
        className,
      )}
      {...props}
    />
  );
}

type CodeBlockBodyProps = React.ComponentProps<'section'> & {
  /**
   * Cap the body at this many whole code lines; longer snippets scroll
   * vertically. The cap is measured in lines (via the CSS `lh` unit) rather
   * than a fixed height, so the block always crops at a line boundary instead
   * of slicing a line in half.
   */
  maxLines?: number;
};

/**
 * Renders the snippet from context inside a scrollable `pre`/`code`. Lines
 * never soft-wrap — overflow scrolls horizontally instead. When the root's
 * `showLineNumbers` is set, a right-aligned gutter is rendered per line;
 * the gutter is `aria-hidden` and non-selectable so copied or selected text
 * stays free of line numbers.
 */
function CodeBlockBody({
  className,
  style,
  maxLines,
  ...props
}: CodeBlockBodyProps) {
  const { code, showLineNumbers, hasFloatingCopyButton } =
    useCodeBlockContext('CodeBlockBody');
  const lines = code.split('\n');

  // Reserve right-edge clearance for the floating copy button (`right-3` +
  // `size-7`) so line-end content can't tuck under it when the line fits.
  // `pr-12` (3rem) clears the button's 2.5rem footprint with room to spare; it
  // overrides the `px-4` right padding via tailwind-merge. A line long enough
  // to overflow instead scrolls horizontally *behind* the button, whose opaque
  // `bg-card` keeps it legible over the moving text.
  const copyButtonClearance = hasFloatingCopyButton ? 'pr-12' : undefined;

  return (
    // A labelled `section` names the focus stop for screen readers; the label
    // is overridable via the props spread below.
    <section
      data-slot="code-block-body"
      data-max-lines={maxLines}
      aria-label="Code"
      // The body is a scroll container, so keyboard users must be able to
      // focus it to scroll (axe: scrollable-region-focusable).
      // biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable region must be keyboard-focusable
      tabIndex={0}
      className={cn(
        // `leading-relaxed` lives here (not on the `pre`) so this element's
        // `1lh` equals exactly one rendered code line for the max-height calc
        // below; the `pre` and every line span inherit the same line-height.
        // The ring is inset because the root's `overflow-hidden` would clip
        // an outer one.
        'overflow-x-auto leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
        // The 1rem term mirrors the `pre`'s top padding (`py-4`) — the only
        // padding on screen at the initial scroll position. The bottom padding
        // scrolls with the content, so counting it too would leave a padding's
        // worth of the next line peeking in. Because the paddings are equal,
        // exactly `maxLines` whole lines are visible at both scroll ends.
        maxLines != null &&
          'max-h-[calc(1rem+var(--code-block-max-lines)*1lh)] overflow-y-auto',
        className,
      )}
      style={
        maxLines != null
          ? ({
              '--code-block-max-lines': maxLines,
              ...style,
            } as React.CSSProperties)
          : style
      }
      {...props}
    >
      <pre className="py-4">
        {showLineNumbers ? (
          // One grid for the whole snippet — not one per line — so the `auto`
          // gutter column sizes to the widest line number and the code column
          // stays put when the count crosses 9→10, 99→100, and so on.
          <code
            className={cn(
              'grid grid-cols-[auto_1fr] gap-x-4 px-4',
              copyButtonClearance,
            )}
          >
            {lines.map((line, index) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: source lines have no stable id and never reorder
              <Fragment key={index}>
                <span
                  aria-hidden="true"
                  className="select-none text-right tabular-nums text-muted-foreground"
                >
                  {index + 1}
                </span>
                <span className="whitespace-pre">{line || ' '}</span>
              </Fragment>
            ))}
          </code>
        ) : (
          <code className="grid">
            <span className={cn('whitespace-pre px-4', copyButtonClearance)}>
              {code}
            </span>
          </code>
        )}
      </pre>
    </section>
  );
}

const codeBlockCopyButtonVariants = cva(
  'inline-flex shrink-0 items-center justify-center rounded-md text-muted-foreground outline-none motion-safe:transition-[color,background-color,opacity,transform] motion-safe:duration-(--duration-fast) motion-safe:ease-(--ease-standard) motion-safe:active:scale-[0.97] hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring [&_svg]:pointer-events-none',
  {
    variants: {
      size: {
        sm: "size-6 [&_svg:not([class*='size-'])]:size-3.5",
        default: "size-7 [&_svg:not([class*='size-'])]:size-4",
      },
      // Overlay the button in a header-less block. It pins to the top-right,
      // offset to sit level with the first code line, and works at any block
      // height (a header-less snippet keeps its copy affordance without a bar).
      // `bg-card` (matching the frame) makes it opaque so a long line that
      // overflows and scrolls horizontally passes *behind* it — the icon stays
      // legible instead of mixing with the code beneath. `shadow-sm` lifts it
      // off that scrolling text.
      floating: {
        true: 'absolute right-3 top-3 bg-card shadow-sm',
        false: '',
      },
    },
    defaultVariants: {
      size: 'default',
      floating: false,
    },
  },
);

type CodeBlockCopyButtonProps = Omit<
  React.ComponentProps<'button'>,
  'children'
> &
  VariantProps<typeof codeBlockCopyButtonVariants> & {
    /** Accessible label in the resting state. */
    copyLabel?: string;
    /** Accessible label while the confirmed state is showing. */
    copiedLabel?: string;
  };

/**
 * Copies the {@link CodeBlock}'s snippet to the clipboard and briefly swaps to a
 * check icon to confirm. Failure paths (insecure context, denied permission)
 * leave the button in its resting state rather than showing a false confirm.
 */
function CodeBlockCopyButton({
  className,
  size,
  floating,
  onClick,
  copyLabel = 'Copy code',
  copiedLabel = 'Copied',
  ...props
}: CodeBlockCopyButtonProps) {
  const { code } = useCodeBlockContext('CodeBlockCopyButton');
  const [copied, setCopied] = useState(false);

  const handleClick = useCallback(
    async (event: React.MouseEvent<HTMLButtonElement>) => {
      onClick?.(event);
      try {
        await navigator.clipboard.writeText(code);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      } catch {
        // Insecure context or a denied permission rejects writeText — keep the
        // button resting rather than flashing a false confirmation.
        setCopied(false);
      }
    },
    [code, onClick],
  );

  return (
    <button
      type="button"
      data-slot="code-block-copy-button"
      data-size={size ?? 'default'}
      data-floating={floating || undefined}
      data-copied={copied || undefined}
      aria-label={copied ? copiedLabel : copyLabel}
      className={cn(codeBlockCopyButtonVariants({ size, floating }), className)}
      onClick={handleClick}
      {...props}
    >
      {copied ? (
        <CheckIcon aria-hidden="true" />
      ) : (
        <CopyIcon aria-hidden="true" />
      )}
    </button>
  );
}

export type { CodeBlockBodyProps, CodeBlockCopyButtonProps, CodeBlockProps };
export {
  CodeBlock,
  CodeBlockBody,
  CodeBlockCopyButton,
  CodeBlockHeader,
  codeBlockCopyButtonVariants,
};
