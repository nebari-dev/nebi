// Owner: U2 (detail). Help menu: "Take the page tour" (pages with a tour,
// i.e. a project's detail page) and Docs.

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { headerActionClass } from '@nebi/ui/lib/header';
import { cn } from '@nebi/ui/lib/utils';
import { CircleQuestionMark, ExternalLink, Route } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import { useTourStore } from '@/components/tour/tourStore';

/** Pages that have a tour: /projects/:id (not /projects/new). */
const TOUR_PAGE = /^\/projects\/(?!new$)[^/]+\/?$/;

export const HelpMenu = () => {
  const { pathname } = useLocation();
  const start = useTourStore((s) => s.start);
  const hasTour = TOUR_PAGE.test(pathname);

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        variant="ghost"
        data-tour="help"
        className={cn('gap-1.5 px-2', headerActionClass)}
      >
        <CircleQuestionMark />
        Help
      </DropdownMenuTrigger>
      <DropdownMenuPortal>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuItem disabled={!hasTour} onClick={start}>
            <Route />
            {hasTour ? 'Take the page tour' : 'No tour for this page yet'}
          </DropdownMenuItem>
          <DropdownMenuItem
            render={
              <a
                href="https://nebi.nebari.dev/"
                target="_blank"
                rel="noreferrer"
              />
            }
          >
            <ExternalLink />
            Docs
            <span className="sr-only">(opens in a new tab)</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenuPortal>
    </DropdownMenu>
  );
};
