import { Component, type ReactNode } from 'react';

// Trusted component failures must leave the Shell's recovery controls reachable.
export default class PluginViewBoundary extends Component<{children: ReactNode; onRecover(): void}, {failed: boolean}> {
  state={failed:false};
  static getDerivedStateFromError(){return {failed:true};}
  render(){
    if(this.state.failed)return <div className="tool-body"><p className="error" role="alert">插件界面加载失败。</p><button onClick={this.props.onRecover}>打开恢复入口</button></div>;
    return this.props.children;
  }
}
