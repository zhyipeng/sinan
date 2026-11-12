import { useState } from 'react';

/** Shared host artwork. An image failure always leaves a readable plugin initial. */
export default function PluginIcon({name,icon,className=''}:{name:string;icon?:string;className?:string}) {
  const [failed,setFailed]=useState('');
  const initial=Array.from(name.trim())[0]||'?';
  return <span className={`plugin-icon ${className}`} aria-hidden="true">
    {icon&&failed!==icon?<img key={icon} src={icon} alt="" draggable={false} onError={()=>setFailed(icon)}/>:initial}
  </span>;
}
