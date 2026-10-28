'use strict';
const $=id=>document.getElementById(id);let snapshot=null,busy=false;const expandedBatches=new Set();
const names={pending:'待處理',processing:'處理中',approved:'已完成',review:'待發布',failed:'失敗',rejected:'已排除'};
const errors={no_source_person:'原照片未檢出人物，已排除',unwanted_people:'擴圖區域疑似新增人物，已排除',timeout:'處理逾時',network_error:'網路或服務請求失敗',validation_error:'圖片或設定驗證失敗',processing_error:'處理程序出錯',capture_date_out_of_range:'拍攝日期不在最近年限內'};
function node(tag,cls,text){const n=document.createElement(tag);if(cls)n.className=cls;if(text!==undefined)n.textContent=text;return n;}
function date(value){return value?new Date(value*1000).toLocaleString('zh-TW',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}):'—';}
function taken(value){return value?value.slice(0,10).replaceAll(':','-'):'日期不明';}
async function refresh(){if(busy)return;busy=true;$('refresh').disabled=true;try{const r=await fetch('/manage/api/overview',{credentials:'omit',cache:'no-store'});if(!r.ok)throw new Error('暫時無法讀取 NAS，稍後再重新整理。');snapshot=await r.json();render(snapshot);$('notice').hidden=snapshot.serving&&snapshot.published_mode;if(!snapshot.serving)$('notice').textContent='供圖已暫停，電視繼續使用有效的舊快取。';else if(!snapshot.published_mode)$('notice').textContent='尚未啟用成品批次發布。';}catch(error){$('notice').textContent=error.message+(snapshot?' 畫面保留上次資料。':'');$('notice').hidden=false;}finally{busy=false;$('refresh').disabled=false;}}
function render(d){
 $('summary').textContent=(d.serving?'供圖正常':'供圖暫停')+' · 只顯示最近 '+d.capture_years+' 年的成品照片';
 for(const [id,label,value] of [['photo-count','目前照片',d.current_photos],['batch-count',d.pool_mode?'成品池':'有效批次',d.available_batches+' / '+d.max_batches],['pending-count','待處理',d.counts.pending||0],['processing-count','處理中',d.counts.processing||0]]){const e=$(id);e.replaceChildren(document.createTextNode(label+' '),node('strong','',String(value)));}
 $('pool-progress').hidden=!d.pool_mode;
 if(d.pool_mode)$('pool-progress').textContent=d.available_batches<d.max_batches?'正在補足成品池 · 下一批已完成 '+d.next_batch_ready+' / '+d.batch_target_size+' 張':'成品池已備好 '+d.available_batches+' 批 · '+(d.next_prepare_at>d.server_time?'下次補貨 '+date(d.next_prepare_at):'正在準備替換批次 · 已完成 '+d.next_batch_ready+' / '+d.batch_target_size+' 張');
 $('updated').textContent='更新 '+date(d.server_time);const current=d.batches.find(b=>b.current);$('current-heading').textContent=d.serving?'目前的照片':'已保存的照片';$('current-details').textContent=current?'批次 '+current.batch_id.slice(0,8)+' · '+date(current.published_at)+' 發布':'';$('no-photos').hidden=!!current;$('photos').replaceChildren();
 if(current)appendPhotos($('photos'),current.photos,'目前批次');
 $('batch-summary').textContent=d.pool_mode?'成品池 · '+d.available_batches+' / '+d.max_batches+' 批':'批次記錄 · '+d.available_batches+' 份可用';$('batches').replaceChildren();
 for(const b of d.batches){
  const detail=node('details','batch-detail'),summary=node('summary','batch-row'),left=node('div'),right=node('div','right');
  detail.open=expandedBatches.has(b.batch_id);
  left.append(node('code','',b.batch_id.slice(0,12)),node('p','',date(b.published_at)+' 發布'));
  if(b.reason)left.append(node('p','',b.reason));
  right.append(node('span','pill '+(b.current?'current':''),b.bootstrap?'過渡批次（不計入成品池）':!b.available?'不供應':b.current?'目前供應':'備用批次'),node('p','',b.photos.length+' 張 · 展開看照片'));
  summary.append(left,right);detail.append(summary);
  const gallery=node('div','photos');let loaded=false;
  const load=()=>{if(!loaded){appendPhotos(gallery,b.photos,'批次 '+b.batch_id.slice(0,8));loaded=true;}};
  detail.addEventListener('toggle',()=>{if(detail.open){expandedBatches.add(b.batch_id);load();}else expandedBatches.delete(b.batch_id);});
  if(detail.open)load();detail.append(gallery);$('batches').append(detail);
 }
 if(!d.batches.length)$('batches').append(node('p','empty','還沒有已發布批次。'));$('task-summary').textContent='任務狀態 · 待處理 '+(d.counts.pending||0)+' · 失敗 '+(d.counts.failed||0);renderJobs();
}
function appendPhotos(container,photos,label){
 for(const a of photos){
  const figure=node('figure'),button=node('button'),image=node('img');button.type='button';
  button.setAttribute('aria-label',label+'：預覽 '+taken(a.taken)+' 的照片');
  image.src='/manage/thumbs/asset-'+a.sha256+'.jpg';image.alt=taken(a.taken)+' 拍攝的照片';image.loading='lazy';
  button.append(image);button.addEventListener('click',()=>{$('preview-title').textContent=label+' · '+taken(a.taken)+' 拍攝';$('preview-image').src='/manage/api/images/'+a.sha256+'.jpg';$('preview').showModal();});
  figure.append(button,node('figcaption','',taken(a.taken)+' 拍攝'));container.append(figure);
 }
}
function renderJobs(){if(!snapshot)return;const filter=$('job-filter').value,old=$('include-old').checked,jobs=snapshot.jobs.filter(j=>(old||j.current_recipe)&&(filter==='all'||j.status===filter));$('jobs').replaceChildren();$('jobs-empty').hidden=jobs.length>0;for(const j of jobs){const row=node('tr'),id=node('td');id.append(node('code','',j.id.slice(0,10)));if(!j.current_recipe)id.append(node('p','','舊測試'));const status=node('td'),expired=j.status==='processing'&&j.lease_until<snapshot.server_time;status.append(node('span','pill '+(expired?'expired':j.status),expired?'租約過期':names[j.status]||j.status));let note='';if(j.status==='failed')note=errors[j.error_code]||'早期記錄未保存詳細原因';else if(j.status==='processing')note=expired?'等待重新領取':'正在處理';else if(j.status==='rejected')note=errors[j.error_code]||'不進入電視批次';else if(j.status==='review')note='尚未發布';row.append(id,status,node('td','',taken(j.taken)),node('td','',date(j.updated)),node('td','',note));$('jobs').append(row);}}
$('close-preview').addEventListener('click',()=>$('preview').close());$('preview').addEventListener('click',e=>{if(e.target===$('preview'))$('preview').close();});$('refresh').addEventListener('click',refresh);$('job-filter').addEventListener('change',renderJobs);$('include-old').addEventListener('change',renderJobs);setInterval(()=>{if(document.visibilityState==='visible')refresh();},30000);document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='visible')refresh();});refresh();
