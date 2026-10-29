package com.gathin.photoframe;

import android.content.Context;
import android.graphics.*;
import android.os.SystemClock;
import android.view.View;
import java.util.*;

/** Photo first; a small preblurred backdrop keeps the glass cheap on TV GPUs. */
final class FrameView extends View {
    private final Paint paint=new Paint(Paint.ANTI_ALIAS_FLAG|Paint.FILTER_BITMAP_FLAG);
    private final RectF bounds=new RectF(),panel=new RectF(),shadowBounds=new RectF(),thumbBounds=new RectF();
    private final Matrix shaderMatrix=new Matrix();
    private final Map<Bitmap,BitmapShader> thumbShaders=new IdentityHashMap<>();
    private BitmapShader backdropShader;
    private final LinearGradient sheen=new LinearGradient(0,0,0,1,new int[]{0x32FFFFFF,0x08FFFFFF,0x08000000},null,Shader.TileMode.CLAMP);
    private final LinearGradient rim=new LinearGradient(0,0,1,1,0xA0FFFFFF,0x18FFFFFF,Shader.TileMode.CLAMP);
    private final boolean animate;
    private Bitmap current,previous,backdrop;
    private List<FrameSession.Photo> photos=Collections.emptyList();
    private int position;
    private long changed,railChanged;
    private float railFrom,railTarget;
    private boolean rail;
    private final Runnable hide=()->rail(false);
    public FrameView(Context c){this(c,true);}
    FrameView(Context c,boolean animate){super(c);this.animate=animate;setFocusable(true);setContentDescription("照片");}
    boolean railVisible(){return rail;}
    void items(List<FrameSession.Photo> items,int selected){photos=items;position=selected;Set<Bitmap> keep=Collections.newSetFromMap(new IdentityHashMap<>());for(FrameSession.Photo p:items){if(p.thumb!=null){keep.add(p.thumb);if(!thumbShaders.containsKey(p.thumb))thumbShaders.put(p.thumb,new BitmapShader(p.thumb,Shader.TileMode.CLAMP,Shader.TileMode.CLAMP));}}thumbShaders.keySet().retainAll(keep);invalidate();}
    void image(Bitmap bitmap){
        if(previous!=null)previous.recycle();previous=current;current=bitmap;changed=SystemClock.uptimeMillis();
        if(backdrop!=null)backdrop.recycle();backdrop=Bitmap.createBitmap(32,24,Bitmap.Config.ARGB_8888);
        Canvas background=new Canvas(backdrop);background.drawColor(Color.BLACK);
        float vw=Math.max(1,getWidth()),vh=Math.max(1,getHeight()),scale=Math.min(vw/bitmap.getWidth(),vh/bitmap.getHeight());
        float bw=bitmap.getWidth()*scale,bh=bitmap.getHeight()*scale;
        background.drawBitmap(bitmap,null,new RectF((vw-bw)*16/vw,(vh-bh)*12/vh,(vw+bw)*16/vw,(vh+bh)*12/vh),paint);
        int[] pixels=new int[32*24];backdrop.getPixels(pixels,0,32,0,0,32,24);
        for(int pass=0;pass<3;pass++){int[] out=pixels.clone();for(int y=0;y<24;y++)for(int x=0;x<32;x++){int r=0,g=0,b=0,n=0;for(int dy=-2;dy<=2;dy++)for(int dx=-2;dx<=2;dx++){int xx=Math.max(0,Math.min(31,x+dx)),yy=Math.max(0,Math.min(23,y+dy)),p=pixels[yy*32+xx];r+=Color.red(p);g+=Color.green(p);b+=Color.blue(p);n++;}out[y*32+x]=Color.rgb(r/n,g/n,b/n);}pixels=out;}
        backdrop.setPixels(pixels,0,32,0,0,32,24);backdropShader=new BitmapShader(backdrop,Shader.TileMode.CLAMP,Shader.TileMode.CLAMP);invalidate();
    }
    void rail(boolean visible){railFrom=railAlpha();railTarget=visible?1:0;railChanged=SystemClock.uptimeMillis();rail=visible;removeCallbacks(hide);if(visible)postDelayed(hide,4500);invalidate();}
    private float railAlpha(){float t=animate?Math.min(1,(SystemClock.uptimeMillis()-railChanged)/220f):1;t=1-(1-t)*(1-t);return railFrom+(railTarget-railFrom)*t;}
    private float dp(float n){return n*getResources().getDisplayMetrics().density;}
    private void photo(Canvas c,Bitmap b,int alpha){if(b==null||b.isRecycled())return;float scale=Math.min((float)getWidth()/b.getWidth(),(float)getHeight()/b.getHeight());float w=b.getWidth()*scale,h=b.getHeight()*scale;bounds.set((getWidth()-w)/2,(getHeight()-h)/2,(getWidth()+w)/2,(getHeight()+h)/2);paint.setAlpha(alpha);c.drawBitmap(b,null,bounds,paint);paint.setAlpha(255);}
    @Override protected void onDraw(Canvas c){
        super.onDraw(c);if(current==null)return;
        c.drawColor(Color.BLACK);float t=animate&&previous!=null?Math.min(1,(SystemClock.uptimeMillis()-changed)/320f):1;photo(c,previous,Math.round(255*(1-t)));photo(c,current,Math.round(255*t));
        if(t<1)postInvalidateOnAnimation();else if(previous!=null){previous.recycle();previous=null;}
        float alpha=railAlpha();if(alpha<=0)return;
        float width=Math.min(getWidth()-dp(96),dp(780)),height=dp(108),bottom=getHeight()-dp(34)+(1-alpha)*dp(16);
        panel.set((getWidth()-width)/2,bottom-height,(getWidth()+width)/2,bottom);
        int layer=c.saveLayerAlpha(0,0,getWidth(),getHeight(),Math.round(alpha*255));
        paint.setColor(0x30000000);paint.setShader(null);shadowBounds.set(panel.left-dp(6),panel.top-dp(2),panel.right+dp(6),panel.bottom+dp(10));c.drawRoundRect(shadowBounds,dp(30),dp(30),paint);
        if(backdrop!=null){shaderMatrix.setScale(getWidth()/32f,getHeight()/24f);backdropShader.setLocalMatrix(shaderMatrix);paint.setShader(backdropShader);paint.setAlpha(255);c.drawRoundRect(panel,dp(26),dp(26),paint);paint.setShader(null);}
        paint.setColor(0x99212A35);c.drawRoundRect(panel,dp(26),dp(26),paint);
        shaderMatrix.setScale(panel.width(),panel.height());shaderMatrix.postTranslate(panel.left,panel.top);sheen.setLocalMatrix(shaderMatrix);rim.setLocalMatrix(shaderMatrix);paint.setShader(sheen);paint.setAlpha(255);c.drawRoundRect(panel,dp(26),dp(26),paint);paint.setShader(null);
        paint.setStyle(Paint.Style.STROKE);paint.setStrokeWidth(dp(1));paint.setShader(rim);c.drawRoundRect(panel,dp(26),dp(26),paint);paint.setShader(null);paint.setStyle(Paint.Style.FILL);
        int slots=Math.min(7,photos.size()),start=Math.max(0,Math.min(position-3,photos.size()-slots));
        float gap=dp(10),tw=Math.min(dp(92),(width-dp(40)-(Math.max(0,slots-1))*gap)/Math.max(1,slots)),th=dp(68);
        float x=panel.centerX()-(slots*tw+Math.max(0,slots-1)*gap)/2;
        for(int i=start;i<start+slots;i++){
            Bitmap thumb=photos.get(i).thumb;boolean selected=i==position;
            RectF r=thumbBounds;r.set(x,panel.centerY()-th/2,x+tw,panel.centerY()+th/2);if(selected)r.inset(-dp(3),-dp(3));
            if(thumb!=null&&!thumb.isRecycled()){
                BitmapShader shader=thumbShaders.get(thumb);Matrix m=shaderMatrix;float s=Math.max(r.width()/thumb.getWidth(),r.height()/thumb.getHeight());m.setScale(s,s);m.postTranslate(r.centerX()-thumb.getWidth()*s/2,r.centerY()-thumb.getHeight()*s/2);shader.setLocalMatrix(m);paint.setShader(shader);paint.setAlpha(selected?255:200);c.drawRoundRect(r,dp(10),dp(10),paint);paint.setShader(null);paint.setAlpha(255);
            }
            if(selected){r.inset(-dp(3),-dp(3));paint.setStyle(Paint.Style.STROKE);paint.setStrokeWidth(dp(2));paint.setColor(0xF2FFFFFF);c.drawRoundRect(r,dp(13),dp(13),paint);paint.setStyle(Paint.Style.FILL);}x+=tw+gap;
        }
        c.restoreToCount(layer);if(Math.abs(alpha-railTarget)>.001f)postInvalidateOnAnimation();
    }
    void release(){removeCallbacks(hide);if(current!=null)current.recycle();if(previous!=null)previous.recycle();if(backdrop!=null)backdrop.recycle();current=previous=backdrop=null;backdropShader=null;thumbShaders.clear();photos=Collections.emptyList();}
}
