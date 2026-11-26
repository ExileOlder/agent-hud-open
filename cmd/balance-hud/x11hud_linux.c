#include "x11hud.h"
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xatom.h>
#include <X11/keysym.h>
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

enum { slots = 4, item = 92, wide = 148, pad = 4, peek_height = 64, peek_pad = 30, peek_gap = 12, peek_spacing = 30, gear = 45, max_cells = 8 };
typedef struct { char label[96], value[192]; int tone; } PeekCell;
struct Hud {
    Display *d;
    Window root, bar, tip, prefs, notice;
    Atom wm_delete, opacity;
    GC gc;
    XftFont *font, *heading, *prefs_font, *notice_font;
    XftDraw *tip_draw, *prefs_draw, *notice_draw;
    XftColor text_color, muted_text, tones[4];
    Pixmap bitmap;
    int bw, bh;
    int tip_x, tip_y, tip_w, tip_h, motion;
    int prefs_visible, page, prefs_count;
    PeekCell prefs_rows[8];
    char prefs_help[192];
    unsigned long card_bg, side_bg, selected_bg;
    long long leave_at, down_at, peek_at;
    double peek_progress;
    int peek_target;
    long long click_at, notice_click_at, notice_until;
    int click_slot, notice_visible;
    char notice_body[128];
    unsigned long bg, fg, track;
    int sx, sy, sw, sh, x, y, w, height;
    int edge, numbers, hover, pressed, moved, outlined, down_x, down_y, grab_x, grab_y;
    double offset;
    PeekCell cells[slots][max_cells];
    int counts[slots];
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
// Paint the gear directly; Xft has no fallback for missing symbol glyphs.
static void paint_gear(Hud *h,int x,int y) {
    XPoint teeth[32];
    for(int i=0;i<32;i++){
        double angle=(i+.5)*2*M_PI/32;
        double radius=(i%4==1||i%4==2)?10:7;
        teeth[i]=(XPoint){x+(short)lround(radius*cos(angle)),y+(short)lround(radius*sin(angle))};
    }
    XSetForeground(h->d,h->gc,h->fg);
    XFillPolygon(h->d,h->tip,h->gc,teeth,32,Nonconvex,CoordModeOrigin);
    XSetForeground(h->d,h->gc,h->bg);
    XFillArc(h->d,h->tip,h->gc,x-4,y-4,8,8,0,360*64);
}
static void shape(Hud *h,Window w,int width,int height,int r,const XRectangle *clip){
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
    if(clip){
        int written=0;
        for(int i=0;i<n;i++){
            XRectangle q=rects[i];
            int x1=q.x>clip->x?q.x:clip->x,y1=q.y>clip->y?q.y:clip->y;
            int x2=q.x+q.width<clip->x+clip->width?q.x+q.width:clip->x+clip->width;
            int y2=q.y+q.height<clip->y+clip->height?q.y+q.height:clip->y+clip->height;
            if(x2>x1&&y2>y1)rects[written++]=(XRectangle){x1,y1,x2-x1,y2-y1};
        }
        n=written;
    }
    XShapeCombineRectangles(h->d,w,ShapeBounding,0,0,rects,n,ShapeSet,YXBanded);
}
static void draw(Hud *h) {
    if(h->bitmap&&h->bw==h->w&&h->bh==h->height)
        XCopyArea(h->d,h->bitmap,h->bar,h->gc,0,0,h->w,h->height,0,0);
    if(!h->outlined)return;
    // Match the original HUD's rounded white 4:3 dashed outline.
    XPoint path[37];
    int n=0,r=(h->w<h->height?h->w:h->height)/2-2;
    int xs[]={h->w-r-2,h->w-r-2,r+2,r+2};
    int ys[]={r+2,h->height-r-2,h->height-r-2,r+2};
    for(int corner=0;corner<4;corner++){
        for(int i=0;i<=8;i++){
            double a=(-M_PI/2)+(corner+i/8.0)*M_PI/2;
            path[n++]=(XPoint){xs[corner]+lround(r*cos(a)),ys[corner]+lround(r*sin(a))};
        }
    }
    path[n++]=path[0];
    XSetForeground(h->d,h->gc,h->fg);
    XSetLineAttributes(h->d,h->gc,1,LineOnOffDash,CapButt,JoinRound);
    XSetDashes(h->d,h->gc,0,(char[]){4,3},2);
    XDrawLines(h->d,h->bar,h->gc,path,n,CoordModeOrigin);
    XSetLineAttributes(h->d,h->gc,0,LineSolid,CapButt,JoinMiter);
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
    h->w = v ? (h->numbers ? wide : item) : 2 * pad + slots * step(h);
    h->height = 2 * pad + (v ? slots * item : item);
    h->x = h->sx + (v ? (h->edge == 1 ? h->sw - h->w - 3 : 3)
                          : (int)((h->sw - h->w) * h->offset));
    h->y = h->sy + (v ? (int)((h->sh - h->height) * h->offset)
                          : (h->edge == 2 ? h->sh - h->height - 3 : 3));
    XMoveResizeWindow(h->d, h->bar, h->x, h->y, h->w, h->height);
    shape(h,h->bar,h->w,h->height,50,NULL);
    draw(h);
}
static void hide_tip(Hud *h) {
    h->hover=-1;h->leave_at=0;h->peek_at=0;h->peek_progress=0;h->peek_target=0;
    XUnmapWindow(h->d,h->tip);
}
static int text_width(Hud *h, XftFont *font, const char *s) {
    XGlyphInfo ext;
    XftTextExtentsUtf8(h->d,font,(const FcChar8*)s,strlen(s),&ext);
    return ext.xOff;
}
// Keep one reversible progress value. Native compositing blends the whole
// capsule, including its text and gear, while the window slides under the bar.
static void peek_frame(Hud *h) {
    double t=h->peek_progress;
    double eased=t*t*(3-2*t);
    int delta=lround(24*(1-eased));
    int dx=h->edge==1?delta:h->edge==3?-delta:0;
    int dy=h->edge==2?delta:h->edge==0?-delta:0;
    unsigned long alpha=lround(0xffffffffUL*eased);
    XChangeProperty(h->d,h->tip,h->opacity,XA_CARDINAL,32,PropModeReplace,(unsigned char*)&alpha,1);
    XMoveWindow(h->d,h->tip,h->tip_x+dx,h->tip_y+dy);
}
static void peek_goal(Hud *h,int shown,long long now) {
    if(h->hover<0 || h->peek_target==shown)return;
    h->peek_target=shown;
    if(!h->motion){
        if(!shown)hide_tip(h);
        else {h->peek_progress=1;peek_frame(h);}
        return;
    }
    h->peek_at=now;
}
static void peek_tick(Hud *h,long long now){
    if(!h->peek_at)return;
    int delta=now-h->peek_at;
    if(delta<=0)return;
    h->peek_at=now;
    if(h->peek_target){
        h->peek_progress+=delta/300.0;
        if(h->peek_progress>=1){h->peek_progress=1;h->peek_at=0;}
    }else{
        h->peek_progress-=delta/235.0;
        if(h->peek_progress<=0){hide_tip(h);return;}
    }
    peek_frame(h);
}
static void draw_tip(Hud *h) {
    if(h->hover<0)return;
    int n=h->counts[h->hover];
    PeekCell *cells=h->cells[h->hover];
    int natural=2*peek_pad+gear;
    for(int i=0;i<n;i++)
        natural+=text_width(h,h->font,cells[i].label)+peek_gap+
            text_width(h,h->heading,cells[i].value)+(i?peek_spacing:0);
    int available=vertical(h)?h->sw-h->w-3:h->sw;
    int width=clamp(natural,gear+1,available);
    int height=peek_height;
    int x=h->x+h->hover*step(h)+(step(h)-width)/2,y=h->y+h->height;
    if(h->edge==1){x=h->x-width;y=h->y+pad+h->hover*item+(item-height)/2;}
    if(h->edge==2)y=h->y-height;
    if(h->edge==3){x=h->x+h->w;y=h->y+pad+h->hover*item+(item-height)/2;}
    x=clamp(x,h->sx,h->sx+h->sw-width);
    y=clamp(y,h->sy,h->sy+h->sh-height);
    if(h->tip_w!=width||h->tip_h!=height){
        XResizeWindow(h->d,h->tip,width,height);
        shape(h,h->tip,width,height,peek_height/2,NULL);
    }
    h->tip_x=x;h->tip_y=y;h->tip_w=width;h->tip_h=height;
    peek_frame(h);
    XMapWindow(h->d,h->tip);
    XWindowChanges z={.sibling=h->bar,.stack_mode=Below};
    XConfigureWindow(h->d,h->tip,CWSibling|CWStackMode,&z);
    XClearArea(h->d,h->tip,0,0,0,0,False);
        int tx=peek_pad;
        int baseline=(height+h->font->ascent-h->font->descent)/2;
        XRectangle clip={0,0,(unsigned short)(width-gear),height};
        XftDrawSetClipRectangles(h->tip_draw,0,0,&clip,1);
        for(int i=0;i<n;i++){
            PeekCell *c=&cells[i];
            label(h,tx,baseline,c->label,h->font,&h->muted_text);
            tx+=text_width(h,h->font,c->label)+peek_gap;
            label(h,tx,baseline,c->value,h->heading,&h->tones[c->tone]);
            tx+=text_width(h,h->heading,c->value)+peek_spacing;
        }
        XftDrawSetClip(h->tip_draw,NULL);
        paint_gear(h,width-gear/2-3,height/2);
}
static int slot_at(Hud *h,int x,int y){
    int p=(vertical(h)?y:x)-pad;
    int i=p/step(h);
    return p<0||i>=slots?-1:i;
}
static void pick(Hud *h, int x, int y) {
    int i=slot_at(h,x,y);
    if(i<0){hide_tip(h);return;}
    h->leave_at=0;

    if(i!=h->hover){
        if(h->hover<0)h->peek_progress=h->motion?0:1;
        h->hover=i;
        peek_goal(h,1,millis());
        draw_tip(h);
    }else peek_goal(h,1,millis());
}
static void drag(Hud *h,int x,int y) {
    h->moved=1;h->outlined=1;
    XMoveWindow(h->d,h->bar,h->grab_x+x-h->down_x,h->grab_y+y-h->down_y);
    draw(h);
}
static void drop(Hud *h, int x, int y) {
    int center_x=h->grab_x+x-h->down_x+h->w/2-h->sx;
    int center_y=h->grab_y+y-h->down_y+h->height/2-h->sy;
    x=clamp(x,h->sx,h->sx+h->sw-1)-h->sx;
    y=clamp(y,h->sy,h->sy+h->sh-1)-h->sy;
    int distances[] = {y, h->sw-1-x, h->sh-1-y, x};
    for(int i=0;i<4;i++)if(distances[i]<distances[h->edge])h->edge=i;
    int run=vertical(h)?2*pad+slots*item:2*pad+slots*step(h);
    int travel=vertical(h)?h->sh-run:h->sw-run;
    int offset=(vertical(h)?center_y:center_x)-run/2;
    h->offset=travel>0?(double)clamp(offset,0,travel)/travel:.5;
    place(h);
}

enum { prefs_w=760, prefs_h=452, sidebar_w=176, prefs_top=107, prefs_row=49 };
static void prefs_box(Hud *h,int x,int y,int w,int height,unsigned long fill) {
    XSetForeground(h->d,h->gc,fill);
    XFillRectangle(h->d,h->prefs,h->gc,x,y,w,height);
}
static void prefs_text(Hud *h,int x,int y,const char *s,XftFont *font,XftColor *ink) {
    XftDrawStringUtf8(h->prefs_draw,ink,font,x,y,(const FcChar8*)s,strlen(s));
}
static void draw_prefs(Hud *h) {
    if(!h->prefs_visible)return;
    prefs_box(h,0,0,prefs_w,prefs_h,h->card_bg);
    prefs_box(h,0,0,sidebar_w,prefs_h,h->side_bg);
    prefs_text(h,20,37,"Balance HUD",h->heading,&h->text_color);
    for(int i=0;i<2;i++){
        if(i==h->page)prefs_box(h,12,68+i*44,sidebar_w-24,38,h->selected_bg);
        prefs_text(h,23,93+i*44,i?"Accounts":"Display",h->prefs_font,&h->text_color);
    }
    int left=sidebar_w+28;
    prefs_text(h,left,47,h->page?"Quota accounts":"Display",h->heading,&h->text_color);
    prefs_text(h,left,77,h->page?"Choose which subscriptions the float follows.":"Choose how the float sits on your desktop.",h->prefs_font,&h->muted_text);
    for(int i=0;i<h->prefs_count;i++){
        int y=prefs_top+i*prefs_row;
        prefs_box(h,left,y,prefs_w-left-28,prefs_row-4,h->side_bg);
        PeekCell *row=&h->prefs_rows[i];
        prefs_text(h,left+12,y+28,row->label,h->prefs_font,&h->text_color);
        int length=text_width(h,h->prefs_font,row->value);
        int right=prefs_w-42;
        XRectangle clip={left+294,y,right-(left+294),prefs_row-4};
        XftDrawSetClipRectangles(h->prefs_draw,0,0,&clip,1);
        prefs_text(h,right-length,y+28,row->value,h->prefs_font,&h->tones[1]);
        XftDrawSetClip(h->prefs_draw,NULL);
    }
    prefs_text(h,left,prefs_h-35,h->prefs_help,h->prefs_font,&h->muted_text);
}
void hud_settings_row(Hud *h,int i,const char *label,const char *value) {
    if(i<0||i>=8)return;
    snprintf(h->prefs_rows[i].label,sizeof(h->prefs_rows[i].label),"%s",label);
    snprintf(h->prefs_rows[i].value,sizeof(h->prefs_rows[i].value),"%s",value);
}
void hud_settings_sync(Hud *h,int page,int n,const char *help) {
    h->page=page;h->prefs_count=clamp(n,0,8);
    snprintf(h->prefs_help,sizeof(h->prefs_help),"%s",help);
    draw_prefs(h); XFlush(h->d);
}
void hud_settings_show(Hud *h) {
    hide_tip(h);
    if(!h->prefs_visible)XMoveWindow(h->d,h->prefs,h->sx+(h->sw-prefs_w)/2,h->sy+(h->sh-prefs_h)/2);
    h->prefs_visible=1;
    XMapRaised(h->d,h->prefs);
    draw_prefs(h); XFlush(h->d);
}
void hud_appearance(Hud *h,int edge,double offset,int numbers,int motion) {
    h->edge=edge;h->offset=offset;h->numbers=numbers;h->motion=motion;
    hide_tip(h);place(h);XFlush(h->d);
}

enum { notice_w=320, notice_h=87 };
static void draw_notice(Hud *h) {
    if(!h->notice_visible)return;
    XClearWindow(h->d,h->notice);
    XftDrawStringUtf8(h->notice_draw,&h->tones[1],h->prefs_font,20,30,(const FcChar8*)"Codex",5);
    XftDrawStringUtf8(h->notice_draw,&h->text_color,h->notice_font,20,61,
                        (const FcChar8*)h->notice_body,strlen(h->notice_body));
}
void hud_notice(Hud *h,const char *body) {
    snprintf(h->notice_body,sizeof(h->notice_body),"%s",body);
    h->notice_visible=1;h->notice_until=millis()+8000;h->notice_click_at=0;
    int x=h->edge==1?h->sx+16:h->sx+h->sw-notice_w-16;
    int y=h->edge==2?h->sy+16:h->sy+h->sh-notice_h-16;
    XMoveResizeWindow(h->d,h->notice,x,y,notice_w,notice_h);
    shape(h,h->notice,notice_w,notice_h,17,NULL);
    XMapRaised(h->d,h->notice);draw_notice(h);XFlush(h->d);
}
int hud_notice_active(Hud *h) {return h->notice_visible;}
static void hide_notice(Hud *h){
    h->notice_visible=0;h->notice_click_at=0;
    XUnmapWindow(h->d,h->notice);
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
    h->bg=color(h,"#050709",BlackPixel(d,DefaultScreen(d)));
    h->fg=color(h,"#e7eef5",WhitePixel(d,DefaultScreen(d)));
    h->track=color(h,"#1d2832",h->fg);
    XSetWindowAttributes a={.override_redirect=True,.background_pixel=h->bg};
    h->bar=XCreateWindow(d,h->root,0,0,1,1,0,CopyFromParent,InputOutput,
        CopyFromParent,CWOverrideRedirect|CWBackPixel,&a);
    h->tip=XCreateWindow(d,h->root,0,0,1,1,0,CopyFromParent,InputOutput,
        CopyFromParent,CWOverrideRedirect|CWBackPixel,&a);
    h->notice=XCreateWindow(d,h->root,0,0,1,1,0,CopyFromParent,InputOutput,
        CopyFromParent,CWOverrideRedirect|CWBackPixel,&a);
    XStoreName(d,h->notice,"Balance HUD notification");
    XSelectInput(d,h->notice,ExposureMask|ButtonPressMask);
    XStoreName(d,h->bar,"Balance HUD");
    XStoreName(d,h->tip,"Balance HUD quota details");
    XSelectInput(d,h->bar,ExposureMask|ButtonPressMask|ButtonReleaseMask|
                          PointerMotionMask|LeaveWindowMask);
    XSelectInput(d,h->tip,ExposureMask|EnterWindowMask|LeaveWindowMask|ButtonPressMask);
    h->gc=XCreateGC(d,h->bar,0,NULL);
    int screen=DefaultScreen(d);
    Visual *visual=DefaultVisual(d,screen);
    Colormap cmap=DefaultColormap(d,screen);
    h->font=XftFontOpenName(d,screen,"sans:pixelsize=21");
    h->notice_font=XftFontOpenName(d,screen,"sans:lang=zh-cn:pixelsize=19");
    h->notice_draw=XftDrawCreate(d,h->notice,visual,cmap);
    h->prefs_font=XftFontOpenName(d,screen,"sans:pixelsize=16");
    h->heading=XftFontOpenName(d,screen,"sans:bold:pixelsize=21");
    h->tip_draw=XftDrawCreate(d,h->tip,visual,cmap);
    h->card_bg=color(h,"#242629",h->bg);h->side_bg=color(h,"#1a1c1f",h->bg);
    h->selected_bg=color(h,"#225da3",h->bg);
    h->prefs=XCreateSimpleWindow(d,h->root,0,0,prefs_w,prefs_h,0,h->bg,h->card_bg);
    XStoreName(d,h->prefs,"Balance HUD Settings");
    XSizeHints hints={.flags=PMinSize|PMaxSize,.min_width=prefs_w,.min_height=prefs_h,.max_width=prefs_w,.max_height=prefs_h};
    XSetWMNormalHints(d,h->prefs,&hints);
    h->wm_delete=XInternAtom(d,"WM_DELETE_WINDOW",False);
    h->opacity=XInternAtom(d,"_NET_WM_WINDOW_OPACITY",False);
    XSetWMProtocols(d,h->prefs,&h->wm_delete,1);
    XSelectInput(d,h->prefs,ExposureMask|ButtonPressMask|KeyPressMask|StructureNotifyMask);
    h->prefs_draw=XftDrawCreate(d,h->prefs,visual,cmap);
    if(!h->font||!h->heading||!h->prefs_font||!h->tip_draw||!h->prefs_draw||!h->notice_draw||!h->notice_font){hud_close(h);return NULL;}
    XftColorAllocName(d,visual,cmap,"#f2f4f6",&h->text_color);
    XftColorAllocName(d,visual,cmap,"#878787",&h->muted_text);
    const char *ink[]={"#f2f4f6","#2dd980","#ffc55e","#ff6878"};
    for(int i=0;i<4;i++) XftColorAllocName(d,visual,cmap,ink[i],&h->tones[i]);
    place(h);
    XMapRaised(d,h->bar);
    XFlush(d);
    return h;
}
void hud_close(Hud *h) {
    if(h->bitmap)XFreePixmap(h->d,h->bitmap);
    if(h->notice_draw)XftDrawDestroy(h->notice_draw);
    if(h->notice_font)XftFontClose(h->d,h->notice_font);
    if(h->tip_draw)XftDrawDestroy(h->tip_draw);
    if(h->prefs_draw)XftDrawDestroy(h->prefs_draw);
    if(h->font)XftFontClose(h->d,h->font);
    if(h->prefs_font)XftFontClose(h->d,h->prefs_font);
    if(h->heading)XftFontClose(h->d,h->heading);
    XFreeGC(h->d,h->gc);
    XCloseDisplay(h->d);
    free(h);
}
void hud_cell(Hud *h,int slot,int index,const char *label,const char *value,int tone) {
    if(slot<0||slot>=slots||index<0||index>=max_cells)return;
    PeekCell *c=&h->cells[slot][index];
    snprintf(c->label,sizeof(c->label),"%s",label);
    snprintf(c->value,sizeof(c->value),"%s",value);
    c->tone=clamp(tone,0,3);
}
void hud_peek(Hud *h,int slot,int n) {
    h->counts[slot]=clamp(n,0,max_cells);
    if(h->hover==slot)draw_tip(h);
}
int hud_step(Hud *h,int timeout) {
    if(h->peek_at&&timeout>16)timeout=16;
    if(h->leave_at){
        int wait=clamp(h->leave_at-millis(),0,timeout);
        if(wait<timeout)timeout=wait;
    }
    if(!XPending(h->d)){
        struct pollfd fd={.fd=ConnectionNumber(h->d),.events=POLLIN};
        poll(&fd,1,timeout);
    }
    while(XPending(h->d)){
        XEvent e;
        XNextEvent(h->d,&e);
        if(e.xany.window==h->notice){
            if(e.type==Expose&&e.xexpose.count==0)draw_notice(h);
            if(e.type==ButtonPress&&h->notice_visible){
                if(e.xbutton.button==Button1){
                    long long now=millis();
                    if(h->notice_click_at&&now-h->notice_click_at<=450){hide_notice(h);return 90;}
                    h->notice_click_at=now;
                } else if(e.xbutton.button==Button3)hide_notice(h);
            }
            continue;
        }
        if(e.xany.window==h->prefs){
            if(e.type==Expose&&e.xexpose.count==0)draw_prefs(h);
            if((e.type==ClientMessage&&(Atom)e.xclient.data.l[0]==h->wm_delete)||
               (e.type==KeyPress&&XLookupKeysym(&e.xkey,0)==XK_Escape)){
                h->prefs_visible=0;XUnmapWindow(h->d,h->prefs);
            }
            if(e.type==ButtonPress&&e.xbutton.button==Button1){
                int x=e.xbutton.x,y=e.xbutton.y;
                if(x<sidebar_w&&y>=68&&y<156)return 10+(y-68)/44;
                if(x>=sidebar_w+28&&y>=prefs_top&&y<prefs_top+h->prefs_count*prefs_row)
                    return 20+(y-prefs_top)/prefs_row;
            }
            continue;
        }
        switch(e.type){
        case Expose:
            // Expose can be queued by mapping the tooltip. Clear/draw without remapping.
            if(e.xexpose.count==0 && e.xexpose.window==h->bar)draw(h);
            if(e.xexpose.count==0 && e.xexpose.window==h->tip)draw_tip(h);
            break;
        case LeaveNotify:
            if(h->hover>=0)h->leave_at=millis()+(h->motion?120:0);
            break;
        case EnterNotify:h->leave_at=0;peek_goal(h,1,millis());break;
        case MotionNotify:
            if(h->pressed){
                if(h->outlined||hypot(e.xmotion.x_root-h->down_x,e.xmotion.y_root-h->down_y)>=6)
                    drag(h,e.xmotion.x_root,e.xmotion.y_root);
            }else if(e.xmotion.window==h->bar)pick(h,e.xmotion.x,e.xmotion.y);
            break;
        case ButtonPress:
            if(e.xbutton.window==h->tip){
                if(e.xbutton.button==Button3){hide_tip(h);break;}
                if(e.xbutton.button!=Button1)break;
                if(e.xbutton.x>=h->tip_w-gear)return 4;
                break;
            }
            if(e.xbutton.button==Button1){
                hide_tip(h);
                h->pressed=1;h->moved=0;
                h->down_x=e.xbutton.x_root;h->down_y=e.xbutton.y_root;
                h->grab_x=h->x;h->grab_y=h->y;h->down_at=millis();
            }else if(e.xbutton.button==Button3){
                if(e.xbutton.state&ShiftMask)return 1;
                hide_tip(h);h->numbers=!h->numbers;place(h);XFlush(h->d);return 2;
            }else if(e.xbutton.button==Button2)return 3;
            break;
        case ButtonRelease:
            if(e.xbutton.button==Button1&&h->pressed){
                h->pressed=0;h->outlined=0;
                if(h->moved){
                    h->click_at=0;
                    drop(h,e.xbutton.x_root,e.xbutton.y_root);
                    XFlush(h->d);return 2;
                }
                draw(h);
                int i=slot_at(h,e.xbutton.x,e.xbutton.y);
                long long now=millis();
                if(i>=0&&i==h->click_slot&&h->click_at&&now-h->click_at<=450){
                    h->click_at=0;hide_tip(h);return 100+i;
                }
                h->click_slot=i;h->click_at=now;
                pick(h,e.xbutton.x,e.xbutton.y);
            }
            break;
        }
    }
    long long now=millis();
    if(h->leave_at&&now>=h->leave_at){h->leave_at=0;peek_goal(h,0,now);}
    peek_tick(h,now);
    if(h->notice_visible&&now>=h->notice_until)hide_notice(h);
    if(h->pressed&&!h->outlined&&millis()-h->down_at>=300){
        h->outlined=1;draw(h);
    }
    XFlush(h->d);
    return 0;
}
int hud_edge(Hud *h){return h->edge;}
double hud_offset(Hud *h){return h->offset;}
int hud_numbers(Hud *h){return h->numbers;}

int hud_hover(Hud *h){return h->hover;}
