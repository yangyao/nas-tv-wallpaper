package com.gathin.photoframe;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/** Stable, bounded history. Missing next photos never create deferred key presses. */
public final class PhotoTimeline<T> {
    public static final int CAPACITY = 60;
    public static final int HISTORY = 40;
    private final List<T> items = new ArrayList<>();
    private int current;
    public synchronized List<T> snapshot() { return new ArrayList<>(items); }
    public synchronized int position() { return current; }
    public synchronized int size() { return items.size(); }
    public synchronized int ahead() { return Math.max(0, items.size() - current - 1); }
    public synchronized T current() { return items.isEmpty() ? null : items.get(current); }
    public synchronized T at(int position) { return position < 0 || position >= items.size() ? null : items.get(position); }
    public synchronized boolean select(int position) {
        if (position < 0 || position >= items.size()) return false;
        current = position;
        while (current > HISTORY) { items.remove(0); current--; }
        return true;
    }
    public synchronized boolean append(T item) {
        if (item == null || (!items.isEmpty() && Objects.equals(items.get(items.size()-1), item))) return false;
        if (items.size() >= CAPACITY) {
            if (current == 0) return false;
            items.remove(0); current--;
        }
        items.add(item); return true;
    }
}
