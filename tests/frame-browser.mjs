// Run against the local Go server with synthetic JPEG fixtures. No family photos.
import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
const engines=await import(process.env.FRAME_PLAYWRIGHT_MODULE||'playwright');
const base=process.env.FRAME_TEST_URL||'http://127.0.0.1:18791/frame/';
const browser=await engines[process.env.FRAME_BROWSER||'chromium'].launch({headless:true,...(process.env.FRAME_CHROME?{executablePath:process.env.FRAME_CHROME}:{})});
const context=await browser.newContext({viewport:{width:1440,height:900}}),page=await context.newPage();
const errors=[];page.on('pageerror',error=>{errors.push(error.message);if(process.env.FRAME_TRACE)console.log(error.stack);});
await page.addInitScript(()=>{window.layoutShifts=[];if(!PerformanceObserver.supportedEntryTypes.includes('layout-shift'))return;new PerformanceObserver(list=>{for(const e of list.getEntries())if(!e.hadRecentInput)window.layoutShifts.push(e.value);}).observe({type:'layout-shift',buffered:true});});
let batches=0;
await page.route('**/frame/api/batch',async route=>{if(batches>0){await route.fulfill({status:503,body:'test outage'});return;}const response=await route.fetch();if(response.status()===200){batches++;const body=await response.json();body.photos=body.photos.slice(0,4);await route.fulfill({response,json:body});}else await route.fulfill({response});});
await page.route('**/frame/photos/*',async route=>{await new Promise(resolve=>setTimeout(resolve,600));await route.continue();});
try{
 await page.goto(base,{waitUntil:'domcontentloaded'});
 const box=await page.locator('#frame').boundingBox();
 assert.equal(await page.locator('#controls').getAttribute('inert'),'');
 await page.waitForFunction(()=>document.querySelector('.thumb[aria-current="true"]'),null,{timeout:65000});
 await page.keyboard.press('ArrowDown');await page.waitForTimeout(300);
 const preloadAnchor=await page.locator('.thumb[aria-current="true"]').boundingBox();
 await page.waitForFunction(()=>document.querySelectorAll('#track .thumb').length>=4,null,{timeout:65000});
 assert.ok(Math.abs((await page.locator('.thumb[aria-current="true"]').boundingBox()).x-preloadAnchor.x)<1,'prefetch moved the selected thumbnail');
 await page.keyboard.press('Space'); // deliberate pause, independent of other-window focus
 const selected=()=>page.locator('.thumb[aria-current="true"]').evaluate(e=>[...e.parentElement.children].indexOf(e));
 const initial=await selected();
 await page.mouse.click(700,300);await page.mouse.dblclick(700,300);await page.mouse.click(700,300,{button:'right'});
 await page.mouse.move(700,300);await page.mouse.down();await page.mouse.move(1000,500,{steps:10});await page.mouse.up();
 assert.equal(await selected(),initial,'passive canvas interactions changed selection');
 assert.deepEqual(await page.locator('#frame').boundingBox(),box,'layout changed after image decode');
 assert.equal(await page.evaluate(()=>document.body.scrollHeight),900);
 await page.keyboard.press('ArrowDown');await page.waitForTimeout(300);
 assert.equal(await page.locator('#controls').evaluate(e=>getComputedStyle(e).opacity),'1');
 assert.equal(await page.locator('.thumb img').first().evaluate(e=>getComputedStyle(e).opacity),'1');
 await mkdir(process.env.FRAME_SHOTS||'/tmp/nas-frame-browser-shots',{recursive:true});
 await page.screenshot({path:(process.env.FRAME_SHOTS||'/tmp/nas-frame-browser-shots')+'/frame.png'});
 for(let i=0;i<4;i++){await page.keyboard.press('ArrowRight');await page.waitForTimeout(500);}
 const last=await selected();assert.equal(last,3);
 for(let i=0;i<20;i++)await page.keyboard.press('ArrowRight');
 assert.equal(await selected(),last,'unavailable navigation changed current photo');
 await context.setOffline(true);await page.keyboard.press('ArrowUp');await page.waitForTimeout(500);
 const before=await page.screenshot();await page.waitForTimeout(1500);const after=await page.screenshot();
 assert.equal(Buffer.compare(before,after),0,'offline waiting changed the scene');
 assert.deepEqual(await page.locator('#frame').boundingBox(),box);
 await page.evaluate(()=>{const event=new Event('drop',{bubbles:true,cancelable:true});document.dispatchEvent(event);if(!event.defaultPrevented)throw Error('drop was not prevented');});
 await context.setOffline(false);await page.evaluate(()=>navigator.serviceWorker.ready);
 await page.reload({waitUntil:'domcontentloaded'});await page.waitForFunction(()=>document.querySelectorAll('#track .thumb').length>=4);await page.waitForFunction(()=>[...document.querySelectorAll('.scene')].some(c=>c.style.opacity==='1'&&c.width>300));
 assert.equal(await selected(),last,'restart lost selected photo');

 // Once the HTTPS/localhost shell is installed, an explicit offline restart works too.
 await context.setOffline(true);await page.reload({waitUntil:'domcontentloaded'});
 await page.waitForFunction(()=>document.querySelector('.thumb[aria-current="true"]'));await page.waitForFunction(()=>[...document.querySelectorAll('.scene')].some(c=>c.style.opacity==='1'&&c.width>300));
 assert.equal(await selected(),last,'offline restart lost session');
 assert.equal(await page.evaluate(()=>window.layoutShifts.reduce((a,b)=>a+b,0)),0,'layout shifts');
 assert.deepEqual(errors,[]);
 console.log('PASS: fixed layout, inert canvas, no drag/drop/context UI, opaque thumbnails, stable history, no queued keys, offline hold and offline restart');
}finally{await browser.close();}
