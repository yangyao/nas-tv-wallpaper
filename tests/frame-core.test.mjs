import {test} from 'node:test';
import assert from 'node:assert/strict';
import {Timeline,Playback,fit,soften} from '../backend/frameweb/core.mjs';

test('rapid navigation into missing content never queues a future move',()=>{
 const t=new Timeline();t.append({id:'one'});for(let i=0;i<100;i++)assert.equal(t.atDelta(1),null);
 t.append({id:'two'});assert.equal(t.current.id,'one');t.select(t.atDelta(1));assert.equal(t.current.id,'two');t.select(t.atDelta(-1));assert.equal(t.current.id,'one');
});
test('prefetch and bounded eviction preserve the displayed occurrence and order',()=>{
 const t=new Timeline();for(let i=0;i<21;i++)t.append({id:`photo-${i}`});
 for(let i=1;i<500;i++){const next=t.atDelta(1);assert.equal(next.id,`photo-${i}`);t.select(next);t.append({id:`photo-${i+20}`});assert.equal(t.current,next);assert.ok(t.items.length<=60);assert.ok(t.index<=40);assert.ok(t.ahead<=20);}
 assert.equal(t.atDelta(-1).id,'photo-498');const snapshot=t.current;t.append({id:'more'});assert.equal(t.current,snapshot);
});
test('duplicate photo IDs remain distinct visits and exclusions keep current first',()=>{
 const t=new Timeline();t.append({id:'a'});t.append({id:'b'});t.append({id:'a'});assert.notEqual(t.items[0].key,t.items[2].key);t.select(t.items[2]);assert.deepEqual(t.exclusions(),['a','b']);assert.equal(t.append({id:'a'}),false);
});
test('restoring missing files clamps the session position',()=>{const t=new Timeline();t.restore([{id:'x'}],30);assert.equal(t.current.id,'x');t.restore([],3);assert.equal(t.current,null);});
test('resuming playback never catches up with a burst of skipped slides',()=>{
 const p=new Playback();p.reset(0);assert.equal(p.due(9999),false);assert.equal(p.due(10000),true);p.visible=false;assert.equal(p.due(90000),false);p.visible=true;p.reset(90000);assert.equal(p.due(90000),false);assert.equal(p.due(100000),true);p.controls=true;assert.equal(p.due(200000),false);p.controls=false;p.reset(200000);assert.equal(p.due(200001),false);
});
test('foreground fits whole photo; backdrop fills every edge',()=>{assert.deepEqual(fit(600,1200,1920,1080),{width:540,height:1080,x:690,y:0});const bg=fit(600,1200,1920,1080,true);assert.equal(bg.width,1920);assert.ok(bg.y<0);assert.equal(bg.x,0);});
test('softening a flat image introduces no dark border or transparent pixels',()=>{
 const pixels=new Uint8ClampedArray(8*6*4);for(let i=0;i<pixels.length;i+=4)pixels.set([200,160,120,255],i);
 const result=soften(pixels,8,6);for(let i=0;i<result.length;i+=4){assert.deepEqual([...result.slice(i,i+4)],[...result.slice(0,4)]);assert.equal(result[i+3],255);}assert.ok(result[0]<200);
});
