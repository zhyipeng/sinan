import type {ReactNode} from 'react';
import PluginIcon from '../../../packages/ui-components/PluginIcon';

export default function PluginDetails({plugin,children}:{plugin:{id:string;name:string;icon?:string};children:ReactNode}) {
  return <section className="store-detail" aria-label={`${plugin.name} 详情`}>
    <div className="store-detail-title"><PluginIcon name={plugin.name} icon={plugin.icon}/><h2>{plugin.name}</h2></div>
    <small>{plugin.id}</small>
    {children}
  </section>;
}
