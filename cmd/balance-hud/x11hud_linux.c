#include "x11hud.h"
#include <X11/Xlib.h>
#include <X11/extensions/Xrandr.h>
#include <errno.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

enum { slots = 4, item = 36, pad = 4, line = 18 };
struct Hud {
    Display *d;
    Window root, bar, tip;
    GC gc;
    XFontStruct *font;
    unsigned long bg, fg, track, muted, accent[slots];
    int percent[slots], stale[slots];
    int sx, sy, sw, sh, x, y, w, height;
    int edge, numbers, hover, pressed, moved, down_x, down_y;
    double offset;
    char mark[slots][8], number[slots][16], details[slots][1024];
};

static int clamp(int v, int min, int max) {
    return v < min ? min : v > max ? max : v;
}
static unsigned long color(Hud *h, const char *name, unsigned long fallback) {
    XColor pixel, actual;
    return XAllocNamedColor(h->d, DefaultColormap(h->d, DefaultScreen(h->d)), name, &pixel, &actual)
        ? pixel.pixel : fallback;
}
static int vertical(Hud *h) { return h->edge % 2; }
static int step(Hud *h) { return !vertical(h) && h->numbers ? 74 : item; }
static void text(Hud *h, Window w, int x, int y, const char *s, int len, unsigned long ink) {
    XSetForeground(h->d, h->gc, ink);
    XDrawString(h->d, w, h->gc, x, y, s, len);
}
static void draw(Hud *h) {
    XClearWindow(h->d, h->bar);
    for (int i = 0; i < slots; i++) {
        int x = pad + (vertical(h) ? 0 : i * step(h));
        int y = pad + (vertical(h) ? i * item : 0);
        unsigned long ink = h->stale[i] ? h->muted : h->accent[i];
        XSetForeground(h->d, h->gc, h->track);
        XDrawArc(h->d, h->bar, h->gc, x + 4, y + 4, 27, 27, 0, 360 * 64);
        if (h->percent[i] > 0) {
            XSetForeground(h->d, h->gc, ink);
            XDrawArc(h->d, h->bar, h->gc, x + 4, y + 4, 27, 27, 90*64,
                     -h->percent[i] * 360 * 64 / 100);
        }
        int count = (int)strlen(h->mark[i]);
        int width = XTextWidth(h->font, h->mark[i], count);
        text(h, h->bar, x + 18 - width / 2, y + 22, h->mark[i], count, ink);
        if (h->numbers) text(h, h->bar, x + 38, y + 22, h->number[i],
                             (int)strlen(h->number[i]), h->fg);
    }
}
static void place(Hud *h) {
    int v = vertical(h);
    h->w = 2 * pad + (v ? (h->numbers ? 74 : item) : slots * step(h));
    h->height = 2 * pad + (v ? slots * item : item);
    h->x = h->sx + (v ? (h->edge == 1 ? h->sw - h->w - 3 : 3)
                          : (int)((h->sw - h->w) * h->offset));
    h->y = h->sy + (v ? (int)((h->sh - h->height) * h->offset)
                          : (h->edge == 2 ? h->sh - h->height - 3 : 3));
    XMoveResizeWindow(h->d, h->bar, h->x, h->y, h->w, h->height);
    draw(h);
}
static void hide_tip(Hud *h) {
    h->hover = -1;
    XUnmapWindow(h->d, h->tip);
}
static void draw_tip(Hud *h) {
    if (h->hover < 0) return;
    const char *s = h->details[h->hover];
    int longest = 0, count = 0, lengths[32];
    const char *lines[32];
    while (*s && count < 32) {
        const char *end = strchr(s, '\n');
        int n = end ? (int)(end - s) : (int)strlen(s);
        lengths[count] = n; lines[count++] = s;
        int width = XTextWidth(h->font, s, n);
        if (width > longest) longest = width;
        if (!end) break;
        s = end + 1;
    }
    int width = clamp(longest + 16, 64, h->sw), height = count * line + 12;
    int x = h->x, y = h->y + h->height + 4;
    if (h->edge == 1) { x = h->x - width - 4; y = h->y; }
    if (h->edge == 2) y = h->y - height - 4;
    if (h->edge == 3) { x = h->x + h->w + 4; y = h->y; }
    XMoveResizeWindow(h->d, h->tip, clamp(x,h->sx,h->sx+h->sw-width),
        clamp(y,h->sy,h->sy+h->sh-height), width, height);
    XMapRaised(h->d, h->tip);
    XClearArea(h->d, h->tip, 0, 0, 0, 0, False);
    for (int i = 0; i < count; i++)
        text(h,h->tip,8,18+i*line,lines[i],lengths[i],h->fg);
}
static void pick(Hud *h, int x, int y) {
    int i = ((vertical(h) ? y : x) - pad) / step(h);
    if (i < 0 || i >= slots) { hide_tip(h); return; }
    if (i != h->hover) { h->hover = i; draw_tip(h); }
}
static void drop(Hud *h, int x, int y) {
    x = clamp(x,h->sx,h->sx+h->sw-1)-h->sx;
    y = clamp(y,h->sy,h->sy+h->sh-1)-h->sy;
    int distances[] = {y, h->sw-1-x, h->sh-1-y, x};
    for (int i=0;i<4;i++) if (distances[i]<distances[h->edge]) h->edge=i;
    h->offset = vertical(h) ? (double)y/(h->sh-1) : (double)x/(h->sw-1);
    place(h);
}

Hud *hud_open(int edge, double offset, int numbers) {
    Display *d = XOpenDisplay(NULL);
    if (!d) return NULL;
    Hud *h = calloc(1, sizeof(*h));
    if (!h) { XCloseDisplay(d); return NULL; }
    h->d=d; h->root=DefaultRootWindow(d);
    h->edge=edge; h->offset=offset; h->numbers=numbers; h->hover=-1;
    h->sw=DisplayWidth(d,DefaultScreen(d)); h->sh=DisplayHeight(d,DefaultScreen(d));
    XRRScreenResources *res = XRRGetScreenResourcesCurrent(d,h->root);
    if (res) {
        RROutput primary=XRRGetOutputPrimary(d,h->root);
        for(int i=-1;i<res->noutput;i++){
            RROutput output=(i<0)?primary:res->outputs[i];
            if(!output)continue;
            XRROutputInfo *oi=XRRGetOutputInfo(d,res,output);
            XRRCrtcInfo *ci=oi&&oi->crtc?XRRGetCrtcInfo(d,res,oi->crtc):NULL;
            int found=ci&&ci->width>1&&ci->height>1;
            if(found){h->sx=ci->x;h->sy=ci->y;h->sw=ci->width;h->sh=ci->height;}
            if(ci)XRRFreeCrtcInfo(ci);
            if(oi)XRRFreeOutputInfo(oi);
            if(found)break;
        }
        XRRFreeScreenResources(res);
    }
    h->bg=color(h,"#151b23",BlackPixel(d,DefaultScreen(d)));
    h->fg=color(h,"#e7eef5",WhitePixel(d,DefaultScreen(d)));
    h->track=color(h,"#465363",h->fg);
    h->muted=color(h,"#9da8b4",h->fg);
    const char *accent[]={"#72e5bd","#8fb8ff","#8fb8ff","#ffb877"};
    for(int i=0;i<slots;i++)h->accent[i]=color(h,accent[i],h->fg);
    XSetWindowAttributes a={.override_redirect=True,.background_pixel=h->bg};
    h->bar=XCreateWindow(d,h->root,0,0,1,1,0,CopyFromParent,InputOutput,
        CopyFromParent,CWOverrideRedirect|CWBackPixel,&a);
    h->tip=XCreateWindow(d,h->root,0,0,1,1,0,CopyFromParent,InputOutput,
        CopyFromParent,CWOverrideRedirect|CWBackPixel,&a);
    XStoreName(d,h->bar,"Balance HUD");
    XStoreName(d,h->tip,"Balance HUD quota details");
    XSelectInput(d,h->bar,ExposureMask|ButtonPressMask|ButtonReleaseMask|
                          PointerMotionMask|LeaveWindowMask);
    XSelectInput(d,h->tip,ExposureMask);
    h->gc=XCreateGC(d,h->bar,0,NULL);
    h->font=XLoadQueryFont(d,"fixed");
    if(!h->font){hud_close(h);return NULL;}
    XSetFont(d,h->gc,h->font->fid);
    place(h);
    XMapRaised(d,h->bar);
    XFlush(d);
    return h;
}
void hud_close(Hud *h) {
    if(h->font)XFreeFont(h->d,h->font);
    XFreeGC(h->d,h->gc);
    XCloseDisplay(h->d);
    free(h);
}
void hud_row(Hud *h,int i,const char *mark,const char *number,const char *details,int percent,int stale) {
    if(i<0||i>=slots)return;
    h->percent[i]=percent;h->stale[i]=stale;
    snprintf(h->mark[i],sizeof(h->mark[i]),"%s",mark);
    snprintf(h->number[i],sizeof(h->number[i]),"%s",number);
    snprintf(h->details[i],sizeof(h->details[i]),"%s",details);
    draw(h);
    if(h->hover==i)draw_tip(h);
    XFlush(h->d);
}
int hud_step(Hud *h,int timeout) {
    if(!XPending(h->d)){
        struct pollfd fd={.fd=ConnectionNumber(h->d),.events=POLLIN};
        if(poll(&fd,1,timeout)<=0)return 0;
    }
    while(XPending(h->d)){
        XEvent e;
        XNextEvent(h->d,&e);
        switch(e.type){
        case Expose:
            // Expose can be queued by mapping the tooltip. Clear/draw without remapping.
            if(e.xexpose.count==0 && e.xexpose.window==h->bar)draw(h);
            if(e.xexpose.count==0 && e.xexpose.window==h->tip)draw_tip(h);
            break;
        case LeaveNotify:hide_tip(h);break;
        case MotionNotify:
            if(h->pressed){
                if(abs(e.xmotion.x_root-h->down_x)+abs(e.xmotion.y_root-h->down_y)>=6)
                    h->moved=1;
            }else if(e.xmotion.window==h->bar)pick(h,e.xmotion.x,e.xmotion.y);
            break;
        case ButtonPress:
            if(e.xbutton.button==Button1){
                hide_tip(h);
                h->pressed=1;h->moved=0;
                h->down_x=e.xbutton.x_root;h->down_y=e.xbutton.y_root;
            }else if(e.xbutton.button==Button3){
                if(e.xbutton.state&ShiftMask)return 1;
                hide_tip(h);h->numbers=!h->numbers;place(h);XFlush(h->d);return 2;
            }else if(e.xbutton.button==Button2)return 3;
            break;
        case ButtonRelease:
            if(e.xbutton.button==Button1&&h->pressed){
                h->pressed=0;
                if(h->moved){
                    drop(h,e.xbutton.x_root,e.xbutton.y_root);
                    XFlush(h->d);return 2;
                }
                pick(h,e.xbutton.x,e.xbutton.y);
            }
            break;
        }
    }
    XFlush(h->d);
    return 0;
}
int hud_edge(Hud *h){return h->edge;}
double hud_offset(Hud *h){return h->offset;}
int hud_numbers(Hud *h){return h->numbers;}
