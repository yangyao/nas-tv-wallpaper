package com.gathin.photoframe;
import org.junit.Test;
import static org.junit.Assert.*;

public class PhotoTimelineTest {
 @Test public void missingNextNeverChangesSelectionOrQueuesKeys(){PhotoTimeline<Integer> t=new PhotoTimeline<>();t.append(1);for(int i=0;i<100;i++)assertFalse(t.select(t.position()+1));t.append(2);assertEquals(Integer.valueOf(1),t.current());assertTrue(t.select(1));assertEquals(Integer.valueOf(2),t.current());assertTrue(t.select(0));assertEquals(Integer.valueOf(1),t.current());}
 @Test public void historyIsStableAndBoundedWhileReplenishing(){PhotoTimeline<Integer> t=new PhotoTimeline<>();for(int i=0;i<21;i++)t.append(i);for(int i=1;i<1000;i++){assertTrue(t.select(t.position()+1));assertEquals(Integer.valueOf(i),t.current());t.append(i+20);assertTrue(t.size()<=60);assertTrue(t.position()<=40);assertTrue(t.ahead()<=20);}assertTrue(t.select(t.position()-1));assertEquals(Integer.valueOf(998),t.current());}
 @Test public void stationaryViewerDoesNotLoseCurrent(){PhotoTimeline<Integer> t=new PhotoTimeline<>();for(int i=0;i<100;i++)t.append(i);assertEquals(60,t.size());assertEquals(Integer.valueOf(0),t.current());assertFalse(t.append(101));assertFalse(t.append(59));}
 @Test public void sourceRequiresExplicitOptInAndPrivateOrigin(){String token="test-token-not-for-production";assertTrue(new PhotoSource(true,"http://10.0.0.2:8791/",token).enabled);for(String url:new String[]{"http://example.com","http://8.8.8.8","http://10.0.0.2/path","http://user@10.0.0.2","http://10.0.0.2?token=x","file:///tmp"})assertFalse(url,new PhotoSource(true,url,token).enabled);assertFalse(new PhotoSource(false,"http://10.0.0.2",token).enabled);assertFalse(new PhotoSource(true,"http://10.0.0.2","short").enabled);assertNotEquals(new PhotoSource(true,"http://10.0.0.2",token).key,new PhotoSource(true,"http://10.0.0.3",token).key);}
}
