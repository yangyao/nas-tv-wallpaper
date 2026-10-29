package com.gathin.tvhome;

import com.gathin.photoframe.FrameActivity;
import com.gathin.photoframe.PhotoSource;
import java.io.File;
import java.util.*;

/** A mode of Home, not an additional application or desktop control. */
public final class PhotoFrameActivity extends FrameActivity {
    @Override protected PhotoSource photoSource(){NasPhotoSettings s=NasPhotoSettings.load(this);return new PhotoSource(s.enabled,s.base,s.token);}
    @Override protected List<File> seedPhotos(){List<File> files=new ArrayList<>(NasPhotoCache.current(this));if(files.isEmpty())files.addAll(Config.load(this).wallpapers);return files;}
    @Override protected boolean animationsEnabled(){Config c=Config.load(this);return c.animations&&!c.reducedMotion;}
}
