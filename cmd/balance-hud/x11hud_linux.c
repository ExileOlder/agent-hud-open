#include "x11hud.h"
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xft/Xft.h>
#include <X11/extensions/shape.h>
#include <math.h>
#include <time.h>
#include <X11/extensions/Xrandr.h>
#include <errno.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

enum { slots = 4, item = 70, wide = 120, pad = 7, line = 27 };
struct Hud {
    Display *d;
    Window root, bar, tip;
    GC gc;
    XftFont *font, *heading;
    XftDraw *tip_draw;
    XftColor text_color, muted_text, button_color;
    Pixmap bitmap;
    int bw, bh;
    int tip_x, tip_y, tip_w, tip_h, settings_top, settings, motion;
    long long leave_at;
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
static int step(Hud *h) { return !vertical(h) && h->numbers ? wide : item; }
static long long millis(void) {
    struct timespec t;
    clock_gettime(CLOCK_MONOTONIC, &t);
    return t.tv_sec*1000LL+t.tv_nsec/1000000;
}
static void text(Hud *h, int x, int y, const char *s, int n, XftFont *font, XftColor *ink) {
    XftDrawStringUtf8(h->tip_draw,ink,font,x,y,(const FcChar8 *)s,n);
}
static void label(Hud *h,int x,int y,const char *s,XftFont *font,XftColor *ink){
    text(h,x,y,s,(int)strlen(s),font,ink);
}
static void box(Hud *h, int x,int y,unsigned w,unsigned height,unsigned long color) {
    XSetForeground(h->d,h->gc,color);
    XFillRectangle(h->d,h->tip,h->gc,x,y,w,height);
}
static void shape(Hud *h,Window w,int width,int height,int r){
    if(!XShapeQueryExtension(h->d,&(int){0},&(int){0}))return;
    XRectangle rects[512];
    int n=0;
    int radius=clamp(r,1,clamp(width,1,height)/2);
    for(int y=0;y<radius;y++){
        double dy=radius-y-.5;
        int left=radius-(int)(sqrt(radius*radius-dy*dy)+.5);
        rects[n++]=(XRectangle){left,y,width-2*left,1};
    }
    if(height>2*radius)rects[n++]=(XRectangle){0,radius,width,height-2*radius};
    for(int y=radius-1;y>=0;y--){
        XRectangle row=rects[y];
        row.y=height-1-y;
        rects[n++]=row;
    }
    XShapeCombineRectangles(h->d,w,ShapeBounding,0,0,rects,n,ShapeSet,YXBanded);
}
static void draw(Hud *h) {
    if(h->bitmap&&h->bw==h->w&&h->bh==h->height)
        XCopyArea(h->d,h->bitmap,h->bar,h->gc,0,0,h->w,h->height,0,0);
}
void hud_bitmap(Hud *h,int w,int height,const unsigned char *pixels){
    if(w!=h->w||height!=h->height)return;
    if(!h->bitmap||h->bw!=w||h->bh!=height){
        if(h->bitmap)XFreePixmap(h->d,h->bitmap);
        h->bitmap=XCreatePixmap(h->d,h->bar,w,height,DefaultDepth(h->d,DefaultScreen(h->d)));
        h->bw=w;h->bh=height;
    }
    XImage *img=XCreateImage(h->d,DefaultVisual(h->d,DefaultScreen(h->d)),
        DefaultDepth(h->d,DefaultScreen(h->d)),ZPixmap,0,(char *)pixels,w,height,32,w*4);
    if(!img)return;
    if(img->bits_per_pixel==32 && img->byte_order==LSBFirst && img->red_mask==0xff0000 &&
       img->green_mask==0xff00 && img->blue_mask==0xff){
        XPutImage(h->d,h->bitmap,h->gc,img,0,0,0,0,w,height);
    }else{
        img->data=NULL;
        XDestroyImage(img);
        return;
    }
    img->data=NULL;XDestroyImage(img);
    draw(h);XFlush(h->d);
}
static void place(Hud *h) {
    int v = vertical(h);
    h->w = 2 * pad + (v ? (h->numbers ? wide : item) : slots * step(h));
    h->height = 2 * pad + (v ? slots * item : item);
    h->x = h->sx + (v ? (h->edge == 1 ? h->sw - h->w - 3 : 3)
                          : (int)((h->sw - h->w) * h->offset));
    h->y = h->sy + (v ? (int)((h->sh - h->height) * h->offset)
                          : (h->edge == 2 ? h->sh - h->height - 3 : 3));
    XMoveResizeWindow(h->d, h->bar, h->x, h->y, h->w, h->height);
    shape(h,h->bar,h->w,h->height,42);
    draw(h);
}
static void hide_tip(Hud *h) {
    h->hover=-1;h->leave_at=0;h->settings=0;
    XUnmapWindow(h->d,h->tip);
}
static void draw_tip(Hud *h) {
    if(h->hover<0)return;
    const char *s=h->details[h->hover];
    const char *lines[28];int lengths[28],count=0,longest=0;
    while(*s&&count<28){
        const char *end=strchr(s,'\n');
        int n=end?(int)(end-s):(int)strlen(s);
        lines[count]=s;lengths[count++]=n;
        XGlyphInfo ext;
        XftTextExtentsUtf8(h->d,h->font,(const FcChar8*)s,n,&ext);
        longest=longest>ext.xOff?longest:ext.xOff;
        if(!end)break;
        s=end+1;
    }
    int width=clamp(longest+44,320,h->sw);
    h->settings_top=43+line*(count>0?count-1:0);
    int height=h->settings?342:h->settings_top+55;
    int x=h->x,y=h->y+h->height;
    if(h->edge==1){x=h->x-width;y=h->y+h->hover*item;}
    if(h->edge==2)y=h->y-height;
    if(h->edge==3){x=h->x+h->w;y=h->y+h->hover*item;}
    x=clamp(x,h->sx,h->sx+h->sw-width);
    y=clamp(y,h->sy,h->sy+h->sh-height);
    h->tip_x=x;h->tip_y=y;h->tip_w=width;h->tip_h=height;
    XMoveResizeWindow(h->d,h->tip,x,y,width,height);
    shape(h,h->tip,width,height,16);
    XMapRaised(h->d,h->tip);
    XClearArea(h->d,h->tip,0,0,0,0,False);
    if(h->settings){
        label(h,20,33,"Balance HUD settings",h->heading,&h->text_color);
        label(h,20,57,"Local appearance controls",h->font,&h->muted_text);
        const char *options[]={h->numbers?"Hide percentages":"Show percentages",
          h->motion?"Turn off animation":"Turn on animation",
          "Move to top edge","Move to right edge","Move to bottom edge","Move to left edge","Refresh balances","Close settings"};
        for(int i=0;i<8;i++){
            int yy=72+i*32;
            box(h,14,yy,width-28,28,h->track);
            label(h,24,yy+19,options[i],h->font,&h->text_color);
        }
    }else{
        if(count>0)text(h,20,31,lines[0],lengths[0],h->heading,&h->text_color);
        for(int i=1;i<count;i++)text(h,20,31+i*line,lines[i],lengths[i],h->font,&h->muted_text);
        box(h,13,h->settings_top,width-26,38,h->track);
        label(h,25,h->settings_top+24,"Settings  ›",h->heading,&h->button_color);
    }
}
static void pick(Hud *h, int x, int y) {
    int i=((vertical(h)?y:x)-pad)/step(h);
    if(i<0||i>=slots){hide_tip(h);return;}
    h->leave_at=0;
    if(h->settings)return;
    if(i!=h->hover){h->hover=i;draw_tip(h);}
}
static void drop(Hud *h, int x, int y) {
    x = clamp(x,h->sx,h->sx+h->sw-1)-h->sx;
    y = clamp(y,h->sy,h->sy+h->sh-1)-h->sy;
    int distances[] = {y, h->sw-1-x, h->sh-1-y, x};
    for (int i=0;i<4;i++) if (distances[i]<distances[h->edge]) h->edge=i;
    h->offset = vertical(h) ? (double)y/(h->sh-1) : (double)x/(h->sw-1);
    place(h);
}

Hud *hud_open(int edge, double offset, int numbers,int motion) {
    Display *d = XOpenDisplay(NULL);
    if (!d) return NULL;
    Hud *h = calloc(1, sizeof(*h));
    if (!h) { XCloseDisplay(d); return NULL; }
    h->d=d; h->root=DefaultRootWindow(d);
    h->edge=edge; h->offset=offset; h->numbers=numbers; h->hover=-1; h->motion=motion;
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
    h->bg=color(h,"#0d1116",BlackPixel(d,DefaultScreen(d)));
    h->fg=color(h,"#e7eef5",WhitePixel(d,DefaultScreen(d)));
    h->track=color(h,"#1d2832",h->fg);
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
    XSelectInput(d,h->tip,ExposureMask|EnterWindowMask|LeaveWindowMask|ButtonPressMask);
    h->gc=XCreateGC(d,h->bar,0,NULL);
    int screen=DefaultScreen(d);
    Visual *visual=DefaultVisual(d,screen);
    Colormap cmap=DefaultColormap(d,screen);
    h->font=XftFontOpenName(d,screen,"sans:pixelsize=15");
    h->heading=XftFontOpenName(d,screen,"sans:bold:pixelsize=17");
    h->tip_draw=XftDrawCreate(d,h->tip,visual,cmap);
    if(!h->font||!h->heading||!h->tip_draw){hud_close(h);return NULL;}
    XftColorAllocName(d,visual,cmap,"#f2f4f6",&h->text_color);
    XftColorAllocName(d,visual,cmap,"#aebbc8",&h->muted_text);
    XftColorAllocName(d,visual,cmap,"#90c9ff",&h->button_color);
    place(h);
    XMapRaised(d,h->bar);
    XFlush(d);
    return h;
}
void hud_close(Hud *h) {
    if(h->bitmap)XFreePixmap(h->d,h->bitmap);
    if(h->tip_draw)XftDrawDestroy(h->tip_draw);
    if(h->font)XftFontClose(h->d,h->font);
    if(h->heading)XftFontClose(h->d,h->heading);
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
        poll(&fd,1,timeout);
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
        case LeaveNotify:if(!h->settings)h->leave_at=millis()+180;break;
        case EnterNotify:h->leave_at=0;break;
        case MotionNotify:
            if(h->pressed){
                if(abs(e.xmotion.x_root-h->down_x)+abs(e.xmotion.y_root-h->down_y)>=6)
                    h->moved=1;
            }else if(e.xmotion.window==h->bar)pick(h,e.xmotion.x,e.xmotion.y);
            break;
        case ButtonPress:
            if(e.xbutton.window==h->tip){
                if(e.xbutton.button==Button3){hide_tip(h);break;}
                if(e.xbutton.button!=Button1)break;
                int y=e.xbutton.y;
                if(!h->settings && y>=h->settings_top && y<h->settings_top+38){
                    h->settings=1;draw_tip(h);break;
                }
                if(h->settings && y>=72 && y<72+8*32){
                    int option=(y-72)/32;
                    if(option==0)h->numbers=!h->numbers;
                    else if(option==1)h->motion=!h->motion;
                    else if(option==6)return 3;
                    else if(option==7){hide_tip(h);break;}
                    else {h->edge=option-2;place(h);hide_tip(h);}
                    if(option<2){place(h);draw_tip(h);}
                    XFlush(h->d);return 2;
                }
                break;
            }
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
    if(h->leave_at && millis()>=h->leave_at)hide_tip(h);
    XFlush(h->d);
    return 0;
}
int hud_edge(Hud *h){return h->edge;}
double hud_offset(Hud *h){return h->offset;}
int hud_numbers(Hud *h){return h->numbers;}

int hud_hover(Hud *h){return h->hover;}
int hud_motion(Hud *h){return h->motion;}
