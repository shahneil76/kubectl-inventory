// kubectl-inventory // radar — SPA
const API = '';
let state = { screen:'connect', ns:'*', inv:null, alerts:null, namespaces:[], radar:null, health:[], resources:[], drillGroup:null, drillKind:null, alertFilter:'ALL', audit:null, auditError:null, auditSettings:null, auditNsScope:'ALL', auditLens:{}, auditPriority:{}, auditFramework:{}, auditSearch:'', auditExpandedNS:{}, auditExpanded:{}, auditShowSettings:false, auditSettingsDraft:null };

async function api(path){ const r=await fetch(API+path); return r.json(); }
async function apiPut(path,body){ const r=await fetch(API+path,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}); return r.json(); }

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
  const screens=[['connect','CONNECT'],['radar','RADAR'],['audit','POSTURE'],['namespaces','NAMESPACES'],['health','HEALTH']];
  return h('div',{class:'nav'},[h('span',{class:'nav-brand'},['kubectl-inventory']),...screens.map(([id,label])=>h('button',{class:'nav-btn'+(state.screen===id?' active':''),onClick:()=>navigate(id)},[label]))]);
}

function navigate(screen){
  state.screen=screen; render();
  if(screen==='radar')loadRadar();
  else if(screen==='audit')loadAudit();
  else if(screen==='health')loadHealth();
  else if(screen==='namespaces')loadNamespaces();
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

// ── Audit ─────────────────────────────────────────────────────────────────────
const AUDIT_LENS_MAP={hardening:'Security',resilience:'Reliability',rightsizing:'Efficiency',inventory:'Reliability'};
const AUDIT_LENS_LABELS=[['hardening','Hardening'],['resilience','Resilience'],['rightsizing','Right-sizing'],['inventory','Inventory DNA']];
const AUDIT_PRIORITY_LABELS=[['mustfix','Must-fix'],['advisory','Advisory']];

function auditResetFilters(){
  state.auditLens={}; state.auditPriority={}; state.auditFramework={}; state.auditSearch=''; state.auditNsScope='ALL';
}

function auditToggleFilter(bucket,key){
  if(!state[bucket]) state[bucket]={};
  if(state[bucket][key]) delete state[bucket][key];
  else state[bucket][key]=true;
  render();
}

function auditHasChipFilters(){
  return Object.keys(state.auditLens||{}).length>0
    || Object.keys(state.auditPriority||{}).length>0
    || Object.keys(state.auditFramework||{}).length>0;
}

function auditFindingMatches(f, checks){
  const lensActive=Object.keys(state.auditLens||{}).filter(k=>state.auditLens[k]);
  if(lensActive.length){
    const isInv=(f.checkID||'').startsWith('inventory:');
    const invOn=!!state.auditLens.inventory;
    const stdOn=lensActive.some(k=>k!=='inventory');
    if(invOn&&stdOn){
      const cats=lensActive.filter(k=>k!=='inventory').map(k=>AUDIT_LENS_MAP[k]).filter(Boolean);
      if(!isInv&&!cats.includes(f.category)) return false;
    }else if(invOn){
      if(!isInv) return false;
    }else{
      if(isInv) return false;
      const cats=lensActive.map(k=>AUDIT_LENS_MAP[k]).filter(Boolean);
      if(!cats.includes(f.category)) return false;
    }
  }
  const priActive=Object.keys(state.auditPriority||{}).filter(k=>state.auditPriority[k]);
  if(priActive.length){
    const ok=(state.auditPriority.mustfix&&f.severity==='danger')
      ||(state.auditPriority.advisory&&f.severity==='warning');
    if(!ok) return false;
  }
  const fwActive=Object.keys(state.auditFramework||{}).filter(k=>state.auditFramework[k]);
  if(fwActive.length&&checks){
    const fws=(checks[f.checkID]&&checks[f.checkID].frameworks)||[];
    if(!fws.some(fw=>state.auditFramework[fw])) return false;
  }
  return true;
}

function auditGroupKey(g){ return (g.kind||'')+'|'+(g.namespace||'')+'|'+(g.name||''); }
function auditNsKey(ns){ return ns||'(cluster-scoped)'; }

function auditFilteredGroups(){
  const groups=(state.audit&&state.audit.groups)||[];
  const checks=(state.audit&&state.audit.checks)||{};
  const q=(state.auditSearch||'').toLowerCase().trim();
  const nsScope=state.auditNsScope||'ALL';
  return groups.map(g=>{
    if(nsScope!=='ALL'&&auditNsKey(g.namespace)!==nsScope) return null;
    const findings=(g.findings||[]).filter(f=>auditFindingMatches(f,checks));
    if(findings.length===0) return null;
    if(q){
      const hay=[
        g.kind,g.name,g.namespace||'',
        ...findings.map(f=>{
          const meta=checks[f.checkID]||{};
          return [f.message,f.category,f.severity,meta.title,meta.description,meta.remediation,f.checkID].join(' ');
        })
      ].join(' ').toLowerCase();
      if(!hay.includes(q)) return null;
    }
    return {
      ...g,findings,
      danger:findings.filter(f=>f.severity==='danger').length,
      warning:findings.filter(f=>f.severity==='warning').length
    };
  }).filter(Boolean);
}

function auditNamespaceIndex(groups){
  const m={};
  groups.forEach(g=>{
    const ns=auditNsKey(g.namespace);
    if(!m[ns]) m[ns]={resources:0,danger:0,warning:0};
    m[ns].resources++;
    m[ns].danger+=g.danger||0;
    m[ns].warning+=g.warning||0;
  });
  return Object.entries(m).sort((a,b)=>{
    if(b[1].danger!==a[1].danger) return b[1].danger-a[1].danger;
    if(b[1].warning!==a[1].warning) return b[1].warning-a[1].warning;
    return a[0].localeCompare(b[0]);
  });
}

function auditGroupedByNamespace(groups){
  const m={};
  groups.forEach(g=>{
    const ns=auditNsKey(g.namespace);
    if(!m[ns]) m[ns]=[];
    m[ns].push(g);
  });
  return Object.entries(m).sort((a,b)=>{
    const ad=a[1].reduce((n,g)=>n+g.danger,0), bd=b[1].reduce((n,g)=>n+g.danger,0);
    if(bd!==ad) return bd-ad;
    const aw=a[1].reduce((n,g)=>n+g.warning,0), bw=b[1].reduce((n,g)=>n+g.warning,0);
    if(bw!==aw) return bw-aw;
    return a[0].localeCompare(b[0]);
  });
}

function auditFrameworksAvailable(checks){
  const set={};
  Object.values(checks||{}).forEach(c=>(c.frameworks||[]).forEach(fw=>{ set[fw]=true; }));
  return Object.keys(set).sort();
}

async function loadAudit(){
  state.audit=null; state.auditError=null; auditResetFilters(); state.auditExpandedNS={}; state.auditExpanded={}; render();
  try{
    const [audit,settings]=await Promise.all([
      api('/api/audit?namespace='+encodeURIComponent(state.ns)),
      api('/api/settings/audit')
    ]);
    state.audit=audit; state.auditSettings=settings;
    const groups=audit.groups||[];
    if(groups.length>0){
      const byNs=auditGroupedByNamespace(groups.map(g=>({...g,danger:(g.findings||[]).filter(f=>f.severity==='danger').length,warning:(g.findings||[]).filter(f=>f.severity==='warning').length})));
      if(byNs.length) state.auditExpandedNS[byNs[0][0]]=true;
    }
  }catch(e){
    console.error(e);
    state.auditError='Failed to load audit data — restart the web server after rebuilding.';
  }
  render();
}

async function exportAuditPDF(){
  try{
    const r=await fetch('/api/audit/export.pdf?namespace='+encodeURIComponent(state.ns));
    if(!r.ok) throw new Error('export failed');
    const blob=await r.blob();
    const url=URL.createObjectURL(blob);
    const a=document.createElement('a');
    a.href=url;
    a.download='kubectl-inventory-posture-'+new Date().toISOString().slice(0,10)+'.pdf';
    document.body.appendChild(a); a.click(); a.remove();
    URL.revokeObjectURL(url);
  }catch(e){ console.error(e); alert('PDF export failed'); }
}

function renderPostureBanner(){
  const p=(state.audit&&state.audit.posture)||{};
  const inv=state.audit&&state.audit.inventoryFindings||0;
  const compound=state.audit&&state.audit.compoundFindings||0;
  const cls=p.score>=75?'clean':p.score>=50?'susp':'dang';
  return h('div',{class:'posture-banner stat-block '+cls},[
    h('div',{class:'posture-banner-main'},[
      h('div',{class:'posture-grade'},[p.grade||'—']),
      h('div',{},[
        h('div',{class:'posture-headline'},[p.headline||'Posture Index']),
        h('div',{class:'posture-desc'},[p.description||''])
      ]),
      h('div',{class:'posture-score'},[String(p.score!=null?p.score:'—')])
    ]),
    h('div',{class:'posture-meta'},[
      inv>0?h('span',{class:'sig sig-GEN'},['INVENTORY ',h('strong',{},[String(inv)])]):null,
      compound>0?h('span',{class:'sig sig-DANG'},['COMPOUND ',h('strong',{},[String(compound)])]):null
    ])
  ]);
}

function renderActionQueue(){
  const queue=(state.audit&&state.audit.actionQueue)||[];
  if(!queue.length) return null;
  const top=queue.slice(0,8);
  return h('div',{class:'action-queue'},[
    h('div',{class:'action-queue-hdr'},[h('span',{class:'sidebar-title'},['Priority action queue']),h('span',{class:'audit-ns-meta'},['impact-ranked fixes'])]),
    h('div',{class:'action-queue-list'},[...top.map(item=>h('div',{class:'action-queue-item'+(item.compound?' compound':'')},[
      h('span',{class:'action-rank'},['#'+item.rank]),
      h('div',{class:'action-body'},[
        h('div',{class:'action-title'},[item.title]),
        h('div',{class:'action-resource'},[item.kind+'/'+item.name+(item.namespace?' · '+item.namespace:'')]),
        h('div',{class:'action-reason'},[item.reason])
      ]),
      h('span',{class:'sig sig-'+(item.severity==='danger'?'DANG':'SUSP')},[item.severity==='danger'?'FIX':'NOTE'])
    ]))])
  ]);
}

function renderAuditSummary(filtered){
  const s=(state.audit&&state.audit.summary)||{danger:0,warning:0,categories:{}};
  const useFiltered=filtered&&filtered.length>=0&&(auditHasChipFilters()||(state.auditSearch||'').trim()||state.auditNsScope!=='ALL');
  let danger=0, warning=0;
  const catCounts={Security:{danger:0,warning:0},Reliability:{danger:0,warning:0},Efficiency:{danger:0,warning:0}};
  if(useFiltered){
    filtered.forEach(f=>{
      if(f.severity==='danger') danger++;
      else if(f.severity==='warning') warning++;
      if(catCounts[f.category]) catCounts[f.category][f.severity==='danger'?'danger':'warning']++;
    });
  }else{
    danger=s.danger||0; warning=s.warning||0;
    ['Security','Reliability','Efficiency'].forEach(cat=>{
      const cs=(s.categories&&s.categories[cat])||{};
      catCounts[cat]={danger:cs.danger||0,warning:cs.warning||0};
    });
  }
  const cats=[['Security','Hardening'],['Reliability','Resilience'],['Efficiency','Right-sizing']];
  return h('div',{class:'audit-summary-grid'},[
    h('div',{class:'audit-summary-total stat-block '+(danger>0?'dang':'clean')},[
      h('div',{class:'stat-hdr'},[h('span',{class:'stat-title'},['Must-fix'])]),
      h('div',{class:'stat-value'},[String(danger)])
    ]),
    h('div',{class:'audit-summary-total stat-block '+(warning>0?'susp':'clean')},[
      h('div',{class:'stat-hdr'},[h('span',{class:'stat-title'},['Advisory'])]),
      h('div',{class:'stat-value'},[String(warning)])
    ]),
    ...cats.map(([cat,label])=>{
      const cs=catCounts[cat]||{danger:0,warning:0};
      const cls=cs.danger>0?'dang':cs.warning>0?'susp':'clean';
      return h('div',{class:'audit-cat-block stat-block '+cls},[
        h('div',{class:'stat-hdr'},[h('span',{class:'stat-title'},[label])]),
        h('div',{class:'audit-cat-counts'},[
          cs.danger>0?h('span',{class:'sig sig-DANG'},['FIX ',h('strong',{},[String(cs.danger)])]):null,
          cs.warning>0?h('span',{class:'sig sig-SUSP'},['NOTE ',h('strong',{},[String(cs.warning)])]):null,
          cs.danger===0&&cs.warning===0?h('span',{class:'audit-pass'},['CLEAR']):null
        ])
      ]);
    })
  ]);
}

function renderAuditFinding(f, checks, enriched){
  const meta=(checks&&checks[f.checkID])||{};
  const extra=enriched||{};
  const lens=extra.lens||(Object.entries(AUDIT_LENS_MAP).find(([,v])=>v===f.category)||[])[0]||f.category;
  const isInventory=(f.checkID||'').startsWith('inventory:');
  return h('div',{class:'audit-finding'+(extra.compound?' compound':'')},[
    h('div',{class:'audit-finding-hdr'},[
      h('span',{class:'sig sig-'+(f.severity==='danger'?'DANG':'SUSP')},[f.severity==='danger'?'MUST-FIX':'ADVISORY']),
      h('span',{class:'audit-check-title'},[meta.title||f.checkID]),
      h('span',{class:'audit-cat-chip'},[lens]),
      isInventory?h('span',{class:'audit-cat-chip inv'},['inventory DNA']):null,
      extra.compound?h('span',{class:'audit-cat-chip compound'},['compound risk']):null,
      extra.inventorySignal&&extra.inventorySignal!=='CLEAN'?h('span',{class:'sig sig-'+extra.inventorySignal},[extra.inventorySignal]):null
    ]),
    h('div',{class:'audit-finding-msg'},[f.message]),
    meta.description?h('div',{class:'audit-finding-desc'},[meta.description]):null,
    meta.remediation?h('div',{class:'audit-finding-fix'},['\u003e ',meta.remediation]):null,
    (meta.frameworks&&meta.frameworks.length)?h('div',{class:'audit-frameworks'},[...meta.frameworks.map(fw=>h('span',{class:'audit-fw-chip'},[fw]))]):null
  ]);
}

function renderAuditResourceGroup(g, checks, enrichedIndex){
  const key=auditGroupKey(g);
  const open=!!state.auditExpanded[key];
  return h('div',{class:'audit-group'+(open?' open':'')},[
    h('div',{class:'audit-group-hdr',onClick:()=>{ state.auditExpanded[key]=!open; render(); }},[
      h('span',{class:'audit-expand'},[open?'▼':'▶']),
      h('span',{class:'audit-res-id'},[g.kind+'/'+g.name]),
      h('div',{class:'audit-group-badges'},[
        g.danger>0?h('span',{class:'sig sig-DANG'},['FIX ',h('strong',{},[String(g.danger)])]):null,
        g.warning>0?h('span',{class:'sig sig-SUSP'},['NOTE ',h('strong',{},[String(g.warning)])]):null
      ]),
      h('button',{class:'btn btn-sm',onClick:(e)=>{ e.stopPropagation(); openDrill('',g.kind); }},['Inspect'])
    ]),
    open?h('div',{class:'audit-group-body'},[...(g.findings||[]).map(f=>renderAuditFinding(f,checks, enrichedIndex&&enrichedIndex[f.checkID+'|'+key]))]):null
  ]);
}

function enrichedIndexForFindings(){
  const idx={};
  (state.audit&&state.audit.enrichedFindings||[]).forEach(ef=>{
    const k=ef.checkID+'|'+auditGroupKey({kind:ef.kind,namespace:ef.namespace,name:ef.name});
    idx[k]=ef;
  });
  return idx;
}

function renderAuditNamespaceSection(ns, groups, checks, nsPosture){
  const open=!!state.auditExpandedNS[ns];
  const danger=groups.reduce((n,g)=>n+g.danger,0);
  const warning=groups.reduce((n,g)=>n+g.warning,0);
  const grade=nsPosture&&nsPosture.grade;
  const enrichedIdx=enrichedIndexForFindings();
  return h('div',{class:'audit-ns-section'+(open?' open':'')},[
    h('div',{class:'audit-ns-hdr',onClick:()=>{
      state.auditExpandedNS[ns]=!open;
      if(!open){
        groups.forEach(g=>{ state.auditExpanded[auditGroupKey(g)]=true; });
      }
      render();
    }},[
      h('span',{class:'audit-expand'},[open?'▼':'▶']),
      h('span',{class:'audit-ns-title'},[ns]),
      grade?h('span',{class:'posture-ns-grade'},['Grade '+grade]):null,
      h('span',{class:'audit-ns-meta'},[groups.length+' resource'+(groups.length!==1?'s':'')]),
      h('div',{class:'audit-group-badges'},[
        danger>0?h('span',{class:'sig sig-DANG'},['FIX ',h('strong',{},[String(danger)])]):null,
        warning>0?h('span',{class:'sig sig-SUSP'},['NOTE ',h('strong',{},[String(warning)])]):null
      ])
    ]),
    open?h('div',{class:'audit-ns-body'},[...groups.map(g=>renderAuditResourceGroup(g,checks,enrichedIdx))]):null
  ]);
}

function renderAuditFilterChip(label, active, onClick, cls){
  return h('button',{class:'filter-btn audit-chip'+(active?' active':'')+(cls?' '+cls:''),onClick},[label]);
}

function renderAuditNamespaceRail(allGroups){
  const index=auditNamespaceIndex(allGroups);
  const nsPostures={};
  (state.audit&&state.audit.namespacePostures||[]).forEach(np=>{ nsPostures[np.namespace]=np; });
  const totalDanger=allGroups.reduce((n,g)=>n+(g.danger||0),0);
  const totalWarning=allGroups.reduce((n,g)=>n+(g.warning||0),0);
  const totalResources=allGroups.length;
  const clusterGrade=state.audit&&state.audit.posture&&state.audit.posture.grade;
  return h('aside',{class:'audit-ns-rail'},[
    h('div',{class:'sidebar-title'},['Namespace heatmap']),
    h('div',{class:'audit-ns-rail-list'},[
      h('div',{class:'audit-ns-rail-item'+(state.auditNsScope==='ALL'?' active':''),onClick:()=>{ state.auditNsScope='ALL'; render(); }},[
        h('span',{},['All namespaces']),
        h('span',{class:'audit-ns-rail-meta'},[
          clusterGrade?h('span',{class:'posture-ns-grade'},[clusterGrade]):null,
          h('span',{class:'audit-ns-rail-count'},[String(totalResources)])
        ])
      ]),
      ...index.map(([ns,stats])=>{
        const np=nsPostures[ns];
        return h('div',{class:'audit-ns-rail-item'+(state.auditNsScope===ns?' active':''),onClick:()=>{ state.auditNsScope=ns; state.auditExpandedNS[ns]=true; render(); }},[
          h('span',{},[ns]),
          h('span',{class:'audit-ns-rail-meta'},[
            np?h('span',{class:'posture-ns-grade grade-'+((np.grade||'').toLowerCase())},[np.grade||'—']):null,
            h('span',{class:'audit-ns-rail-count'},[String(stats.resources)])
          ])
        ]);
      })
    ]),
    h('div',{class:'audit-ns-rail-foot'},[
      h('span',{class:'sig sig-DANG'},['FIX ',h('strong',{},[String(totalDanger)])]),
      h('span',{class:'sig sig-SUSP'},['NOTE ',h('strong',{},[String(totalWarning)])])
    ])
  ]);
}

function openAuditSettings(){
  const s=state.auditSettings||{ignoredNamespaces:[],disabledChecks:[]};
  state.auditSettingsDraft={ignoredNamespaces:[...(s.ignoredNamespaces||[])],disabledChecks:[...(s.disabledChecks||[])],newNs:''};
  state.auditShowSettings=true;
  render();
}

function renderAuditSettingsModal(){
  if(!state.auditShowSettings) return null;
  const draft=state.auditSettingsDraft||{ignoredNamespaces:[],disabledChecks:[],newNs:''};
  const checks=state.audit&&state.audit.checks?Object.values(state.audit.checks).sort((a,b)=>(a.title||'').localeCompare(b.title||'')):[];
  return h('div',{class:'overlay',onClick:()=>{ state.auditShowSettings=false; render(); }},[
    h('div',{class:'modal audit-settings-modal',onClick:(e)=>e.stopPropagation()},[
      h('div',{class:'modal-hdr'},[
        h('span',{class:'modal-title'},['Audit Settings']),
        h('button',{class:'close-btn',onClick:()=>{ state.auditShowSettings=false; render(); }},['×'])
      ]),
      h('div',{class:'cfg-section'},[
        h('label',{class:'cfg-label'},['Ignored Namespaces']),
        h('p',{class:'audit-settings-help'},['Findings in these namespaces are hidden from all views.']),
        h('div',{class:'audit-ns-list'},[
          ...(draft.ignoredNamespaces||[]).map(ns=>h('div',{class:'audit-ns-item'},[
            h('span',{},[ns]),
            h('button',{class:'btn btn-sm',onClick:()=>{ draft.ignoredNamespaces=draft.ignoredNamespaces.filter(n=>n!==ns); render(); }},['Remove'])
          ])),
          (draft.ignoredNamespaces||[]).length===0?h('div',{class:'audit-settings-empty'},['No namespaces ignored.']):null
        ]),
        h('div',{class:'audit-ns-add'},[
          h('input',{class:'text-input',type:'text',placeholder:'Add namespace or pattern (e.g. *-system)',value:draft.newNs||'',onInput:(e)=>{ draft.newNs=e.target.value; }}),
          h('button',{class:'btn btn-primary',onClick:()=>{
            const ns=(draft.newNs||'').trim();
            if(!ns||draft.ignoredNamespaces.includes(ns)) return;
            draft.ignoredNamespaces.push(ns); draft.newNs=''; render();
          }},['Add'])
        ])
      ]),
      h('div',{class:'cfg-section'},[
        h('label',{class:'cfg-label'},['Enabled Checks']),
        h('p',{class:'audit-settings-help'},['Uncheck to disable specific checks globally.']),
        h('div',{class:'audit-check-list'},[
          ...checks.map(c=>{
            const enabled=!(draft.disabledChecks||[]).includes(c.id);
            return h('label',{class:'audit-check-row'},[
              h('input',{type:'checkbox',checked:enabled,onChange:()=>{
                if(enabled) draft.disabledChecks=[...(draft.disabledChecks||[]),c.id];
                else draft.disabledChecks=(draft.disabledChecks||[]).filter(x=>x!==c.id);
                render();
              }}),
              h('span',{class:'audit-check-row-title'},[c.title]),
              h('span',{class:'audit-check-row-desc'},[c.description])
            ]);
          })
        ])
      ]),
      h('div',{class:'modal-actions'},[
        h('button',{class:'btn',onClick:()=>{ state.auditShowSettings=false; render(); }},['Cancel']),
        h('button',{class:'btn btn-primary',onClick:async()=>{
          const payload={ignoredNamespaces:draft.ignoredNamespaces,disabledChecks:draft.disabledChecks};
          try{
            state.auditSettings=await apiPut('/api/settings/audit',payload);
            state.auditShowSettings=false;
            await loadAudit();
          }catch(e){ console.error(e); }
        }},['Save'])
      ])
    ])
  ]);
}

function renderAudit(){
  const inv=state.inv||{};
  if(!state.audit && !state.auditError) return h('div',{class:'cockpit'},[renderLoading('RUNNING AUDIT CHECKS...')]);
  if(state.auditError) return h('div',{class:'cockpit'},[
    h('header',{class:'page-header'},[h('div',{class:'branding'},[h('h1',{class:'red'},['kubectl-inventory // audit']),h('div',{class:'meta-row'},[h('span',{},['CTX: '+(inv.context||'—')])])])]),
    h('div',{class:'audit-empty',style:{color:'var(--red)'}},['// '+state.auditError])
  ]);

  const checks=(state.audit&&state.audit.checks)||{};
  const allGroupsRaw=(state.audit&&state.audit.groups)||[];
  const allGroups=allGroupsRaw.map(g=>({
    ...g,
    danger:(g.findings||[]).filter(f=>f.severity==='danger').length,
    warning:(g.findings||[]).filter(f=>f.severity==='warning').length
  }));
  const groups=auditFilteredGroups();
  const filteredFindings=groups.flatMap(g=>g.findings||[]);
  const total=(state.audit.summary&&state.audit.summary.danger||0)+(state.audit.summary&&state.audit.summary.warning||0);
  const ignored=(state.auditSettings&&state.auditSettings.ignoredNamespaces||[]).length;
  const frameworks=auditFrameworksAvailable(checks);
  const namespaced=auditGroupedByNamespace(groups);
  const hasFilters=auditHasChipFilters()||(state.auditSearch||'').trim()||state.auditNsScope!=='ALL';

  const toolbar=h('div',{class:'audit-toolbar'},[
    h('div',{class:'audit-toolbar-row'},[
      h('input',{class:'text-input audit-search',type:'text',placeholder:'Search rules, resources, namespaces... (press /)',value:state.auditSearch||'',onInput:(e)=>{ state.auditSearch=e.target.value; render(); }}),
      hasFilters?h('button',{class:'btn btn-sm',onClick:()=>{ auditResetFilters(); render(); }},['Clear filters']):null
    ]),
    h('div',{class:'audit-filter-groups'},[
      h('div',{class:'audit-filter-row'},[
        h('span',{class:'audit-filter-label'},['Lens']),
        renderAuditFilterChip('Show all',!auditHasChipFilters(),()=>{ state.auditLens={}; state.auditPriority={}; state.auditFramework={}; render(); }),
        ...AUDIT_LENS_LABELS.map(([key,label])=>renderAuditFilterChip(label,!!state.auditLens[key],()=>auditToggleFilter('auditLens',key)))
      ]),
      h('div',{class:'audit-filter-row'},[
        h('span',{class:'audit-filter-label'},['Priority']),
        ...AUDIT_PRIORITY_LABELS.map(([key,label])=>renderAuditFilterChip(label,!!state.auditPriority[key],()=>auditToggleFilter('auditPriority',key),key==='mustfix'?'chip-danger':'chip-warn'))
      ]),
      frameworks.length?h('div',{class:'audit-filter-row'},[
        h('span',{class:'audit-filter-label'},['Standards']),
        ...frameworks.map(fw=>renderAuditFilterChip(fw,!!state.auditFramework[fw],()=>auditToggleFilter('auditFramework',fw)))
      ]):null
    ])
  ]);

  const nsPostures={};
  (state.audit&&state.audit.namespacePostures||[]).forEach(np=>{ nsPostures[np.namespace]=np; });

  const content=groups.length===0
    ? h('div',{class:'audit-empty'},[total===0?'// POSTURE CLEAR — NO ACTION ITEMS IN SCOPE':(hasFilters?'// NO FINDINGS MATCH THE CURRENT SCOPE OR FILTERS':'// NO FINDINGS IN THIS VIEW')])
    : h('div',{class:'audit-namespaced-list'},[...namespaced.map(([ns,nsGroups])=>renderAuditNamespaceSection(ns,nsGroups,checks,nsPostures[ns]))]);

  const main=h('div',{class:'audit-main'},[
    !hasFilters?renderActionQueue():null,
    toolbar,
    content,
    h('div',{class:'audit-footer'},[
      'Showing '+filteredFindings.length+' finding'+(filteredFindings.length!==1?'s':'')+' across '+groups.length+' resource'+(groups.length!==1?'s':'')+
      (hasFilters?' (filtered)':'')
    ])
  ]);

  const cockpit=h('div',{class:'cockpit'},[
    h('header',{class:'page-header'},[
      h('div',{class:'branding'},[
        h('h1',{class:total>0?'red':''},['kubectl-inventory // posture']),
        h('div',{class:'meta-row'},[
          h('span',{},['CTX: '+(inv.context||'—')]),
          h('span',{},['NS: '+(state.ns==='*'?'ALL':state.ns)]),
          h('span',{},['MODE: INVENTORY_POSTURE'])
        ]),
        h('div',{class:'audit-subtitle'},['Posture Index blends best-practice checks with inventory-native signals — dangling owners, stuck deletes, GitOps drift — ranked into an action queue.'])
      ]),
      h('div',{class:'audit-header-actions'},[
        h('button',{class:'btn btn-sm btn-primary',onClick:exportAuditPDF,title:'Download posture PDF'},['Export PDF']),
        ignored>0?h('button',{class:'btn btn-sm',onClick:openAuditSettings},[ignored+' NS HIDDEN']):null,
        h('button',{class:'btn btn-sm',onClick:openAuditSettings,title:'Audit settings'},['⚙ Settings']),
        h('div',{class:'status-ind '+(total>0?'red':'')},[h('div',{class:'dot '+(total>0?'red':'')},[]),total>0?'ACTION ITEMS: '+total:'POSTURE CLEAR'])
      ])
    ]),
    !hasFilters?renderPostureBanner():null,
    renderAuditSummary(filteredFindings),
    h('div',{class:'audit-workspace'},[
      renderAuditNamespaceRail(allGroups),
      main
    ])
  ]);

  const modal=renderAuditSettingsModal();
  if(modal){
    const wrap=document.createElement('div');
    wrap.appendChild(cockpit);
    wrap.appendChild(modal);
    return wrap;
  }
  return cockpit;
}

document.addEventListener('keydown',(e)=>{
  if(e.key==='/'&&!e.ctrlKey&&!e.metaKey&&state.screen==='audit'&&document.activeElement&&document.activeElement.tagName!=='INPUT'){
    e.preventDefault();
    const el=document.querySelector('.audit-search');
    if(el) el.focus();
  }
});

function render(){
  const app=document.getElementById('app');
  app.innerHTML='';
  app.appendChild(renderNav());
  try{
    const screens={connect:renderConnect,radar:renderRadar,audit:renderAudit,drill:renderDrill,namespaces:renderNamespaces,health:renderHealth};
    app.appendChild((screens[state.screen]||renderConnect)());
  }catch(e){ console.error('render error:',e); app.appendChild(h('div',{class:'loading'},[h('div',{class:'loading-text',style:{color:'var(--red)'}},['RENDER ERROR']),h('div',{class:'loading-sub'},[e.message])])); }
}

(async()=>{
  render();
  await loadConnect();
  if(state.inv){ state.screen='radar'; await loadRadar(); }
  render();
})();
