package com.gathin.tvhome;
import org.junit.Test;
import static org.junit.Assert.*;

public class HomeIdleCountdownTest {
 @Test public void interactionGrantsAFullNewInterval(){HomeIdleCountdown timer=new HomeIdleCountdown();timer.restart(0,300000,true);timer.restart(240000,300000,true);assertFalse(timer.consume(300000));assertEquals(240000,timer.remaining(300000));assertTrue(timer.consume(540000));assertFalse(timer.consume(540001));}
 @Test public void returningFromAnotherAppDoesNotInheritIdleTime(){HomeIdleCountdown timer=new HomeIdleCountdown();timer.restart(0,300000,true);timer.cancel();assertFalse(timer.consume(900000));timer.restart(900000,300000,true);assertFalse(timer.consume(1199999));assertTrue(timer.consume(1200000));}
 @Test public void dialogsAndDisabledModeCannotTriggerStaleCallbacks(){HomeIdleCountdown timer=new HomeIdleCountdown();timer.restart(0,300000,true);timer.restart(1000,300000,false);assertEquals(-1,timer.remaining(301000));assertFalse(timer.consume(301000));timer.restart(400000,0,true);assertFalse(timer.consume(1000000));timer.restart(1000000,600000,true);assertFalse(timer.consume(1599999));assertTrue(timer.consume(1600000));}
}
