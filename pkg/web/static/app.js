// kubectl-inventory // radar — SPA
const API = '';
let state = { screen:'connect', ns:'*', inv:null, alerts:null, namespaces:[], radar:null, health:[], resources:[], canvasData:null, drillGroup:null, drillKind:null, alertFilter:'ALL', scale:1, panX:0, panY:0 };

async function api(path){ const r=await fetch(API+path); return r.json(); }

function h(tag,attrs,children){
  attrs=attrs||{}; children=Array.isArray(children)?children:(children!=null?[children]:[]);
  const el=document.createElement(tag);
  Object.entries(attrs).forEach(([k,v])=>{
    if(k==='class')el.className=v;
    else if(k==='style'&&typeof v==='object')Object.assign(el.style,v);
    else if(k.startsWith('on'))el.addEventListener(k.slice(2).toLowerCase(),v);
    else el.setAttribute(k,v);
  });
  children.forEach(c=>{ if(c==null)return; el.appendChild(typeof c==='string'?document.createTextNode(c):c); });
  return el;
}

function renderNav(){
  const screens=[['connect','CONNECT'],['radar','RADAR'],['namespaces','NAMESPACES'],['health','HEALTH'],['canvas','CANVAS']];
  return h('div',{class:'nav'},[h('span',{class:'nav-brand'},['kubectl-inventory']),...screens.map(([id,label])=>h('button',{class:'nav-btn'+(state.screen===id?' active':''),onClick:()=>navigate(id)},[label]))]);
}

function navigate(screen){
  state.screen=screen; render();
  if(screen==='radar')loadRadar();
  else if(screen==='health')loadHealth();
  else if(screen==='namespaces')loadNamespaces();
  else if(screen==='canvas')loadCanvas();
}

function renderLoading(msg){ msg=msg||'SCANNING RESOURCES...'; return h('div',{class:'loading'},[h('div',{class:'loading-text'},[msg]),h('div',{class:'loading-sub'},['please wait'])]); }

async function loadConnect(){
  try{ const [inv,ctxs]=await Promise.all([api('/api/inventory'),api('/api/contexts')]); state.inv=inv; state.contexts=ctxs; }
  catch(e){ console.error(e); }
  render();
}

function renderConnect(){
  const ctxs=state.contexts||[]; const inv=state.inv||{};
  return h('div',{class:'cockpit'},[
    h('header',{class:'page-header'},[h('div',{class:'branding'},[h('h1',{},['kubectl-inventory // connect']),h('div',{class:'meta-row'},[h('span',{},['MODULE: KUBECONFIG_SCANNER']),h('span',{},['FOUND: '+ctxs.length+' CONTEXT'+(ctxs.length!==1?'S':'')])])]),h('div',{class:'status-ind'},[h('div',{class:'dot'},[]),'AWAITING TARGET'])]),
    h('div',{class:'instruction'},['\u003e Select a cluster context to launch the radar interface.']),
    h('div',{},[...ctxs.map(ctx=>h('div',{class:'ctx-item'+(ctx.active?' active':''),onClick:()=>navigate('radar')},[h('div',{class:'ctx-badge badge-ok'},[ctx.status||'CONNECTED']),h('div',{},[h('div',{class:'ctx-name'},[ctx.name||'unknown']),h('div',{class:'ctx-server'},['Local cluster context'])]),h('div',{class:'ctx-stat'},[h('div',{class:'stat-lbl'},['LAST SCAN']),h('div',{class:'stat-val'},[ctx.lastScan||'just now'])]),h('div',{class:'ctx-stat'},[h('div',{class:'stat-lbl'},['RESOURCES']),h('div',{class:'stat-val hi'},[String(ctx.resources||inv.totalResources||0)])]),h('button',{class:'btn btn-primary'},['Launch Radar'])]))])
  ]);
}

async function loadRadar(){
  state.radar=null; render();
  try{ state.radar=await api('/api/radar?namespace='+encodeURIComponent(state.ns)); }catch(e){ state.radar=[]; }
  render();
}

function radarCard(grp){
  return h('div',{class:'api-group'},[h('div',{class:'group-hdr'},[h('span',{class:'group-title'},[grp.group]),h('span',{class:'group-meta'},[grp.resources.length+' types'])]),h('div',{},[...grp.resources.map(r=>h('div',{class:'res-row',onClick:()=>openDrill(grp.group,r.kind)},[h('span',{class:'res-name'},[r.kind||r.name]),h('div',{class:'leader'},[]),h('div',{class:'res-data'},[h('span',{class:'total'},[String(r.total)]),h('div',{class:'signals'},[...Object.entries(r.signals).map(([sig,cnt])=>h('span',{class:'sig sig-'+sig},[sig+' ',h('strong',{},[String(cnt)])]))])])]))]) ]);
}

function renderRadar(){
  const inv=state.inv||{};
  if(!state.radar) return h('div',{class:'cockpit'},[renderLoading()]);
  const grid=h('div',{class:'radar-grid'});
  state.radar.forEach(g=>grid.appendChild(radarCard(g)));
  return h('div',{class:'cockpit'},[h('header',{class:'page-header'},[h('div',{class:'branding'},[h('h1',{},['kubectl-inventory // radar']),h('div',{class:'meta-row'},[h('span',{},['CTX: '+(inv.context||'—')]),h('span',{},['NS: '+(state.ns==='*'?'ALL':state.ns)]),h('span',{},['MODE: DEEP_SCAN'])])]),h('div',{class:'status-ind'},[h('div',{class:'dot'},[]),'LIVE INTELLIGENCE STREAMING'])]),grid]);
}

function openDrill(group,kind){ state.drillGroup=group; state.drillKind=kind; state.screen='drill'; state.resources=[]; render(); loadDrill(); }

async function loadDrill(){
  try{ state.resources=await api('/api/resources?kind='+encodeURIComponent(state.drillKind||'')+'&namespace='+encodeURIComponent(state.ns)); }catch(e){ state.resources=[]; }
  render();
}

function renderDrill(){
  const inv=state.inv||{}; const rows=state.resources||[];
  const sigCounts={}; rows.forEach(r=>{ sigCounts[r.signal]=(sigCounts[r.signal]||0)+1; });
  const hasDang=rows.some(r=>r.signal==='DANG'||r.signal==='STUCK');
  return h('div',{class:'cockpit'},[
    h('header',{class:'page-header'},[h('div',{class:'branding'},[h('h1',{class:hasDang?'red':''},['kubectl-inventory // radar // '+(state.drillKind||'')]),h('div',{class:'meta-row'},[h('span',{},['CTX: '+(inv.context||'—')]),h('span',{},['NS: '+(state.ns==='*'?'ALL':state.ns)]),h('span',{},['MODE: DRILL_DOWN'])])]),h('div',{class:'status-ind '+(hasDang?'red':'')},[h('div',{class:'dot '+(hasDang?'red':'')},[]),hasDang?'ATTENTION REQUIRED':'LIVE'])]),
    h('div',{style:{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:'16px'}},[h('div',{class:'signals'},[...Object.entries(sigCounts).map(([s,c])=>h('span',{class:'sig sig-'+s},[s+' ',h('strong',{},[String(c)])]))]),h('button',{class:'btn',onClick:()=>navigate('radar')},['< Back'])]),
    h('div',{style:{overflowX:'auto'}},[h('table',{class:'data-table'},[h('thead',{},[h('tr',{},[h('th',{},['Resource Name']),h('th',{},['Namespace']),h('th',{},['Signal']),h('th',{},['Age']),h('th',{},['Reason'])])]),h('tbody',{},[...rows.map(r=>h('tr',{},[h('td',{class:'col-name'},[r.name]),h('td',{class:'col-ns'},[r.namespace||'—']),h('td',{},[h('span',{class:'sig sig-'+r.signal},[r.signal])]),h('td',{class:'col-age'},[r.age]),h('td',{class:'col-reason'},[r.reason||'—'])]))])]),h('div',{style:{marginTop:'12px',textAlign:'right',color:'var(--muted)',fontSize:'.8rem'}},['Showing '+rows.length+' resources'])])
  ]);
}

async function loadAlerts(){
  state.alerts=null; state.alertFilter='ALL'; render();
  try{ state.alerts=await api('/api/alerts'); }catch(e){ state.alerts=[]; }
  render();
}

function renderAlerts(){
  if(!state.alerts) return h('div',{class:'cockpit'},[renderLoading()]);
  const all=state.alerts; const f=state.alertFilter||'ALL';
  const shown=f==='ALL'?all:all.filter(a=>a.signal===f);
  const dc=all.filter(a=>a.signal==='DANG').length;
  const sc=all.filter(a=>a.signal==='STUCK').length;
  return h('div',{class:'cockpit'},[
    h('header',{class:'page-header'},[h('div',{class:'branding'},[h('h1',{class:'red'},['kubectl-inventory // alerts']),h('div',{class:'meta-row'},[h('span',{},['CTX: '+(state.inv&&state.inv.context||'—')]),h('span',{},['MODE: TRIAGE'])])]),h('div',{class:'status-ind red'},[h('div',{class:'dot red'},[]),'CRITICAL INCIDENTS: '+all.length])]),
    h('div',{class:'filters'},[h('button',{class:'filter-btn'+(f==='ALL'?' active':''),onClick:()=>{state.alertFilter='ALL';render();}},['ALL ALERTS ['+all.length+']']),h('button',{class:'filter-btn'+(f==='DANG'?' active':''),onClick:()=>{state.alertFilter='DANG';render();}},['DANG ['+dc+']']),h('button',{class:'filter-btn'+(f==='STUCK'?' active':''),onClick:()=>{state.alertFilter='STUCK';render();}},['STUCK ['+sc+']'])]),
    shown.length===0 ? h('div',{style:{textAlign:'center',padding:'60px 0',color:'var(--green)',fontSize:'.9rem'}},['// NO ALERTS — CLUSTER HEALTHY']) : h('div',{},[...shown.map(a=>h('div',{class:'alert-card'},[h('div',{class:'alert-info'},[h('span',{class:'sig sig-'+a.signal},[a.signal]),h('span',{class:'alert-sev sev-'+a.severity},[a.severity]),h('div',{class:'alert-details'},[h('span',{class:'res-id'},[a.resource]),h('div',{class:'res-meta'},[h('span',{},['API: '+a.apiGroup]),h('span',{},['NS: '+(a.namespace||'—')]),h('span',{},['ISSUE: '+a.issue])])])]),h('button',{class:'btn-action'},['Investigate'])]))])
  ]);
}

async function loadNamespaces(){
  try{ state.namespaces=await api('/api/namespaces'); if(!state.radar) state.radar=await api('/api/radar?namespace=*'); }catch(e){ state.namespaces=[]; }
  render();
}

function renderNamespaces(){
  const nsList=[{name:'* [ALL]',count:state.inv&&state.inv.totalResources||0},...(state.namespaces||[])];
  const inv=state.inv||{}; const groups=state.radar||[];
  return h('div',{class:'cockpit'},[
    h('header',{class:'page-header'},[h('div',{class:'branding'},[h('h1',{},['kubectl-inventory // radar']),h('div',{class:'meta-row'},[h('span',{},['CTX: '+(inv.context||'—')]),h('span',{style:{color:'var(--blue)'}},['NS: '+(state.ns==='*'?'ALL':state.ns)]),h('span',{},['MODE: NS_FILTER'])])]),h('div',{class:'status-ind'},[h('div',{class:'dot'},[]),'LIVE'])]),
    h('div',{class:'workspace'},[h('aside',{class:'sidebar'},[h('div',{class:'sidebar-title'},['Namespaces']),h('div',{class:'ns-list'},[...nsList.map(ns=>{ const nsKey=ns.name==='* [ALL]'?'*':ns.name; return h('div',{class:'ns-item'+(state.ns===nsKey?' active':''),onClick:async()=>{ state.ns=nsKey; state.radar=null; render(); await loadRadar(); }},[h('span',{},[ns.name]),h('span',{class:'ns-count'},[String(ns.count)])]); })])]),h('div',{class:'radar-grid',style:{flexGrow:'1',overflowY:'auto'}},[...groups.map(g=>radarCard(g))])])
  ]);
}

async function loadHealth(){
  try{ state.health=await api('/api/health'); }catch(e){ state.health=[]; }
  render();
}

function renderHealth(){
  const inv=state.inv||{}; const stats=state.health||[];
  const sparkData=[40,45,52,60,65,72,78,82,88,90,95,100];
  return h('div',{class:'cockpit'},[
    h('header',{class:'page-header'},[h('div',{class:'branding'},[h('h1',{},['kubectl-inventory // health summary']),h('div',{class:'meta-row'},[h('span',{},['CTX: '+(inv.context||'—')]),h('span',{},['VIEW: CLUSTER_HEALTH'])])]),h('div',{class:'status-ind'},[h('div',{class:'dot'},[]),'LIVE INTELLIGENCE STREAMING'])]),
    h('div',{class:'stat-grid'},[...stats.map(s=>h('div',{class:'stat-block '+s.class,onClick:async()=>{ state.screen='drill'; state.resources=[]; state.drillKind=s.label; render(); await loadResourcesBySignal(s.class); }},[h('div',{class:'stat-hdr'},[h('span',{class:'stat-title'},[s.label]),h('span',{style:{fontSize:'.68rem',color:'var(--muted)',textShadow:'none'}},['24H TREND'])]),h('div',{class:'stat-value'},[String(s.count)]),h('div',{class:'sparkline'},[...sparkData.map(p=>h('div',{class:'spark-bar',style:{height:p+'%'}},[])) ])]))])
  ]);
}

async function loadResourcesBySignal(cls){
  const m={clean:'CLEAN',owned:'OWNED',gen:'GEN',generated:'GEN',referenced:'REF',ref:'REF',susp:'SUSP',suspicious:'SUSP',dang:'DANG',dangling:'DANG',stuck:'STUCK',standalone:'CLEAN'};
  try{ state.resources=await api('/api/resources?signal='+(m[cls]||cls.toUpperCase())); }catch(e){ state.resources=[]; }
  render();
}

// ── Canvas ────────────────────────────────────────────────────────────────────
async function loadCanvas(){
  state.canvasData=null; state.selectedCanvasNode=null; render();
  try{ state.canvasData=await api('/api/canvas?namespace='+encodeURIComponent(state.ns)); }
  catch(e){ console.error('canvas error',e); state.canvasData={nodes:[],edges:[]}; }
  render();
}

function iconFor(kind){
  const icons={
    Deployment:'<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>',
    ReplicaSet:'<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>',
    StatefulSet:'<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>',
    DaemonSet:'<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>',
    Pod:'<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>',
    Service:'<circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><path d="M10 8l-3 8M14 8l3 8M7 19h10"/>',
    Ingress:'<circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><path d="M10 8l-3 8M14 8l3 8M7 19h10"/>',
    EndpointSlice:'<circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><path d="M10 8l-3 8M14 8l3 8M7 19h10"/>',
    NetworkPolicy:'<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
    Secret:'<path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/>',
    ConfigMap:'<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/>',
    HorizontalPodAutoscaler:'<path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>',
    ServiceAccount:'<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>',
    CronJob:'<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>',
    Job:'<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>',
  };
  const d=icons[kind]||'<circle cx="12" cy="12" r="9"/>';
  return '<svg viewBox="0 0 24 24">'+d+'</svg>';
}

function chipClass(sig){
  const m={CLEAN:'clean',GEN:'gen',DANG:'dang',SUSP:'susp',REF:'ref',OWNED:'owned',STUCK:'stuck'};
  return 'chip chip-'+(m[sig]||'clean');
}

function renderCanvas(){
  const inv=state.inv||{};
  if(!state.canvasData) return h('div',{class:'canvas-cockpit'},[h('div',{class:'canvas-header'},[h('div',{class:'branding'},[h('h1',{},['kubectl-inventory // canvas'])]),renderLoading('BUILDING CANVAS...')])]);
  const nodes=state.canvasData.nodes||[];
  const edges=state.canvasData.edges||[];

  // BFS depth
  const childOf={}, parentOf={};
  edges.forEach(e=>{ if(!childOf[e.from])childOf[e.from]=[]; childOf[e.from].push(e.to); parentOf[e.to]=e.from; });
  const depth={};
  nodes.filter(n=>!parentOf[n.id]).forEach(n=>{ const q=[{id:n.id,d:0}]; while(q.length){const {id,d}=q.shift();if(depth[id]!==undefined)continue;depth[id]=d;(childOf[id]||[]).forEach(c=>q.push({id:c,d:d+1}));} });
  nodes.forEach(n=>{ if(depth[n.id]===undefined)depth[n.id]=0; });

  // Layout positions
  const NW=240, NH=108, GAPX=80, GAPY=16;
  const colCount={}, pos={};
  const hasEdges=edges.length>0;
  const GCOLS=4;
  nodes.forEach((n,idx)=>{
    if(hasEdges){ const col=depth[n.id]||0; const row=colCount[col]||0; colCount[col]=row+1; pos[n.id]={x:col*(NW+GAPX)+24,y:row*(NH+GAPY)+24}; }
    else { pos[n.id]={x:(idx%GCOLS)*(NW+GAPX)+24,y:Math.floor(idx/GCOLS)*(NH+GAPY)+24}; }
  });

  // Canvas size
  const allP=Object.values(pos);
  const cW=allP.length?Math.max(...allP.map(p=>p.x))+NW+60:800;
  const cH=allP.length?Math.max(...allP.map(p=>p.y))+NH+60:500;

  // Outer wrap (scrollable/pannable)
  const wrap=document.createElement('div');
  wrap.className='canvas-wrap';
  wrap.addEventListener('wheel',e=>{ e.preventDefault(); state.scale=Math.max(.2,Math.min(3,state.scale-e.deltaY*.001)); applyTransform(); },{passive:false});
  let panning=false, panStart={x:0,y:0};
  wrap.addEventListener('mousedown',e=>{ if(!e.target.closest('.node')&&!e.target.closest('.panel')){ panning=true; panStart={x:e.clientX-state.panX,y:e.clientY-state.panY}; wrap.style.cursor='grabbing'; } });
  document.addEventListener('mousemove',e=>{ if(panning){ state.panX=e.clientX-panStart.x; state.panY=e.clientY-panStart.y; applyTransform(); } });
  document.addEventListener('mouseup',()=>{ panning=false; wrap.style.cursor=''; });

  // Inner (transformable)
  const inner=document.createElement('div');
  inner.className='canvas-inner';
  inner.style.cssText='width:'+cW+'px;height:'+cH+'px;';
  wrap.appendChild(inner);

  // SVG — must match inner size exactly
  const SN='http://www.w3.org/2000/svg';
  const svg=document.createElementNS(SN,'svg');
  svg.setAttribute('width',cW); svg.setAttribute('height',cH);
  svg.setAttribute('viewBox','0 0 '+cW+' '+cH);
  svg.style.cssText='position:absolute;top:0;left:0;pointer-events:none;z-index:1;';
  inner.appendChild(svg);

  // Draw edges
  const eCol={owner:'#4b5563',gen:'#bf5af2',dang:'#ff3b30',spec:'#6b7280'};
  const eDash={dang:'6,4',spec:'4,4'};
  edges.forEach(e=>{
    const fp=pos[e.from], tp=pos[e.to]; if(!fp||!tp)return;
    const x1=fp.x+NW, y1=fp.y+NH/2, x2=tp.x, y2=tp.y+NH/2;
    const cx1=x1+GAPX*0.55, cx2=x2-GAPX*0.55;
    const path=document.createElementNS(SN,'path');
    path.setAttribute('d','M '+x1+' '+y1+' C '+cx1+' '+y1+' '+cx2+' '+y2+' '+x2+' '+y2);
    path.setAttribute('fill','none');
    path.setAttribute('stroke',eCol[e.type]||'#4b5563');
    path.setAttribute('stroke-width','1.5');
    if(eDash[e.type]) path.setAttribute('stroke-dasharray',eDash[e.type]);
    svg.appendChild(path);
  });

  // Draw nodes
  const sel=state.selectedCanvasNode;
  nodes.forEach(n=>{
    const p=pos[n.id]||{x:0,y:0};
    const sig=n.signal||'CLEAN';
    const isRoot=!parentOf[n.id]&&childOf[n.id];
    const isSel=sel&&sel.id===n.id;

    const nd=document.createElement('div');
    nd.className='node'+(isRoot?' root':'')+(isSel?' is-selected':'');
    nd.style.cssText='left:'+p.x+'px;top:'+p.y+'px;z-index:2;';
    nd.addEventListener('click',()=>{ state.selectedCanvasNode=n; renderCanvasPanel(wrap,nodes,edges); nd.parentElement.querySelectorAll('.node').forEach(x=>x.classList.remove('is-selected')); nd.classList.add('is-selected'); });

    // Header
    const hdr=document.createElement('div'); hdr.className='node-header';
    const tw=document.createElement('div'); tw.className='node-title-wrap';
    const ic=document.createElement('div'); ic.className='node-icon'; ic.innerHTML=iconFor(n.kind);
    const tl=document.createElement('div'); tl.className='node-title'; tl.textContent=n.name;
    tw.appendChild(ic); tw.appendChild(tl);
    const chip=document.createElement('div'); chip.className=chipClass(sig); chip.textContent=sig;
    hdr.appendChild(tw); hdr.appendChild(chip);

    // Body
    const body=document.createElement('div'); body.className='node-body';
    [{k:'KIND',v:n.kind},{k:'NS',v:n.namespace||'—'},{k:'AGE',v:n.age}].forEach(({k,v})=>{
      const row=document.createElement('div'); row.className='node-meta';
      row.innerHTML='<span class="key">'+k+'</span><span>'+v+'</span>';
      body.appendChild(row);
    });

    nd.appendChild(hdr); nd.appendChild(body);
    inner.appendChild(nd);
  });

  // Legend
  const leg=document.createElement('div'); leg.className='panel legend-panel';
  const legTitle=document.createElement('div'); legTitle.className='panel-title-sm'; legTitle.textContent='CANVAS LEGEND'; leg.appendChild(legTitle);
  [['#4b5563','0','OWNER REFERENCE'],['#6b7280','4,4','SPEC REFERENCE'],['#bf5af2','0','CONTROLLER GENERATED'],['#ff3b30','6,4','DANGLING / MISSING']].forEach(([col,dash,lbl])=>{
    const row=document.createElement('div'); row.className='legend-item';
    const s=document.createElementNS(SN,'svg'); s.setAttribute('width','22'); s.setAttribute('height','10');
    const l=document.createElementNS(SN,'line'); l.setAttribute('x1','0'); l.setAttribute('y1','5'); l.setAttribute('x2','22'); l.setAttribute('y2','5'); l.setAttribute('stroke',col); l.setAttribute('stroke-width','1.5'); if(dash!=='0')l.setAttribute('stroke-dasharray',dash);
    s.appendChild(l); row.appendChild(s); row.appendChild(document.createTextNode(lbl)); leg.appendChild(row);
  });
  wrap.appendChild(leg);

  // Zoom controls
  const zm=document.createElement('div'); zm.className='zoom-ctrl';
  ['+','−','[]'].forEach((lbl,i)=>{
    const b=document.createElement('button'); b.className='zoom-btn'; b.textContent=lbl;
    b.addEventListener('click',()=>{ if(i===0)state.scale=Math.min(3,state.scale+.15); else if(i===1)state.scale=Math.max(.2,state.scale-.15); else{state.scale=1;state.panX=0;state.panY=0;} applyTransform(); });
    zm.appendChild(b);
  });
  wrap.appendChild(zm);

  // If a node was previously selected, show its panel
  if(sel) renderCanvasPanel(wrap,nodes,edges);

  window._canvasInner=inner;
  setTimeout(()=>applyTransform(),0);

  // Canvas cockpit layout (full height, no scroll)
  const cockpit=document.createElement('div'); cockpit.className='canvas-cockpit';
  const hdr2=document.createElement('div'); hdr2.className='canvas-header';
  const br=document.createElement('div'); br.className='branding';
  const t=document.createElement('h1'); t.textContent='kubectl-inventory // resource dependency canvas'; br.appendChild(t);
  const mr=document.createElement('div'); mr.className='meta-row';
  mr.innerHTML='<span>CTX: '+(inv.context||'—')+'</span><span>NS: '+(state.ns==='*'?'ALL':state.ns)+'</span><span>['+nodes.length+' NODES] ['+edges.length+' EDGES]</span>';
  br.appendChild(mr);
  const si=document.createElement('div'); si.className='status-ind';
  const dot=document.createElement('div'); dot.className='dot'; si.appendChild(dot); si.appendChild(document.createTextNode('LIVE INTELLIGENCE STREAMING'));
  hdr2.appendChild(br); hdr2.appendChild(si);
  cockpit.appendChild(hdr2); cockpit.appendChild(wrap);
  return cockpit;
}

function renderCanvasPanel(wrap,nodes,edges){
  // Remove old panel
  const old=wrap.querySelector('.explain-panel'); if(old)old.remove();
  const n=state.selectedCanvasNode; if(!n)return;

  // Count incoming refs (edges pointing TO this node)
  const incoming=edges.filter(e=>e.to===n.id);

  const panel=document.createElement('div'); panel.className='panel explain-panel';

  // Header
  const phdr=document.createElement('div'); phdr.className='panel-header';
  const psm=document.createElement('div'); psm.className='panel-title-sm'; psm.textContent='SELECTED RESOURCE'; phdr.appendChild(psm);
  const plg=document.createElement('div'); plg.className='panel-title-lg'; plg.textContent=n.kind+' // '+n.name; phdr.appendChild(plg);
  panel.appendChild(phdr);

  // KV details
  const kv=document.createElement('div'); kv.className='kv-list';
  [{k:'Namespace',v:n.namespace||'—'},{k:'Age',v:n.age},{k:'Status',v:n.signal,cls:(n.signal==='CLEAN'||n.signal==='REF')?'ok':''}].forEach(({k,v,cls})=>{
    const row=document.createElement('div'); row.className='kv-row';
    const key=document.createElement('span'); key.className='kv-key'; key.textContent=k;
    const val=document.createElement('span'); val.className='kv-val'+(cls?' '+cls:''); val.textContent=v;
    row.appendChild(key); row.appendChild(val); kv.appendChild(row);
  });
  panel.appendChild(kv);

  // Incoming refs
  if(incoming.length){
    const rt=document.createElement('div'); rt.className='panel-title-sm'; rt.style.marginTop='4px'; rt.textContent='INCOMING REFERENCES ('+incoming.length+')'; panel.appendChild(rt);
    const rl=document.createElement('div'); rl.className='ref-list';
    incoming.slice(0,4).forEach(e=>{
      const src=nodes.find(x=>x.id===e.from);
      const ri=document.createElement('div'); ri.className='ref-item';
      const chip=document.createElement('div'); chip.className='chip chip-ref'; chip.textContent='SPEC';
      const lbl=document.createElement('span'); lbl.style.fontSize='.72rem'; lbl.textContent=(src?src.kind+'/'+src.name:'—');
      ri.appendChild(chip); ri.appendChild(lbl); rl.appendChild(ri);
    });
    panel.appendChild(rl);
  }

  // Actions
  const acts=document.createElement('div'); acts.className='panel-actions';

  // Copy kubectl command to clipboard
  const copyBtn=document.createElement('button'); copyBtn.className='panel-btn'; copyBtn.textContent='COPY KUBECTL COMMAND';
  copyBtn.addEventListener('click',()=>{
    const ns=n.namespace?'-n '+n.namespace:'';
    const cmd='kubectl get '+n.kind.toLowerCase()+' '+n.name+' '+ns+' -o yaml';
    navigator.clipboard.writeText(cmd).then(()=>{
      copyBtn.textContent='COPIED!';
      copyBtn.style.borderColor='var(--green)'; copyBtn.style.color='var(--green)';
      setTimeout(()=>{ copyBtn.textContent='COPY KUBECTL COMMAND'; copyBtn.style.borderColor=''; copyBtn.style.color=''; },2000);
    }).catch(()=>{
      copyBtn.textContent='COPY FAILED'; setTimeout(()=>{ copyBtn.textContent='COPY KUBECTL COMMAND'; },2000);
    });
  });
  acts.appendChild(copyBtn);

  // Drill into kind
  const drillBtn=document.createElement('button'); drillBtn.className='panel-btn primary'; drillBtn.textContent='DRILL INTO KIND';
  drillBtn.addEventListener('click',()=>openDrill('',n.kind));
  acts.appendChild(drillBtn);

  panel.appendChild(acts);

  wrap.appendChild(panel);
}

function applyTransform(){
  if(window._canvasInner) window._canvasInner.style.transform='translate('+state.panX+'px,'+state.panY+'px) scale('+state.scale+')';
}



function render(){
  const app=document.getElementById('app');
  app.innerHTML='';
  app.appendChild(renderNav());
  try{
    const screens={connect:renderConnect,radar:renderRadar,drill:renderDrill,namespaces:renderNamespaces,health:renderHealth,canvas:renderCanvas};
    app.appendChild((screens[state.screen]||renderConnect)());
  }catch(e){ console.error('render error:',e); app.appendChild(h('div',{class:'loading'},[h('div',{class:'loading-text',style:{color:'var(--red)'}},['RENDER ERROR']),h('div',{class:'loading-sub'},[e.message])])); }
}

(async()=>{
  render();
  await loadConnect();
  if(state.inv){ state.screen='radar'; await loadRadar(); }
  render();
})();
