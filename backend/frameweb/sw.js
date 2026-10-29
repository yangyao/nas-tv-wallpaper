// Shell only. Photo blobs live in a separately bounded IndexedDB, never here.
const CACHE='photo-frame-shell-1.6.1';
const SHELL=['/frame/','/frame/app.js?v=1.6.1','/frame/core.mjs?v=1.6.1','/frame/style.css?v=1.6.1','/frame/icon.svg','/frame/manifest.webmanifest'];
self.addEventListener('install',event=>event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(SHELL))));
// No skipWaiting or forced page reload: an open slideshow keeps its current build.
self.addEventListener('activate',event=>event.waitUntil(caches.keys().then(keys=>Promise.all(keys.filter(k=>k.startsWith('photo-frame-shell-')&&k!==CACHE).map(k=>caches.delete(k))))));
self.addEventListener('fetch',event=>{
 const url=new URL(event.request.url);if(url.origin!==location.origin||event.request.method!=='GET')return;
 const key=url.pathname==='/frame'?'/frame/':url.pathname+url.search;
 if(!SHELL.includes(key))return;
 event.respondWith(caches.open(CACHE).then(async cache=>(await cache.match(key))??fetch(event.request)));
});
