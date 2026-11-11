export type ThemeMode='system'|'light'|'dark';
export interface ThemeSnapshot{mode:ThemeMode;revision:number;id:string;appearance:'light'|'dark';tokens:Record<string,string>}
export function currentTheme():ThemeSnapshot|undefined;
export function onThemeChanged(listener:(snapshot:ThemeSnapshot|undefined)=>void):()=>void;
