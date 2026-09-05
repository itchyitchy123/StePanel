(()=>{'use strict';
const target=document.querySelector('#siteOverview'),status=document.querySelector('#siteOverviewStatus');
if(!target||!status)return;
const read=async response=>{const text=await response.text();let data={};try{data=JSON.parse(text)}catch(error){}if(!response.ok)throw new Error(data.error||text||`Request failed (${response.status})`);return data};
const load=async()=>{const data=await read(await fetch('/api/sites/overview'));target.replaceChildren();for(const site of data.sites||[]){const card=document.createElement('article');card.className='site-card';const title=document.createElement('h3');title.textContent=site.site;const root=document.createElement('code');root.textContent=site.document_root;const domains=document.createElement('p');domains.textContent='Domains: '+((site.routes||[]).map(route=>route.domain).join(', ')||'none');const details=document.createElement('p');details.textContent=`${(site.applications||[]).length} app · ${site.database_count||0} database(s)`;card.append(title,root,domains,details);target.append(card)}status.textContent=data.sites&&data.sites.length?`${data.sites.length} managed site(s).`:'No managed sites yet.'};
load().catch(error=>{status.textContent=error.message});
})();
