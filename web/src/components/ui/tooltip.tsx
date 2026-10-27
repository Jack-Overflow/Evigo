import { useState, type ReactElement, type ReactNode } from 'react';
import { Tooltip as Primitive } from 'radix-ui';

export function Tooltip({
  children,
  content,
  side = 'top',
}: {
  children: ReactElement;
  content: ReactNode;
  side?: 'top' | 'bottom';
}) {
  const [open, setOpen] = useState(false);

  return (
    <Primitive.Provider delayDuration={200}>
      <Primitive.Root open={open} onOpenChange={setOpen}>
        <Primitive.Trigger
          asChild
          onClick={(event) => {
            event.preventDefault();
            setOpen(true);
          }}
        >
          {children}
        </Primitive.Trigger>
        <Primitive.Portal>
          <Primitive.Content
            side={side}
            sideOffset={6}
            className="z-50 max-w-xs rounded-md border border-border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md"
          >
            {content}
            <Primitive.Arrow className="fill-popover" />
          </Primitive.Content>
        </Primitive.Portal>
      </Primitive.Root>
    </Primitive.Provider>
  );
}
