import { useEffect, useRef } from 'react';

type Props = {
  open: boolean;
  onOpenChange(open: boolean): void;
  onSettings(): void;
  onPlugins(): void;
  warning: boolean;
};

export default function LauncherMenu({ open, onOpenChange, onSettings, onPlugins, warning }: Props) {
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const items = useRef<(HTMLButtonElement | null)[]>([]);
  const initialItem = useRef(0);

  useEffect(() => {
    if (!open) return;
    items.current[initialItem.current]?.focus({ preventScroll: true });
    const outside = (event: PointerEvent) => {
      if (!container.current?.contains(event.target as Node)) onOpenChange(false);
    };
    const blur = () => onOpenChange(false);
    window.addEventListener('pointerdown', outside);
    window.addEventListener('blur', blur);
    return () => {
      window.removeEventListener('pointerdown', outside);
      window.removeEventListener('blur', blur);
    };
  }, [open, onOpenChange]);

  function choose(action: () => void) {
    onOpenChange(false);
    action();
  }

  return <div ref={container} className="launcher-menu" onBlur={event => {
    if (!event.currentTarget.contains(event.relatedTarget)) onOpenChange(false);
  }}>
    <button ref={trigger} type="button" className="manage-button" aria-label="更多"
      title="更多" aria-haspopup="menu" aria-expanded={open} aria-controls={open ? 'launcher-menu' : undefined}
      onClick={() => { initialItem.current = 0; onOpenChange(!open); }} onKeyDown={event => {
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
          event.preventDefault();
          initialItem.current = event.key === 'ArrowUp' ? 1 : 0;
          if (open) items.current[initialItem.current]?.focus({ preventScroll: true });
          else onOpenChange(true);
        }
      }}>···{warning && <span className="warning-dot" />}</button>
    {open && <div id="launcher-menu" className="launcher-menu-items" role="menu" aria-label="更多" onKeyDown={event => {
      if (event.key === 'Escape') {
        event.preventDefault(); event.stopPropagation(); onOpenChange(false); trigger.current?.focus({ preventScroll: true });
      } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
        event.preventDefault();
        const current = items.current.indexOf(document.activeElement as HTMLButtonElement);
        const index = event.key === 'Home' ? 0 : event.key === 'End' ? 1 : (current + (event.key === 'ArrowDown' ? 1 : -1) + 2) % 2;
        items.current[index]?.focus({ preventScroll: true });
      }
    }}>
      <button ref={element => { items.current[0] = element; }} type="button" role="menuitem" onClick={() => choose(onSettings)}>设置</button>
      <button ref={element => { items.current[1] = element; }} type="button" role="menuitem" onClick={() => choose(onPlugins)}>插件管理</button>
    </div>}
  </div>;
}
