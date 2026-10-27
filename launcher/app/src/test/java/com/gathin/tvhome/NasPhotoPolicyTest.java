package com.gathin.tvhome;
import org.junit.Test;
import static org.junit.Assert.*;
public class NasPhotoPolicyTest {
 @Test public void disabledAndUnconfiguredNeverFetch(){assertFalse(NasPhotoPolicy.refresh(false,true,false,false,100000,0,0));assertFalse(NasPhotoPolicy.refresh(true,false,false,false,100000,0,0));}
 @Test public void cacheAndRetryWindows(){long now=100000000;assertFalse(NasPhotoPolicy.refresh(true,true,true,true,now,now-1000,0));assertTrue(NasPhotoPolicy.refresh(true,true,true,true,now,now-NasPhotoCache.REFRESH,0));assertFalse(NasPhotoPolicy.refresh(true,true,false,true,now,0,now-1000));assertTrue(NasPhotoPolicy.refresh(true,true,false,true,now,0,now-NasPhotoCache.RETRY));assertTrue(NasPhotoPolicy.refresh(true,true,true,false,now,now,0));}
 @Test public void clockCorrectionDoesNotFreezeRefresh(){assertTrue(NasPhotoPolicy.refresh(true,true,true,true,1000,2000,2000));}
 @Test public void endpointsStayOnThePrivateLAN(){assertTrue(NasPhotoSettings.validURL("http://192.168.1.10:8791"));assertTrue(NasPhotoSettings.validURL("https://10.0.0.3:443"));assertFalse(NasPhotoSettings.validURL("http://8.8.8.8"));assertFalse(NasPhotoSettings.validURL("file:///etc/passwd"));assertFalse(NasPhotoSettings.validURL("http://user@192.168.1.1"));assertFalse(NasPhotoSettings.validURL("http://192.168.2.1/a"));assertFalse(NasPhotoSettings.validURL("http://192.168.2.1?token=secret"));}
}
