package com.gathin.tvhome;

import android.content.Context;
import android.graphics.BitmapFactory;
import android.os.Handler;
import android.os.Looper;
import android.util.AtomicFile;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicBoolean;

/** Versioned completed batches only. Never exposes a partial download or raw source. */
final class NasPhotoCache {
 static final long REFRESH=15*60*1000L,RETRY=30*60*1000L,LIMIT=192L*1024*1024;
 private static final AtomicBoolean BUSY=new AtomicBoolean();
 private final Context context;private final Runnable updated;
 private final Handler main=new Handler(Looper.getMainLooper());
 private final ExecutorService worker=Executors.newSingleThreadExecutor();
 private volatile boolean active,closed;private volatile HttpURLConnection connection;
 private final Runnable poll=()->{if(active&&!closed)resume();};
 NasPhotoCache(Context c,Runnable updated){context=c.getApplicationContext();this.updated=updated;}
 static File root(Context c){return new File(c.getFilesDir(),"nas-photo-cache");}
 static JSONObject json(File f){try{if(f.length()>256*1024)return new JSONObject();return new JSONObject(new String(new AtomicFile(f).readFully(),StandardCharsets.UTF_8));}catch(Exception ignored){return new JSONObject();}}
 static void write(File f,JSONObject data)throws Exception{
  File parent=f.getParentFile();if(parent!=null&&!parent.isDirectory()&&!parent.mkdirs())throw new IOException("cache directory unavailable");
  AtomicFile af=new AtomicFile(f);FileOutputStream out=null;
  try{out=af.startWrite();out.write(data.toString().getBytes(StandardCharsets.UTF_8));af.finishWrite(out);}catch(Exception e){if(out!=null)af.failWrite(out);throw e;}
 }
 static List<File> current(Context c){
  NasPhotoSettings settings=NasPhotoSettings.load(c);if(!settings.enabled)return Collections.emptyList();
  JSONObject state=json(new File(root(c),"state.json"));
  // Legacy batches can contain raw photos; do not silently migrate them into ready-only mode.
  if(state.optInt("protocol")!=2||!settings.sourceKey().equals(state.optString("source")))return Collections.emptyList();
  List<File> files=batchFiles(c,state.optString("current"));
  if(files.size()==state.optInt("count",-1)&&!files.isEmpty()&&PublishedPhotoProtocol.validUntil(state.optLong("valid_until"),System.currentTimeMillis()/1000))return files;
  files=batchFiles(c,state.optString("previous"));
  return files.size()==state.optInt("previous_count",-1)&&PublishedPhotoProtocol.validUntil(state.optLong("previous_valid_until"),System.currentTimeMillis()/1000)?files:Collections.emptyList();
 }
 static List<File> batchFiles(Context c,String name){if(!name.matches("batch-[a-zA-Z0-9-]+"))return Collections.emptyList();return WallpaperRotation.images(new File(root(c),name));}
 static int[] dimensions(File photo){JSONObject shape=json(new File(photo.getPath()+".shape"));return new int[]{shape.optInt("width"),shape.optInt("height")};}
 void resume(){
  active=true;main.removeCallbacks(poll);if(closed)return;main.postDelayed(poll,60000);
  NasPhotoSettings settings=NasPhotoSettings.load(context);if(!settings.enabled||!settings.configured())return;
  long now=System.currentTimeMillis();JSONObject state=json(new File(root(context),"state.json"));
  boolean cached=!current(context).isEmpty(),same=settings.sourceKey().equals(state.optString("source"));
  boolean refresh=NasPhotoPolicy.refresh(true,true,cached,same,now,state.optLong("success",0),NasPhotoSettings.prefs(context).getLong("failed",0));
  if(!refresh||!BUSY.compareAndSet(false,true))return;
  worker.execute(()->{try{sync(settings);}catch(InterruptedIOException cancelled){/* Keep every completed old batch; retry staging on the next Home. */}catch(Exception failure){if(active&&!closed)NasPhotoSettings.prefs(context).edit().putLong("failed",System.currentTimeMillis()).putString("status","暂无新成品，继续使用本地缓存").apply();}finally{connection=null;BUSY.set(false);}});
 }
 void pause(){active=false;main.removeCallbacks(poll);HttpURLConnection c=connection;if(c!=null)c.disconnect();}
 void close(){closed=true;pause();worker.shutdownNow();}
 private void check()throws InterruptedIOException{if(!active||closed||Thread.currentThread().isInterrupted())throw new InterruptedIOException("Home is not visible");}
 private HttpURLConnection open(NasPhotoSettings settings,String path)throws Exception{
  check();if(!path.matches("/v2/wallpapers/latest|/v2/wallpapers/assets/[0-9a-f]{64}\\.jpg"))throw new IOException("invalid published endpoint");
  HttpURLConnection c=(HttpURLConnection)new URL(settings.base+path).openConnection();connection=c;c.setInstanceFollowRedirects(false);c.setConnectTimeout(4000);c.setReadTimeout(30000);c.setRequestMethod("GET");c.setRequestProperty("Authorization","Bearer "+settings.token);return c;
 }
 private boolean localBatch(JSONObject state){return state.optInt("protocol")==2&&PublishedPhotoProtocol.validUntil(state.optLong("valid_until"),System.currentTimeMillis()/1000)&&PublishedPhotoProtocol.batch(state.optString("batch_id"))&&batchFiles(context,state.optString("current")).size()==state.optInt("count",-1)&&state.optInt("count")>0;}
 private void sync(NasPhotoSettings settings)throws Exception{
  File root=root(context);if(!root.isDirectory()&&!root.mkdirs())throw new IOException("cache unavailable");
  JSONObject prior=json(new File(root,"state.json"));boolean local=localBatch(prior)&&settings.sourceKey().equals(prior.optString("source"));
  HttpURLConnection c=open(settings,"/v2/wallpapers/latest");
  if(local)c.setRequestProperty("If-None-Match","\""+prior.getString("batch_id")+"\"");
  JSONObject manifest;
  try{
   int status=c.getResponseCode();
   if(status==304&&local){checked(root,prior);return;}
   if(status!=200)throw new IOException("no completed published batch");
   try(InputStream in=c.getInputStream()){manifest=new JSONObject(new String(readLimited(in,128*1024),StandardCharsets.UTF_8));}
  }finally{c.disconnect();connection=null;}
  String version=manifest.getString("batch_id");JSONArray photos=manifest.getJSONArray("photos");
  if(!PublishedPhotoProtocol.validUntil(manifest.optLong("valid_until"),System.currentTimeMillis()/1000)||!PublishedPhotoProtocol.batch(version)||photos.length()<1||photos.length()>20)throw new IOException("invalid batch manifest");
  Set<String> ids=new HashSet<>();
  for(int i=0;i<photos.length();i++){
   JSONObject a=photos.getJSONObject(i);
   if(!PublishedPhotoProtocol.asset(a.getString("id"),a.getString("url"),a.getString("sha256"),a.getLong("bytes"),a.getInt("width"),a.getInt("height"))||!ids.add(a.getString("id")))throw new IOException("invalid published asset");
  }
  if(local&&version.equals(prior.optString("batch_id"))){checked(root,prior);return;}
  String directory="batch-"+version;File stage=new File(root,directory);if(!stage.isDirectory()&&!stage.mkdirs())throw new IOException("stage unavailable");
  Set<String> keep=new HashSet<>(Arrays.asList(prior.optString("current"),directory));
  if(prior.optInt("protocol")==2)keep.add(prior.optString("previous"));cleanup(root,keep);
  write(new File(root,"pending.json"),new JSONObject().put("source",settings.sourceKey()).put("batch_id",version).put("directory",directory));
  for(int i=0;i<photos.length();i++){
   check();JSONObject a=photos.getJSONObject(i);File target=new File(stage,a.getString("id")+".jpg");
   if(!verified(target,a)){
    if(size(root)+a.getLong("bytes")>LIMIT)throw new IOException("cache limit");
    boolean reused=false;
    if(prior.optInt("protocol")==2&&settings.sourceKey().equals(prior.optString("source"))){
     for(String name:new String[]{prior.optString("current"),prior.optString("previous")}){
      if(!name.matches("batch-[a-zA-Z0-9-]+")||name.equals(directory))continue;
      File old=new File(new File(root,name),target.getName());
      if(verified(old,a)){Files.copy(old.toPath(),target.toPath(),StandardCopyOption.REPLACE_EXISTING);reused=true;break;}
     }
    }
    if(!reused)download(settings,a,target);
   }
   write(new File(target.getPath()+".shape"),new JSONObject().put("width",1920).put("height",1080));
  }
  check();if(!NasPhotoSettings.load(context).enabled||!settings.sourceKey().equals(NasPhotoSettings.load(context).sourceKey()))throw new InterruptedIOException("source changed");
  // Every expected file has been checksum verified. One atomic pointer publishes the whole batch.
  JSONObject next=new JSONObject().put("protocol",2).put("source",settings.sourceKey()).put("batch_id",version).put("current",directory).put("count",photos.length()).put("valid_until",manifest.getLong("valid_until")).put("success",System.currentTimeMillis());
  if(local){next.put("previous",prior.optString("current")).put("previous_count",prior.optInt("count")).put("previous_valid_until",prior.optLong("valid_until"));}
  write(new File(root,"state.json"),next);new AtomicFile(new File(root,"pending.json")).delete();
  cleanup(root,new HashSet<>(Arrays.asList(directory,next.optString("previous"))));
  NasPhotoSettings.prefs(context).edit().remove("failed").putString("status","已缓存 "+photos.length()+" 张成品照片").apply();
  main.post(()->{if(active&&!closed&&NasPhotoSettings.load(context).enabled)updated.run();});
 }
 private void checked(File root,JSONObject state)throws Exception{state.put("success",System.currentTimeMillis());write(new File(root,"state.json"),state);NasPhotoSettings.prefs(context).edit().remove("failed").putString("status","已缓存 "+state.optInt("count")+" 张成品照片").apply();}
 private boolean verified(File file,JSONObject a)throws Exception{
  if(!validImage(file)||file.length()!=a.getLong("bytes"))return false;
  MessageDigest digest=MessageDigest.getInstance("SHA-256");try(InputStream in=new FileInputStream(file)){byte[] buffer=new byte[16384];int n;while((n=in.read(buffer))!=-1){check();digest.update(buffer,0,n);}}
  return PublishedPhotoProtocol.matches(a.getString("sha256"),a.getLong("bytes"),file.length(),digest.digest());
 }
 private void download(NasPhotoSettings settings,JSONObject a,File target)throws Exception{
  HttpURLConnection c=open(settings,a.getString("url"));File tmp=new File(target.getParentFile(),target.getName()+".part");
  try{
   if(c.getResponseCode()!=200||c.getContentLengthLong()!=a.getLong("bytes"))throw new IOException("asset unavailable");
   MessageDigest digest=MessageDigest.getInstance("SHA-256");long total=0;
   try(InputStream in=c.getInputStream();FileOutputStream out=new FileOutputStream(tmp)){
    byte[] buffer=new byte[16384];int n;while((n=in.read(buffer))!=-1){check();total+=n;if(total>a.getLong("bytes"))throw new IOException("asset too large");digest.update(buffer,0,n);out.write(buffer,0,n);}out.getFD().sync();
   }
   if(!PublishedPhotoProtocol.matches(a.getString("sha256"),a.getLong("bytes"),total,digest.digest())||!validImage(tmp)||!tmp.renameTo(target))throw new IOException("asset checksum mismatch");
  }finally{c.disconnect();connection=null;tmp.delete();}
 }
 static boolean validImage(File file){if(!file.isFile()||file.length()<100||file.length()>PublishedPhotoProtocol.MAX_BYTES)return false;BitmapFactory.Options o=new BitmapFactory.Options();o.inJustDecodeBounds=true;BitmapFactory.decodeFile(file.getPath(),o);return o.outWidth==1920&&o.outHeight==1080;}
 private byte[] readLimited(InputStream in,int limit)throws IOException{ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] buffer=new byte[8192];int n;while((n=in.read(buffer))!=-1){check();if(out.size()+n>limit)throw new IOException("response too large");out.write(buffer,0,n);}return out.toByteArray();}
 static long size(File root){long n=0;File[] files=root.listFiles();if(files!=null)for(File f:files)n+=f.isDirectory()?size(f):f.length();return n;}
 static void cleanup(File root,Set<String> keep){File[] dirs=root.listFiles();if(dirs!=null)for(File dir:dirs)if(dir.isDirectory()&&dir.getName().startsWith("batch-")&&!keep.contains(dir.getName()))delete(dir);}
 static void delete(File file){File[] children=file.listFiles();if(children!=null)for(File child:children)delete(child);file.delete();}
 static void refreshNextHome(Context c){JSONObject state=json(new File(root(c),"state.json"));try{state.put("success",0);write(new File(root(c),"state.json"),state);}catch(Exception ignored){}NasPhotoSettings.prefs(c).edit().remove("failed").apply();}
}
