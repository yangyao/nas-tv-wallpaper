import {Timeline,Playback,AHEAD,fit,soften} from '/frame/core.mjs?v=1.6.1';

const $=id=>document.getElementById(id);
const timeline=new Timeline(),playback=new Playback();
const controls=$('controls'),track=$('track'),selection=$('selection');
const scenes=[...document.querySelectorAll('.scene')],buttons=new Map(),thumbs=new Map();
const reduced=matchMedia('(prefers-reduced-motion: reduce)');
let initialized=false,database=null,fetching=false,moving=false,stopped=false,activeScene=0,displayed=null;
let retryAt=0;
let hideTimer,cursorTimer,pumpTimer,requestController,pointerDown=null,blockClickUntil=0;
let railStart=0,lastRailEntry=null,lastRailIndex=0,lastRailSlots=0;
const pending=[],memory=new Map();
const MAX_RAW=40*1024*1024,MAX_DISPLAY=2*1024*1024,MAX_THUMB=32*1024;

function request(req){return new Promise((resolve,reject)=>{req.onsuccess=()=>resolve(req.result);req.onerror=()=>reject(req.error);});}
function complete(tx){return new Promise((resolve,reject)=>{tx.oncomplete=resolve;tx.onerror=()=>reject(tx.error);tx.onabort=()=>reject(tx.error);});}
async function openStore(){
 try{
  const req=indexedDB.open('nas-photo-frame',1);
  req.onupgradeneeded=()=>{req.result.createObjectStore('photos',{keyPath:'id'});req.result.createObjectStore('state');};
  database=await request(req);database.onversionchange=()=>{database.close();database=null;};
 }catch{/* Private mode or unavailable storage: bounded session memory remains usable. */}
}
async function getPhoto(id){
 if(memory.has(id))return memory.get(id);
 try{
  const record=await request(database.transaction('photos').objectStore('photos').get(id));
  if(!record)return null;
  // Byte buffers also work in WebKit stores that cannot persist canvas Blobs.
  for(const name of ['photo','thumb','background'])if(record[name] instanceof ArrayBuffer)record[name]=new Blob([record[name]],{type:'image/jpeg'});
  return record;
 }catch{return null;}
}
async function putPhoto(record){
 try{
  const stored={id:record.id,photo:await record.photo.arrayBuffer(),thumb:await record.thumb.arrayBuffer(),background:await record.background.arrayBuffer()};
  const tx=database.transaction('photos','readwrite'),done=complete(tx);tx.objectStore('photos').put(stored);await done;
 }
 catch{memory.set(record.id,record);}
}
function saveState(){
 if(!initialized||!timeline.current)return;
 try{const tx=database.transaction('state','readwrite');complete(tx).catch(()=>{});tx.objectStore('state').put({ids:timeline.items.map(p=>p.id),index:timeline.index},'timeline');}catch{}
}
async function cleanup(){
 const keep=new Set(timeline.items.map(p=>p.id));
 for(const [id,value] of thumbs)if(!keep.has(id)){URL.revokeObjectURL(value.url);thumbs.delete(id);}
 for(const id of memory.keys())if(!keep.has(id))memory.delete(id);
 try{
  const tx=database.transaction('photos','readwrite'),done=complete(tx),store=tx.objectStore('photos');
  store.getAllKeys().onsuccess=e=>{for(const id of e.target.result)if(!keep.has(id))store.delete(id);};
  await done;
 }catch{}
}
async function thumbnail(record){
 if(thumbs.has(record.id))return thumbs.get(record.id);
 const url=URL.createObjectURL(record.thumb),image=new Image();image.draggable=false;image.alt='';image.src=url;
 try{await image.decode();}catch(error){URL.revokeObjectURL(url);throw error;}
 const result={url,image};thumbs.set(record.id,result);return result;
}
function canvas(width,height){const c=document.createElement('canvas');c.width=width;c.height=height;return c;}
function drawFit(context,image,width,height,cover=false){const r=fit(image.width,image.height,width,height,cover);context.drawImage(image,r.x,r.y,r.width,r.height);}
function jpeg(c,quality){return new Promise((resolve,reject)=>c.toBlob(blob=>blob?resolve(blob):reject(Error('encode')),'image/jpeg',quality));}
async function normalize(blob,item){
 // JPEG EXIF orientation is applied by the browser decoder exactly once.
 const rotated=item.orientation>=5&&item.orientation<=8;
 const sourceW=rotated?item.height:item.width,sourceH=rotated?item.width:item.height;
 const factor=Math.min(1,2560/sourceW,1440/sourceH),width=Math.max(1,Math.round(sourceW*factor)),height=Math.max(1,Math.round(sourceH*factor));
 const bitmap=await createImageBitmap(blob,{imageOrientation:'from-image',resizeWidth:width,resizeHeight:height,resizeQuality:'high'});
 try{
  const display=canvas(width,height);display.getContext('2d').drawImage(bitmap,0,0,width,height);
  let photo;for(const quality of [.86,.71,.56,.41]){photo=await jpeg(display,quality);if(photo.size<=MAX_DISPLAY)break;}
  if(photo.size>MAX_DISPLAY)throw Error('display limit');
  const small=canvas(192,144);drawFit(small.getContext('2d'),bitmap,192,144,true);const thumb=await jpeg(small,.78);
  if(thumb.size>MAX_THUMB)throw Error('thumbnail limit');
  const background=canvas(128,72),ctx=background.getContext('2d',{willReadFrequently:true});
  drawFit(ctx,bitmap,128,72,true);const pixels=ctx.getImageData(0,0,128,72);pixels.data.set(soften(pixels.data,128,72));ctx.putImageData(pixels,0,0);
  const back=await jpeg(background,.85);
  return {id:item.id,photo,thumb,background:back};
 }finally{bitmap.close();}
}
async function boundedBody(response,limit){
 if(Number(response.headers.get('content-length'))>limit)throw Error('body limit');
 const reader=response.body.getReader(),parts=[];let total=0;
 try{while(true){const {done,value}=await reader.read();if(done)break;total+=value.length;if(total>limit)throw Error('body limit');parts.push(value);}}catch(error){await reader.cancel();throw error;}
 return new Blob(parts,{type:response.headers.get('content-type')??'application/octet-stream'});
}
function resetClock(){playback.reset(performance.now());}
function setControls(show){
 controls.classList.toggle('visible',show);controls.inert=!show;playback.controls=show;
 if(!show&&controls.contains(document.activeElement))document.activeElement.blur();
 resetClock();clearTimeout(hideTimer);if(!show){clearTimeout(cursorTimer);cursorTimer=setTimeout(()=>document.body.classList.add('quiet'),2000);}if(show)hideTimer=setTimeout(()=>setControls(false),4500);
}
function wakePointer(event){
 if(event.buttons)return;
 document.body.classList.remove('quiet');clearTimeout(cursorTimer);cursorTimer=setTimeout(()=>{if(!playback.controls)document.body.classList.add('quiet');},2000);
 if(event.clientY>innerHeight-200)setControls(true);
}
function renderRail(){
 const keep=new Set(timeline.items.map(p=>p.key));
 for(const [key,button] of buttons)if(!keep.has(key)){button.remove();buttons.delete(key);}
 for(const entry of timeline.items){
  let button=buttons.get(entry.key);
  if(!button){
   button=document.createElement('button');button.className='thumb';button.type='button';button.setAttribute('aria-label','查看這張照片');button.draggable=false;
   const image=thumbs.get(entry.id)?.image.cloneNode();if(image)button.append(image);
   button.addEventListener('click',()=>{setControls(true);void show(entry);});buttons.set(entry.key,button);track.append(button);
  }
  button.classList.toggle('current',entry===timeline.current);button.setAttribute('aria-current',entry===timeline.current?'true':'false');
 }
 const width=$('viewport').clientWidth,slots=Math.max(1,Math.min(7,Math.floor((width+12)/104)));
 if(timeline.current!==lastRailEntry||slots!==lastRailSlots){
  railStart=Math.max(0,Math.min(timeline.index-Math.floor(slots/2),timeline.items.length-slots));
 }else railStart=Math.max(0,railStart+timeline.index-lastRailIndex);
 lastRailEntry=timeline.current;lastRailIndex=timeline.index;lastRailSlots=slots;
 const start=railStart,visibleWidth=slots*104-12,offset=(width-visibleWidth)/2;
 track.style.transform=`translate3d(${offset-start*104}px,0,0)`;
 selection.style.transform=`translate3d(${offset+(timeline.index-start)*104-5}px,0,0)`;
 selection.classList.toggle('empty',timeline.items.length===0);
}
function paintScene(c,image,background){
 const factor=Math.min(devicePixelRatio||1,2560/Math.max(1,innerWidth),1440/Math.max(1,innerHeight));
 const width=Math.max(1,Math.round(innerWidth*factor)),height=Math.max(1,Math.round(innerHeight*factor));
 c.width=width;c.height=height;const ctx=c.getContext('2d',{alpha:false});ctx.drawImage(background,0,0,width,height);drawFit(ctx,image,width,height);
}
async function show(entry){
 if(moving||!entry||stopped)return false;
 moving=true;let image,background;
 try{
  const record=await getPhoto(entry.id);if(!record)throw Error('cache missing');
  image=await createImageBitmap(record.photo);background=await createImageBitmap(record.background);
  if(stopped||!timeline.items.includes(entry))throw Error('stale image');
  const next=scenes[1-activeScene],old=scenes[activeScene];
  paintScene(next,image,background);next.style.zIndex='2';next.style.opacity='1';old.style.zIndex='1';
  if(!timeline.select(entry))throw Error('stale selection');renderRail();saveState();resetClock();
  const previous=displayed;displayed={image,background,canvas:next};image=background=null;
  if(previous&&!reduced.matches){const transition=next.animate([{opacity:0},{opacity:1}],{duration:360,easing:'ease-in-out'});try{await transition.finished;}catch{}transition.cancel();}
  old.style.opacity='0';activeScene=1-activeScene;
  previous?.image.close();previous?.background.close();
  return true;
 }catch{
  image?.close();background?.close();return false;
 }finally{moving=false;void pump();}
}
function move(delta){resetClock();const next=timeline.atDelta(delta);if(next)void show(next);else void pump();}
async function fetchBatch(){
 const response=await fetch('/frame/api/batch',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({count:20,exclude:timeline.exclusions()}),signal:requestController.signal,cache:'no-store'});
 if(!response.ok)throw Error('batch unavailable');
 const data=JSON.parse(await (await boundedBody(response,65536)).text());if(!Array.isArray(data.photos))throw Error('manifest');
 return data.photos.slice(0,20).filter(p=>/^[a-f0-9]{32}$/.test(p.id)&&p.url===`/v1/frame/photos/${p.id}`&&Number.isInteger(p.width)&&Number.isInteger(p.height)&&p.width>0&&p.height>0&&p.width*p.height<=40000000&&p.orientation>=1&&p.orientation<=8);
}
async function pump(){
 if(!initialized||fetching||stopped||document.hidden||timeline.ahead>=AHEAD)return;
 if(!pending.length&&timeline.ahead>5)return;
 if(performance.now()<retryAt){clearTimeout(pumpTimer);pumpTimer=setTimeout(()=>void pump(),retryAt-performance.now());return;}
 fetching=true;requestController=new AbortController();const timeout=setTimeout(()=>requestController.abort(),30000);
 try{
  if(!pending.length)pending.push(...await fetchBatch());
  const item=pending.shift();if(!item)throw Error('no candidates');
  let record=await getPhoto(item.id);
  if(!record){
   const response=await fetch(`/frame/photos/${item.id}`,{signal:requestController.signal,cache:'no-store'});
   if(!response.ok)throw Error('photo unavailable');
   record=await normalize(await boundedBody(response,MAX_RAW),item);await putPhoto(record);
  }
  await thumbnail(record);const empty=!timeline.current;
  if(timeline.append({id:item.id})){renderRail();saveState();await cleanup();if(empty)await show(timeline.current);}
 }catch{retryAt=performance.now()+(pending.length?1000:30000);/* Keep the photograph; retry without a request burst. */}
 finally{
  clearTimeout(timeout);fetching=false;clearTimeout(pumpTimer);
  if(!stopped&&!document.hidden)pumpTimer=setTimeout(()=>void pump(),pending.length?100:30000);
 }
}
async function restore(){
 await openStore();let state;
 try{state=await request(database.transaction('state').objectStore('state').get('timeline'));}catch{}
 const restored=[];let position=0;
 for(let i=0;i<Math.min(60,state?.ids?.length??0);i++){
  const id=state.ids[i];if(!/^[a-f0-9]{32}$/.test(id))continue;
  const record=await getPhoto(id);if(!record?.photo||record.photo.size>MAX_DISPLAY||!record.thumb||record.thumb.size>MAX_THUMB||!record.background)continue;
  try{await thumbnail(record);restored.push({id});if(i<state.index)position++;}catch{}
 }
 timeline.restore(restored,position);renderRail();if(timeline.current)await show(timeline.current);if(restored.length||!state?.ids?.length)await cleanup();initialized=true;void pump();
}
function togglePlay(){playback.playing=!playback.playing;resetClock();$('play').setAttribute('aria-label',playback.playing?'暫停播放':'開始播放');$('play-path').setAttribute('d',playback.playing?'M9 6v12M15 6v12':'m9 5 10 7-10 7Z');}
async function fullscreen(){try{if(document.fullscreenElement)await document.exitFullscreen();else await document.documentElement.requestFullscreen();}catch{}setControls(false);}
$('previous').addEventListener('click',()=>{move(-1);setControls(true);});$('next').addEventListener('click',()=>{move(1);setControls(true);});$('play').addEventListener('click',()=>{togglePlay();setControls(true);});$('fullscreen').addEventListener('click',fullscreen);
for(const event of ['dragstart','dragover','drop','contextmenu','selectstart','dblclick'])document.addEventListener(event,e=>e.preventDefault());
document.addEventListener('wheel',e=>e.preventDefault(),{passive:false});
document.addEventListener('pointerdown',e=>{pointerDown={x:e.clientX,y:e.clientY,dragged:false};});
document.addEventListener('pointermove',e=>{if(pointerDown&&Math.hypot(e.clientX-pointerDown.x,e.clientY-pointerDown.y)>6)pointerDown.dragged=true;wakePointer(e);});
document.addEventListener('pointerup',()=>{if(pointerDown?.dragged)blockClickUntil=performance.now()+200;pointerDown=null;});
document.addEventListener('pointercancel',()=>{pointerDown=null;});
document.addEventListener('click',e=>{if(performance.now()<blockClickUntil){e.preventDefault();e.stopImmediatePropagation();}},true);
document.addEventListener('keydown',e=>{
 if(e.metaKey||e.ctrlKey||e.altKey)return;
 if(e.key===' '&&e.target.closest?.('button'))return;
 switch(e.key){
  case 'ArrowLeft':e.preventDefault();move(-1);if(playback.controls)setControls(true);break;
  case 'ArrowRight':e.preventDefault();move(1);if(playback.controls)setControls(true);break;
  case 'ArrowDown':e.preventDefault();setControls(true);break;
  case 'ArrowUp':case 'Escape':setControls(false);break;
  case ' ':e.preventDefault();if(!e.repeat)togglePlay();break;
  case 'f':case 'F':e.preventDefault();if(!e.repeat)void fullscreen();break;
  case 'Tab':setControls(true);break;
 }
});
document.addEventListener('focusin',()=>{if(controls.contains(document.activeElement))setControls(true);});
document.addEventListener('visibilitychange',()=>{playback.visible=!document.hidden;resetClock();if(document.hidden){requestController?.abort();clearTimeout(pumpTimer);}else void pump();});
new ResizeObserver(()=>{if(displayed)paintScene(displayed.canvas,displayed.image,displayed.background);renderRail();}).observe(document.documentElement);
setInterval(()=>{if(playback.due(performance.now())){resetClock();const entry=timeline.atDelta(1);if(entry)void show(entry);else void pump();}},250);
window.addEventListener('pagehide',event=>{requestController?.abort();clearTimeout(pumpTimer);saveState();if(!event.persisted)stopped=true;});
window.addEventListener('pageshow',()=>{stopped=false;resetClock();void pump();});
if(isSecureContext&&'serviceWorker' in navigator)navigator.serviceWorker.register('/frame/sw.js',{scope:'/frame/'}).catch(()=>{});
resetClock();cursorTimer=setTimeout(()=>document.body.classList.add('quiet'),2000);void restore();
