package com.gathin.photoframe;

import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

/** The launcher supplies opt-in LAN credentials; this library owns no account UI. */
public final class PhotoSource {
    public final boolean enabled;
    public final String base, token, key;
    public PhotoSource(boolean requested, String base, String token) {
        this.base = base.replaceAll("/+$", ""); this.token = token;
        enabled = requested && privateURL(this.base) && token.length() >= 24;
        try {
            byte[] bytes = MessageDigest.getInstance("SHA-256").digest((enabled ? this.base+"\n"+token : "local").getBytes(StandardCharsets.UTF_8));
            StringBuilder result = new StringBuilder(); for (byte b : bytes) result.append(String.format(java.util.Locale.ROOT,"%02x",b&255)); key=result.toString();
        } catch (Exception e) { throw new IllegalStateException(e); }
    }
    private static boolean privateURL(String value) {
        try {
            URI u = new URI(value);
            if (!("http".equals(u.getScheme()) || "https".equals(u.getScheme())) || u.getUserInfo()!=null || u.getQuery()!=null || u.getFragment()!=null || !u.getPath().isEmpty()) return false;
            String[] p=u.getHost().split("\\."); if(p.length!=4)return false;
            int[] a=new int[4];for(int i=0;i<4;i++){a[i]=Integer.parseInt(p[i]);if(a[i]<0||a[i]>255)return false;}
            return a[0]==10 || a[0]==192&&a[1]==168 || a[0]==172&&a[1]>=16&&a[1]<=31;
        } catch(Exception e){return false;}
    }
}
