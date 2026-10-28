package com.gathin.tvhome;

/** Immutable photo protocol checks, shared by download and cache reuse. */
final class PublishedPhotoProtocol {
 static final long MAX_BYTES=3L*1024*1024;
 static boolean validUntil(long until,long now){return until>now;}
 static boolean batch(String id){return id!=null&&id.matches("[0-9a-f]{32}");}
 static boolean asset(String id,String url,String sha,long bytes,int width,int height){
  return batch(id)&&sha!=null&&sha.matches("[0-9a-f]{64}")&&id.equals(sha.substring(0,32))&&url.equals("/v2/wallpapers/assets/"+sha+".jpg")&&bytes>0&&bytes<=MAX_BYTES&&width==1920&&height==1080;
 }
 static String hex(byte[] bytes){StringBuilder out=new StringBuilder();for(byte b:bytes)out.append(String.format(java.util.Locale.ROOT,"%02x",b&255));return out.toString();}
 static boolean matches(String expected,long expectedBytes,long actualBytes,byte[] digest){return expectedBytes==actualBytes&&expected.equals(hex(digest));}
 private PublishedPhotoProtocol(){}
}
