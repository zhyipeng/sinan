type OpenKeyEvent = Pick<KeyboardEvent, 'key' | 'isComposing' | 'keyCode' | 'repeat' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>;

export function isSearchOpenKey(event: OpenKeyEvent, spaceToOpen: boolean): boolean {
  // WebKit may only identify the final IME keydown through keyCode 229.
  if (event.isComposing || event.keyCode === 229) return false;
  if (event.key === 'Enter') return true;
  return spaceToOpen && event.key === ' ' && !event.repeat &&
    !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey;
}
