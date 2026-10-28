package com.gathin.tvhome;
import org.junit.Test;
import static org.junit.Assert.*;
import java.security.MessageDigest;
import java.nio.charset.StandardCharsets;

public class PublishedPhotoProtocolTest {
 private static final String HASH="ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
 @Test public void hardAgeExpiryCannotBeKeptIndefinitelyOffline(){assertTrue(PublishedPhotoProtocol.validUntil(101,100));assertFalse(PublishedPhotoProtocol.validUntil(100,100));assertFalse(PublishedPhotoProtocol.validUntil(0,100));}
 @Test public void immutableAssetMustMatchHashPathAndTvDimensions(){
  String id=HASH.substring(0,32),url="/v2/wallpapers/assets/"+HASH+".jpg";
  assertTrue(PublishedPhotoProtocol.asset(id,url,HASH,1000,1920,1080));
  assertFalse(PublishedPhotoProtocol.asset(id,"/v1/photos/"+id,HASH,1000,1920,1080));
  assertFalse(PublishedPhotoProtocol.asset(id,url,HASH,1000,810,1080));
  assertFalse(PublishedPhotoProtocol.asset(id,url,HASH,4*1024*1024,1920,1080));
  assertFalse(PublishedPhotoProtocol.asset(id,url+"/../other",HASH,1000,1920,1080));
 }
 @Test public void truncatedOrAlteredBytesCannotPassIntegrityCheck() throws Exception{
  byte[] digest=MessageDigest.getInstance("SHA-256").digest("abc".getBytes(StandardCharsets.UTF_8));
  assertTrue(PublishedPhotoProtocol.matches(HASH,3,3,digest));
  assertFalse(PublishedPhotoProtocol.matches(HASH,3,2,digest));
  digest=MessageDigest.getInstance("SHA-256").digest("abd".getBytes(StandardCharsets.UTF_8));
  assertFalse(PublishedPhotoProtocol.matches(HASH,3,3,digest));
 }
}
