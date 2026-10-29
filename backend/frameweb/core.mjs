export const MAX_ITEMS=60, MAX_HISTORY=40, AHEAD=20, INTERVAL=10000;
export class Timeline {
 constructor(){this.items=[];this.index=0;this.sequence=0;}
 get current(){return this.items[this.index]??null;}
 get ahead(){return Math.max(0,this.items.length-this.index-1);}
 append(photo){
  if(!photo?.id||this.items.at(-1)?.id===photo.id)return false;
  if(this.items.length>=MAX_ITEMS){if(this.index===0)return false;this.items.shift();this.index--;}
  this.items.push({...photo,key:++this.sequence});return true;
 }
 select(entry){const index=this.items.indexOf(entry);if(index<0)return false;this.index=index;while(this.index>MAX_HISTORY){this.items.shift();this.index--;}return true;}
 atDelta(delta){return this.items[this.index+delta]??null;}
 exclusions(){return [...new Set([this.current?.id,...[...this.items].reverse().map(p=>p.id)].filter(Boolean))];}
 restore(records,index){this.items=records.slice(0,MAX_ITEMS).map(r=>({...r,key:++this.sequence}));this.index=Math.max(0,Math.min(index,this.items.length-1));}
}
// Visibility and deliberate interaction control the clock. A missing photo never
// queues navigation: the caller simply waits for the next ordinary interval.
export class Playback {
 constructor(){this.playing=true;this.visible=true;this.controls=false;this.deadline=0;}
 reset(now){this.deadline=now+INTERVAL;}
 due(now){return this.playing&&this.visible&&!this.controls&&now>=this.deadline;}
}
export function fit(width,height,maxWidth,maxHeight,cover=false){
 const scale=(cover?Math.max:Math.min)(maxWidth/width,maxHeight/height);
 return {width:width*scale,height:height*scale,x:(maxWidth-width*scale)/2,y:(maxHeight-height*scale)/2};
}
export function soften(pixels,width,height,radius=6){
 let source=new Uint8ClampedArray(pixels),target=new Uint8ClampedArray(source.length);
 for(let pass=0;pass<3;pass++)for(const horizontal of [true,false]){
  const lines=horizontal?height:width,length=horizontal?width:height;
  const offset=(line,at)=>((horizontal?line*width+at:at*width+line)*4);
  for(let line=0;line<lines;line++){
   const sums=[0,0,0];for(let k=-radius;k<=radius;k++){const at=offset(line,Math.max(0,Math.min(length-1,k)));for(let c=0;c<3;c++)sums[c]+=source[at+c];}
   for(let at=0;at<length;at++){
    const out=offset(line,at);for(let c=0;c<3;c++)target[out+c]=Math.round(sums[c]/(radius*2+1));target[out+3]=255;
    const remove=offset(line,Math.max(0,at-radius)),add=offset(line,Math.min(length-1,at+radius+1));for(let c=0;c<3;c++)sums[c]+=source[add+c]-source[remove+c];
   }
  }
  [source,target]=[target,source];
 }
 for(let i=0;i<source.length;i+=4){const gray=.2126*source[i]+.7152*source[i+1]+.0722*source[i+2];for(let c=0;c<3;c++)source[i+c]=(gray+(source[i+c]-gray)*.65)*.72;}
 return source;
}
