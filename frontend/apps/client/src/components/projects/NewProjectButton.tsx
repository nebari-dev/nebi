// Owner: U1 (projects). "New project" split button (prototype): the main
// action opens the GUI form; the menu offers pixi.toml or GUI.
import { Button } from '@nebi/ui/components/ui/button';
import {
  ButtonGroup,
  ButtonGroupSeparator,
} from '@nebi/ui/components/ui/button-group';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { ChevronDown, CodeXml, LayoutTemplate, Plus } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

export const NewProjectButton = () => {
  const navigate = useNavigate();
  return (
    <ButtonGroup aria-label="New project">
      <Button onClick={() => navigate('/projects/new')}>
        <Plus />
        New project
      </Button>
      <ButtonGroupSeparator className="bg-primary-foreground/40" />
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger
          aria-label="More ways to create a project"
          className="w-8 bg-primary px-0 text-primary-foreground shadow-xs hover:bg-primary-hover hover:text-primary-foreground active:bg-primary-hover active:text-primary-foreground data-[popup-open]:bg-primary-hover data-[popup-open]:text-primary-foreground"
        >
          <ChevronDown />
        </DropdownMenuTrigger>
        <DropdownMenuPortal>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onClick={() => navigate('/projects/new?mode=toml')}
            >
              <CodeXml />
              Create from pixi.toml
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => navigate('/projects/new')}>
              <LayoutTemplate />
              Create from GUI
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenuPortal>
      </DropdownMenu>
    </ButtonGroup>
  );
};
