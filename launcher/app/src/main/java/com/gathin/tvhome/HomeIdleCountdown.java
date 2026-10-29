package com.gathin.tvhome;

/** Monotonic, one-shot idle deadline. Losing Home focus cancels rather than defers it. */
final class HomeIdleCountdown {
    private long deadline=-1;
    void restart(long now,long delay,boolean eligible){deadline=eligible&&delay>0?now+delay:-1;}
    void cancel(){deadline=-1;}
    long remaining(long now){return deadline<0?-1:Math.max(0,deadline-now);}
    boolean consume(long now){if(deadline<0||now<deadline)return false;deadline=-1;return true;}
}
