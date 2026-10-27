package com.gathin.tvhome;
/** Clock-safe, foreground-only refresh eligibility; it never schedules a wakeup. */
final class NasPhotoPolicy {
 static boolean due(long now,long last,long interval){return last<=0||now<last||now-last>=interval;}
 static boolean refresh(boolean enabled,boolean configured,boolean cached,boolean sameSource,long now,long success,long failed){return enabled&&configured&&(!cached||!sameSource||due(now,success,NasPhotoCache.REFRESH))&&due(now,failed,NasPhotoCache.RETRY);}
 private NasPhotoPolicy(){}
}
