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
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicBoolean;

/** Downloads only while Home is visible. Commits whole batches with an atomic pointer. */
final class NasPhotoCache {
 static final long REFRESH=6*60*60*1000L,RETRY=30*60*1000L,LIMIT=100L*1024*1024;
 private static final AtomicBoolean BUSY=new AtomicBoolean();
 private final Context context;private final Runnable updated;private final Handler main=new Handler(Looper.getMainLooper());private final ExecutorService worker=Executors.newSingleThreadExecutor();private volatile boolean active,closed;private volatile HttpURLConnection connection;
 NasPhotoCache(Context c,Runnable updated){context=c.getApplicationContext();this.updated=updated;}
 static File root(Context c){return new File(c.getFilesDir(),"nas-photo-cache");}
 static JSONObject json(File f){try{if(f.length()>256*1024)return new JSONObject();return new JSONObject(new String(new AtomicFile(f).readFully(),StandardCharsets.UTF_8));}catch(Exception e){return new JSONObject();}}
 static void write(File f,JSONObject data)throws Exception{File parent=f.getParentFile();if(parent!=null&&!parent.isDirectory()&&!parent.mkdirs())throw new IOException("cache directory unavailable");AtomicFile af=new AtomicFile(f);FileOutputStream out=null;try{out=af.startWrite();out.write(data.toString().getBytes(StandardCharsets.UTF_8));af.finishWrite(out);}catch(Exception e){if(out!=null)af.failWrite(out);throw e;}}
 static List<File> current(Context c){NasPhotoSettings settings=NasPhotoSettings.load(c);if(!settings.enabled)return Collections.emptyList();JSONObject state=json(new File(root(c),"state.json"));if(!settings.sourceKey().equals(state.optString("source")))return Collections.emptyList();List<File> files=batchFiles(c,state.optString("current"));if(files.isEmpty())files=batchFiles(c,state.optString("previous"));return files;}
 static List<File> batchFiles(Context c,String name){if(!name.matches("batch-[a-zA-Z0-9-]+"))return Collections.emptyList();return WallpaperRotation.images(new File(root(c),name));}
 void resume(){active=true;NasPhotoSettings settings=NasPhotoSettings.load(context);if(closed||!settings.enabled||!settings.configured())return;long now=System.currentTimeMillis();JSONObject state=json(new File(root(context),"state.json"));boolean source=settings.sourceKey().equals(state.optString("source"));if(!NasPhotoPolicy.refresh(settings.enabled,settings.configured(),!current(context).isEmpty(),source,now,state.optLong("success",0),NasPhotoSettings.prefs(context).getLong("failed",0)))return;if(!BUSY.compareAndSet(false,true))return;worker.execute(()->{try{sync(settings);}catch(InterruptedIOException cancelled){/* Leave partial staging files for the next Home. */}catch(Exception failure){if(active&&!closed)NasPhotoSettings.prefs(context).edit().putLong("failed",System.currentTimeMillis()).putString("status","NAS 暂不可用，继续使用本地缓存").apply();}finally{connection=null;BUSY.set(false);}});}
 void pause(){active=false;HttpURLConnection c=connection;if(c!=null)c.disconnect();}
 void close(){closed=true;pause();worker.shutdownNow();}
 private void check()throws InterruptedIOException{if(!active||closed||Thread.currentThread().isInterrupted())throw new InterruptedIOException("Home is not visible");}
 private HttpURLConnection open(NasPhotoSettings settings,String path,String method)throws Exception{check();if(!path.matches("/v1/batch|/v1/photos/[0-9a-f]{32}"))throw new IOException("invalid photo endpoint");HttpURLConnection c=(HttpURLConnection)new URL(settings.base+path).openConnection();connection=c;c.setInstanceFollowRedirects(false);c.setConnectTimeout(4000);c.setReadTimeout(30000);c.setRequestMethod(method);c.setRequestProperty("Authorization","Bearer "+settings.token);return c;}
 private void sync(NasPhotoSettings settings)throws Exception{
  File root=root(context);if(!root.isDirectory()&&!root.mkdirs())throw new IOException("cache unavailable");JSONObject prior=json(new File(root,"state.json"));JSONObject pending=json(new File(root,"pending.json"));if(!settings.sourceKey().equals(pending.optString("source"))||System.currentTimeMillis()-pending.optLong("created",0)>86400000L){pending=new JSONObject();}
  JSONArray photos=pending.optJSONArray("photos");String directory=pending.optString("directory");if(photos==null||!directory.matches("batch-[a-zA-Z0-9-]+")){
   JSONObject request=new JSONObject().put("count",20);JSONArray exclude=new JSONArray();for(String n:new String[]{prior.optString("current"),prior.optString("previous")})for(File f:batchFiles(context,n)){String id=f.getName().replace(".jpg","");if(id.matches("[0-9a-f]{32}"))exclude.put(id);}request.put("exclude",exclude);
   HttpURLConnection c=open(settings,"/v1/batch","POST");c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");byte[] body=request.toString().getBytes(StandardCharsets.UTF_8);c.setFixedLengthStreamingMode(body.length);try(OutputStream out=c.getOutputStream()){out.write(body);}if(c.getResponseCode()!=200)throw new IOException("batch refused");byte[] data;try(InputStream in=c.getInputStream()){data=readLimited(in,128*1024);}finally{c.disconnect();connection=null;}JSONObject response=new JSONObject(new String(data,StandardCharsets.UTF_8));photos=response.getJSONArray("photos");if(photos.length()==0||photos.length()>30)throw new IOException("no qualified photos yet");directory="batch-"+UUID.randomUUID();pending=new JSONObject().put("source",settings.sourceKey()).put("created",System.currentTimeMillis()).put("directory",directory).put("photos",photos);write(new File(root,"pending.json"),pending);
  }
  cleanup(root,new HashSet<>(Arrays.asList(prior.optString("current"),directory)));File stage=new File(root,directory);stage.mkdirs();int failures=0;boolean bootstrap=current(context).isEmpty();
  for(int i=0;i<photos.length();i++){check();JSONObject p=photos.getJSONObject(i);String id=p.getString("id"),url=p.getString("url");if(!id.matches("[0-9a-f]{32}")||!url.equals("/v1/photos/"+id))continue;File target=new File(stage,id+".jpg");if(validImage(target))continue;if(size(root)>LIMIT-3*1024*1024)break;try{download(settings,url,target);failures=0;
    if(bootstrap && WallpaperRotation.images(stage).size()>=Math.min(3,photos.length()) && active && NasPhotoSettings.load(context).enabled){
      JSONObject first=new JSONObject().put("source",settings.sourceKey()).put("current",directory).put("previous",prior.optString("current")).put("success",0);
      write(new File(root,"state.json"),first);bootstrap=false;
      main.post(()->{if(active&&!closed&&NasPhotoSettings.load(context).enabled)updated.run();});
    }
   }catch(InterruptedIOException e){throw e;}catch(Exception e){if(!active||closed)throw new InterruptedIOException("paused");if(++failures>=3)break;}}
  check();List<File> ready=WallpaperRotation.images(stage);int required=Math.min(current(context).isEmpty()?3:10,photos.length());if(ready.size()<required)throw new IOException("not enough completed photos");if(!NasPhotoSettings.load(context).enabled||!settings.sourceKey().equals(NasPhotoSettings.load(context).sourceKey()))throw new InterruptedIOException("disabled or source changed");
  JSONObject state=new JSONObject().put("source",settings.sourceKey()).put("current",directory).put("previous",prior.optString("current")).put("success",System.currentTimeMillis());write(new File(root,"state.json"),state);new AtomicFile(new File(root,"pending.json")).delete();NasPhotoSettings.prefs(context).edit().remove("failed").putString("status","已缓存 "+ready.size()+" 张照片").apply();cleanup(root,new HashSet<>(Arrays.asList(directory,prior.optString("current"))));main.post(()->{if(active&&!closed&&NasPhotoSettings.load(context).enabled)updated.run();});
 }
 private void download(NasPhotoSettings settings,String path,File target)throws Exception{HttpURLConnection c=open(settings,path,"GET");File tmp=new File(target.getParentFile(),target.getName()+".part");try{if(c.getResponseCode()!=200||c.getContentLengthLong()>3*1024*1024)throw new IOException("image refused");try(InputStream in=c.getInputStream();FileOutputStream out=new FileOutputStream(tmp)){byte[] b=new byte[16384];int n;long size=0;while((n=in.read(b))!=-1){check();size+=n;if(size>3*1024*1024)throw new IOException("image too large");out.write(b,0,n);}out.getFD().sync();if(c.getContentLengthLong()>=0&&size!=c.getContentLengthLong())throw new IOException("incomplete image");}if(!validImage(tmp)||!tmp.renameTo(target))throw new IOException("invalid image");}finally{c.disconnect();connection=null;tmp.delete();}}
 static boolean validImage(File file){if(!file.isFile()||file.length()<100||file.length()>3*1024*1024)return false;BitmapFactory.Options o=new BitmapFactory.Options();o.inJustDecodeBounds=true;BitmapFactory.decodeFile(file.getPath(),o);return o.outWidth>0&&o.outHeight>0&&(long)o.outWidth*o.outHeight<=8_400_000;}
 private byte[] readLimited(InputStream in,int max)throws IOException{ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] buffer=new byte[8192];int n;while((n=in.read(buffer))!=-1){check();if(out.size()+n>max)throw new IOException("response too large");out.write(buffer,0,n);}return out.toByteArray();}
 static long size(File root){long n=0;File[] files=root.listFiles();if(files!=null)for(File f:files)n+=f.isDirectory()?size(f):f.length();return n;}
 static void cleanup(File root,Set<String> keep){File[] dirs=root.listFiles();if(dirs!=null)for(File dir:dirs)if(dir.isDirectory()&&dir.getName().startsWith("batch-")&&!keep.contains(dir.getName()))delete(dir);}
 static void delete(File file){File[] children=file.listFiles();if(children!=null)for(File c:children)delete(c);file.delete();}
 static void refreshNextHome(Context c){JSONObject state=json(new File(root(c),"state.json"));try{state.put("success",0);write(new File(root(c),"state.json"),state);}catch(Exception ignored){}NasPhotoSettings.prefs(c).edit().remove("failed").apply();}
}
