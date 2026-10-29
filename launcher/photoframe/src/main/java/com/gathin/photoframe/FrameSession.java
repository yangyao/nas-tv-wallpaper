package com.gathin.photoframe;

import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Matrix;
import android.os.Handler;
import android.os.Looper;
import org.json.JSONArray;
import org.json.JSONObject;
import java.io.*;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** One serial download lane and one display decoder. No UI waits on the network. */
final class FrameSession {
    static final long ORIGINAL_LIMIT=40L*1024*1024, PHOTO_LIMIT=3L*1024*1024;
    static final class Photo {
        final String id; final File file; final Bitmap thumb;
        Photo(String id,File file,Bitmap thumb){this.id=id;this.file=file;this.thumb=thumb;}
        @Override public boolean equals(Object other){return other instanceof Photo&&id.equals(((Photo)other).id);}
        @Override public int hashCode(){return id.hashCode();}
    }
    interface Listener { void timeline(List<Photo> photos,int position); void image(Bitmap bitmap,Bitmap backdrop); }
    private static final class Candidate {
        final String id,path; final int orientation;
        Candidate(String id,String path,int orientation){this.id=id;this.path=path;this.orientation=orientation;}
    }
    private final Handler main=new Handler(Looper.getMainLooper());
    // Serialize disk transactions across activity instances, including a quick exit/re-entry.
    private static final ExecutorService CACHE_LANE=Executors.newSingleThreadExecutor();
    private final ExecutorService network=CACHE_LANE, decoder=Executors.newSingleThreadExecutor();
    private final PhotoTimeline<Photo> timeline=new PhotoTimeline<>();
    private final ArrayDeque<Candidate> pending=new ArrayDeque<>();
    private final PhotoSource source;
    private final Listener listener;
    private final File root;
    private final SharedPreferences prefs;
    private volatile HttpURLConnection connection;
    private volatile boolean active=true;
    private boolean busy,moving;
    private int ticket;
    private final Runnable retry=this::pump;

    FrameSession(Context context,PhotoSource source,List<File> seeds,Listener listener){
        this.source=source;this.listener=listener;
        root=new File(new File(context.getCacheDir(),"photo-frame"),source.key);
        prefs=context.getSharedPreferences("photo-frame-state",Context.MODE_PRIVATE);
        String saved=prefs.getString("source","").equals(source.key)?prefs.getString("timeline","{}" ):"{}";
        network.execute(()->{
            if(!active)return;
            List<Photo> restored=new ArrayList<>();int position=0;
            try{
                root.mkdirs();File[] siblings=root.getParentFile().listFiles();
                if(siblings!=null)for(File f:siblings)if(!f.equals(root))delete(f);
                JSONObject state=new JSONObject(saved);JSONArray items=state.optJSONArray("items");
                int old=state.optInt("position",0);
                if(items!=null)for(int i=0;i<Math.min(60,items.length());i++){
                    String id=items.optString(i);File f=new File(root,id+".jpg");
                    if(id.matches("[0-9a-f]{32}")&&f.isFile()&&f.length()>100&&f.length()<=PHOTO_LIMIT){
                        if(i<old)position++;restored.add(photo(id));
                    }
                }
            }catch(Exception ignored){restored.clear();position=0;}
            if(restored.isEmpty())for(File seed:seeds){
                try{
                    String id=seedID(seed);File target=new File(root,id+".jpg");
                    if(!target.isFile()){int orientation=new androidx.exifinterface.media.ExifInterface(seed.getPath()).getAttributeInt(androidx.exifinterface.media.ExifInterface.TAG_ORIENTATION,1);normalize(seed,target,orientation);}
                    restored.add(photo(id));break;
                }catch(Exception ignored){}
            }
            final int start=position;
            main.post(()->{if(!active)return;for(Photo p:restored)timeline.append(p);timeline.select(Math.min(start,Math.max(0,timeline.size()-1)));notifyTimeline();show(timeline.position());prune();pump();});
        });
    }
    private static String seedID(File file)throws Exception{
        byte[] hash=MessageDigest.getInstance("SHA-256").digest(("seed:"+file.getPath()+":"+file.length()+":"+file.lastModified()).getBytes(StandardCharsets.UTF_8));
        StringBuilder s=new StringBuilder();for(int i=0;i<16;i++)s.append(String.format(java.util.Locale.ROOT,"%02x",hash[i]&255));return s.toString();
    }
    void move(int delta){if(!active||moving)return;int next=timeline.position()+delta;if(timeline.at(next)!=null)show(next);else pump();}
    void select(int position){if(active&&!moving&&timeline.at(position)!=null)show(position);}
    private void show(int position){
        Photo p=timeline.at(position);if(p==null||moving)return;
        moving=true;int generation=++ticket;
        decoder.execute(()->{
            Bitmap bitmap=BitmapFactory.decodeFile(p.file.getPath());
            Bitmap backdrop=bitmap==null?null:PhotoBackdrop.create(bitmap);
            main.post(()->{
                if(!active||generation!=ticket){if(bitmap!=null)bitmap.recycle();if(backdrop!=null)backdrop.recycle();return;}
                moving=false;
                if(bitmap!=null){int resolved=-1;List<Photo> now=timeline.snapshot();for(int i=0;i<now.size();i++)if(now.get(i)==p){resolved=i;break;}
                    if(resolved>=0){timeline.select(resolved);listener.image(bitmap,backdrop);notifyTimeline();save();}else {bitmap.recycle();if(backdrop!=null)backdrop.recycle();}}
                pump();
            });
        });
    }
    private void notifyTimeline(){listener.timeline(timeline.snapshot(),timeline.position());}
    private void pump(){
        if(!active||busy||!source.enabled||timeline.ahead()>=20)return;
        if(pending.isEmpty()){
            if(timeline.ahead()>5)return;
            busy=true;JSONArray exclude=new JSONArray();Set<String> seen=new HashSet<>();
            Photo current=timeline.current();if(current!=null){exclude.put(current.id);seen.add(current.id);}
            List<Photo> snapshot=timeline.snapshot();for(int i=snapshot.size()-1;i>=0;i--)if(seen.add(snapshot.get(i).id))exclude.put(snapshot.get(i).id);
            network.execute(()->{
                List<Candidate> candidates=new ArrayList<>();
                try{
                    HttpURLConnection c=open("/v1/frame/batch","POST");
                    try{
                        c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");
                        byte[] body=new JSONObject().put("count",20).put("exclude",exclude).toString().getBytes(StandardCharsets.UTF_8);
                        try(OutputStream out=c.getOutputStream()){out.write(body);}
                        if(c.getResponseCode()!=200)throw new IOException("unavailable");
                        JSONArray a=new JSONObject(new String(read(c.getInputStream(),64*1024),StandardCharsets.UTF_8)).getJSONArray("photos");
                        for(int i=0;i<Math.min(20,a.length());i++){
                            JSONObject item=a.getJSONObject(i);String id=item.getString("id"),path=item.getString("url");int o=item.optInt("orientation",1);
                            if(!id.matches("[0-9a-f]{32}")||!path.equals("/v1/frame/photos/"+id)||o<1||o>8)throw new IOException("invalid manifest");
                            candidates.add(new Candidate(id,path,o));
                        }
                    }finally{c.disconnect();connection=null;}
                }catch(Exception ignored){candidates.clear();}
                main.post(()->{if(!active)return;busy=false;pending.addAll(candidates);if(pending.isEmpty())later();else pump();});
            });return;
        }
        Candidate next=pending.removeFirst();busy=true;
        network.execute(()->{
            Photo ready=null;File part=new File(root,next.id+".part");
            try{
                File target=new File(root,next.id+".jpg");
                if(!target.isFile()){
                    HttpURLConnection c=open(next.path,"GET");
                    try{
                        if(c.getResponseCode()!=200||c.getContentLengthLong()>ORIGINAL_LIMIT)throw new IOException("unavailable");
                        long total=0;try(InputStream in=c.getInputStream();FileOutputStream out=new FileOutputStream(part)){
                            byte[] buffer=new byte[32768];int n;while((n=in.read(buffer))!=-1){check();total+=n;if(total>ORIGINAL_LIMIT)throw new IOException("too large");out.write(buffer,0,n);}
                        }
                    }finally{c.disconnect();connection=null;}
                    check();normalize(part,target,next.orientation);
                }
                ready=photo(next.id);
            }catch(Exception ignored){}finally{part.delete();}
            Photo result=ready;
            main.post(()->{
                if(!active)return;busy=false;
                if(result!=null){boolean empty=timeline.size()==0;if(timeline.append(result)){notifyTimeline();save();prune();if(empty)show(0);}}
                if(result==null&&pending.isEmpty())later();else pump();
            });
        });
    }
    private void later(){main.removeCallbacks(retry);main.postDelayed(retry,30000);}
    private HttpURLConnection open(String path,String method)throws IOException{
        check();HttpURLConnection c=(HttpURLConnection)new URL(source.base+path).openConnection();connection=c;
        c.setInstanceFollowRedirects(false);c.setConnectTimeout(4000);c.setReadTimeout(20000);c.setRequestMethod(method);c.setRequestProperty("Authorization","Bearer "+source.token);return c;
    }
    private void check()throws InterruptedIOException{if(!active||Thread.currentThread().isInterrupted())throw new InterruptedIOException("closed");}
    private byte[] read(InputStream input,int limit)throws IOException{
        try(InputStream in=input;ByteArrayOutputStream out=new ByteArrayOutputStream()){
            byte[] b=new byte[4096];int n;while((n=in.read(b))!=-1){check();if(out.size()+n>limit)throw new IOException("too large");out.write(b,0,n);}return out.toByteArray();
        }
    }
    private Photo photo(String id)throws IOException{
        File file=new File(root,id+".jpg"),thumb=new File(root,id+".thumb");
        Bitmap small=thumb.isFile()?BitmapFactory.decodeFile(thumb.getPath()):null;
        if(small==null){BitmapFactory.Options o=new BitmapFactory.Options();o.inJustDecodeBounds=true;BitmapFactory.decodeFile(file.getPath(),o);o.inSampleSize=1;while(o.outWidth/o.inSampleSize>192||o.outHeight/o.inSampleSize>144)o.inSampleSize*=2;o.inJustDecodeBounds=false;small=BitmapFactory.decodeFile(file.getPath(),o);if(small==null)throw new IOException("invalid image");try(FileOutputStream out=new FileOutputStream(thumb)){small.compress(Bitmap.CompressFormat.JPEG,78,out);}}
        return new Photo(id,file,small);
    }
    private static void normalize(File input,File target,int orientation)throws IOException{
        BitmapFactory.Options o=new BitmapFactory.Options();o.inJustDecodeBounds=true;BitmapFactory.decodeFile(input.getPath(),o);
        if(o.outWidth<1||o.outHeight<1||(long)o.outWidth*o.outHeight>100_000_000)throw new IOException("invalid dimensions");
        o.inSampleSize=1;while((long)(o.outWidth/o.inSampleSize)*(o.outHeight/o.inSampleSize)>4_147_200)o.inSampleSize*=2;
        o.inJustDecodeBounds=false;Bitmap b=BitmapFactory.decodeFile(input.getPath(),o);if(b==null)throw new IOException("decode failed");
        Matrix m=new Matrix();switch(orientation){case 2:m.setScale(-1,1);break;case 3:m.setRotate(180);break;case 4:m.setScale(1,-1);break;case 5:m.setRotate(90);m.postScale(-1,1);break;case 6:m.setRotate(90);break;case 7:m.setRotate(-90);m.postScale(-1,1);break;case 8:m.setRotate(-90);break;default:break;}
        if(!m.isIdentity()){Bitmap next=Bitmap.createBitmap(b,0,0,b.getWidth(),b.getHeight(),m,true);if(next!=b)b.recycle();b=next;}
        float factor=Math.min(1f,Math.min(1920f/b.getWidth(),1080f/b.getHeight()));
        if(factor<1){Bitmap next=Bitmap.createScaledBitmap(b,Math.max(1,Math.round(b.getWidth()*factor)),Math.max(1,Math.round(b.getHeight()*factor)),true);if(next!=b)b.recycle();b=next;}
        File tmp=new File(target.getPath()+".tmp");
        try{
            for(int quality=86;quality>=56;quality-=15){try(FileOutputStream out=new FileOutputStream(tmp)){if(!b.compress(Bitmap.CompressFormat.JPEG,quality,out))throw new IOException("encode failed");}if(tmp.length()<=PHOTO_LIMIT)break;}
            if(tmp.length()>PHOTO_LIMIT||!tmp.renameTo(target))throw new IOException("cache limit");
        }finally{b.recycle();tmp.delete();}
    }
    private void save(){JSONArray a=new JSONArray();for(Photo p:timeline.snapshot())a.put(p.id);try{prefs.edit().putString("source",source.key).putString("timeline",new JSONObject().put("items",a).put("position",timeline.position()).toString()).apply();}catch(Exception ignored){}}
    private void prune(){
        Set<String> keep=new HashSet<>();for(Photo p:timeline.snapshot()){keep.add(p.id+".jpg");keep.add(p.id+".thumb");}
        network.execute(()->{if(!active)return;File[] files=root.listFiles();if(files!=null)for(File f:files)if(!keep.contains(f.getName()))f.delete();});
    }
    void close(){active=false;ticket++;main.removeCallbacks(retry);HttpURLConnection c=connection;if(c!=null)c.disconnect();decoder.shutdownNow();}
    private static void delete(File f){File[] children=f.listFiles();if(children!=null)for(File child:children)delete(child);f.delete();}
}
