package com.gathin.tvhome;

import android.content.Context;
import android.content.SharedPreferences;
import org.json.JSONObject;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.net.URI;
import java.security.MessageDigest;

/** Opt-in personal fork feature. Runtime endpoint/token never belong in source control. */
final class NasPhotoSettings {
 static final String PREFS="nas_photo_wallpaper";
 final boolean enabled;final String base,token;final int dim;
 private NasPhotoSettings(boolean enabled,String base,String token,int dim){this.enabled=enabled;this.base=base;this.token=token;this.dim=dim;}
 static SharedPreferences prefs(Context c){return c.getSharedPreferences(PREFS,Context.MODE_PRIVATE);}
 static NasPhotoSettings load(Context c){JSONObject json=new JSONObject();try{File dir=Config.dir(c);File file=dir==null?null:new File(dir,"config.json");if(file!=null&&file.length()<128*1024)json=new JSONObject(new String(Files.readAllBytes(file.toPath()),StandardCharsets.UTF_8));}catch(Exception ignored){}SharedPreferences p=prefs(c);return new NasPhotoSettings(p.contains("enabled")?p.getBoolean("enabled",false):json.optBoolean("nas_photo_enabled",false),p.getString("url",json.optString("nas_photo_url","")).replaceAll("/+$",""),p.getString("token",json.optString("nas_photo_token","")),Math.max(0,Math.min(90,json.optInt("nas_photo_dim",40))));}
 static boolean validURL(String base){try{URI u=new URI(base);if(!("http".equals(u.getScheme())||"https".equals(u.getScheme()))||u.getUserInfo()!=null||u.getQuery()!=null||u.getFragment()!=null||u.getPath()!=null&&!u.getPath().isEmpty())return false;String host=u.getHost();if(host==null)return false;String[] parts=host.split("\\.");if(parts.length!=4)return false;int[] a=new int[4];for(int i=0;i<4;i++){a[i]=Integer.parseInt(parts[i]);if(a[i]<0||a[i]>255)return false;}return a[0]==10||a[0]==192&&a[1]==168||a[0]==172&&a[1]>=16&&a[1]<=31;}catch(Exception e){return false;}}
 boolean configured(){return validURL(base)&&token.length()>=24;}
 String sourceKey(){try{byte[] digest=MessageDigest.getInstance("SHA-256").digest((base+"\n"+token).getBytes(StandardCharsets.UTF_8));StringBuilder b=new StringBuilder();for(byte v:digest)b.append(String.format(java.util.Locale.ROOT,"%02x",v&255));return b.toString();}catch(Exception e){throw new IllegalStateException(e);}}
 static void changed(Context c){SharedPreferences p=c.getSharedPreferences(Config.SETTINGS_NAME,Context.MODE_PRIVATE);p.edit().putLong(Config.SETTINGS_REVISION_KEY,p.getLong(Config.SETTINGS_REVISION_KEY,0)+1).commit();c.getContentResolver().notifyChange(android.net.Uri.parse("content://com.gathin.tvhome.theme/nas-settings"),null);}
}
