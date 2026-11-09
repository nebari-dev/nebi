// Owner: U2 (detail). Help menu: "Take the page tour" (pages with a tour)
// and Docs. U2 wires the guided tour.

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { headerActionClass } from '@nebi/ui/lib/header';
import { cn } from '@nebi/ui/lib/utils';
import { BookOpen, CircleQuestionMark, Route } from 'lucide-react';

export const HelpMenu = () => (
  <DropdownMenu modal={false}>
    <DropdownMenuTrigger
      variant="ghost"
      className={cn('gap-1.5 px-2', headerActionClass)}
    >
      <CircleQuestionMark />
      Help
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuItem disabled>
          <Route />
          No tour for this page yet
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
          <BookOpen />
          Docs
          <span className="sr-only">(opens in a new tab)</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenu>
);
