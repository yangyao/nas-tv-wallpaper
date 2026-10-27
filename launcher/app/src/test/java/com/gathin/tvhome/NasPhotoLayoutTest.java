package com.gathin.tvhome;
import org.junit.Test;
import static org.junit.Assert.*;
public class NasPhotoLayoutTest {
 @Test public void originalShapeSelectsLayoutNotThePaddedFrame(){
  assertTrue(NasPhotoLayout.portrait(3000,4000));
  assertTrue(NasPhotoLayout.portrait(3000,3000));
  assertFalse(NasPhotoLayout.portrait(4000,3000));
  assertFalse(NasPhotoLayout.portrait(0,0));
  assertFalse(NasPhotoLayout.portrait(100000,100000));
 }
 @Test public void extractsCompleteForegroundForPhoneAndSquarePhotos(){
  assertArrayEquals(new int[]{555,0,810,1080},NasPhotoLayout.foreground(3000,4000,1920,1080));
  assertArrayEquals(new int[]{656,0,607,1080},NasPhotoLayout.foreground(2250,4000,1920,1080));
  assertArrayEquals(new int[]{420,0,1080,1080},NasPhotoLayout.foreground(3000,3000,1920,1080));
  assertArrayEquals(new int[]{0,0,1920,1080},NasPhotoLayout.foreground(0,0,1920,1080));
 }
}
