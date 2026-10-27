package com.gathin.tvhome;

/** Layout uses the EXIF-oriented original dimensions, never the padded cache frame. */
final class NasPhotoLayout {
 static boolean valid(int w,int h){return w>0&&h>0&&w<=100000&&h<=100000&&(long)w*h<=40000000;}
 static boolean portrait(int w,int h){return valid(w,h)&&w<=h;}
 /** The original photo's complete foreground rectangle in the NAS's padded frame. */
 static int[] foreground(int w,int h,int frameW,int frameH){
  if(!valid(w,h)||frameW<=0||frameH<=0)return new int[]{0,0,frameW,frameH};
  double scale=Math.min((double)frameW/w,(double)frameH/h);
  int fw=Math.max(1,Math.min(frameW,(int)(w*scale))),fh=Math.max(1,Math.min(frameH,(int)(h*scale)));
  return new int[]{(frameW-fw)/2,(frameH-fh)/2,fw,fh};
 }
}
