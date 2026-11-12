export const MAX_PASTE_TEXT_BYTES = 64 * 1024;
export const MAX_PASTE_IMAGE_BYTES = 4 * 1024 * 1024;
export const MAX_PASTE_IMAGE_PIXELS = 16 * 1024 * 1024;
export type PastedInput =
  | { kind: 'text'; text: string }
  | { kind: 'image'; dataBase64: string; previewURL: string; width: number; height: number };

export function pasteBackspaceText(paste: PastedInput): string {
  return paste.kind === 'text' && Array.from(paste.text).length <= 30 ? paste.text : '';
}
export function filterPasteResults<T extends { title: string; subtitle: string; pluginId: string; commandId: string }>(
  items: T[], keyword: string, plugins: { id: string; name: string; commands: { id: string; keywords?: string[] | null }[] }[]
): T[] {
  const terms = keyword.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean);
  if (!terms.length) return items;
  return items.filter(item => {
    const plugin = plugins.find(plugin => plugin.id === item.pluginId);
    const command = plugin?.commands.find(command => command.id === item.commandId);
    const text = [item.title, item.subtitle, plugin?.name, ...(command?.keywords ?? [])].join(' ').toLocaleLowerCase();
    return terms.every(term => text.includes(term));
  });
}
export function escapeAction(hasOtherMode: boolean, query: string): 'reset' | 'hide' {
  return hasOtherMode || query.length > 0 ? 'reset' : 'hide';
}
export function disposePaste(paste?: PastedInput) {
  if (paste?.kind === 'image') URL.revokeObjectURL(paste.previewURL);
}
export async function readPastedImage(file: File): Promise<PastedInput> {
  if (file.size > 16 * 1024 * 1024) throw new Error('图片源文件超过 16 MiB');
  const source = URL.createObjectURL(file);
  try {
    const image = new Image();
    await new Promise<void>((resolve, reject) => { image.onload = () => resolve(); image.onerror = () => reject(new Error('无法读取粘贴图片')); image.src = source; });
    if (!image.naturalWidth || !image.naturalHeight || image.naturalWidth * image.naturalHeight > MAX_PASTE_IMAGE_PIXELS) throw new Error('图片超过 1600 万像素');
    const canvas = document.createElement('canvas');
    canvas.width = image.naturalWidth; canvas.height = image.naturalHeight;
    const context = canvas.getContext('2d');
    if (!context) throw new Error('无法处理粘贴图片');
    context.drawImage(image, 0, 0);
    const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob(value => value ? resolve(value) : reject(new Error('无法转换粘贴图片')), 'image/png'));
    if (blob.size > MAX_PASTE_IMAGE_BYTES) throw new Error('图片转换后超过 4 MiB');
    const encoded = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result).split(',')[1]);
      reader.onerror = () => reject(new Error('无法读取粘贴图片'));
      reader.readAsDataURL(blob);
    });
    return { kind: 'image', dataBase64: encoded, previewURL: URL.createObjectURL(blob), width: canvas.width, height: canvas.height };
  } finally { URL.revokeObjectURL(source); }
}
