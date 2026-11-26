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

enum { slots = 5, item = 92, wide = 148, pad = 4, peek_height = 64, peek_pad = 30, peek_gap = 12, peek_spacing = 30, gear = 45, max_cells = 8 };
typedef struct { char label[96], value[192]; int tone; } PeekCell;
struct Hud {
    Display *d;
    Window root, bar, tip, prefs, notice, mail_panel, paper;
    Atom wm_delete, opacity, completion;
    unsigned int hook_session;
    GC gc;
    XftFont *font, *heading, *prefs_font, *notice_font, *mail_small, *mail_title, *mail_header_font;
    XftDraw *tip_draw, *prefs_draw, *notice_draw, *mail_draw, *bar_draw;
    XftColor text_color, muted_text, tones[4], number_ink[5];
    char figures[slots][16];
    int figure_tone[slots];
    Pixmap bitmap,tip_bitmap;
    int tip_bitmap_width;
    int bw, bh, visible_slots;
    int tip_x, tip_y, tip_w, tip_h, motion;
    int tip_from_x,tip_from_y,tip_from_w,tip_draw_w,tip_mapped;
    long long tip_motion_at;
    int prefs_visible, page, prefs_count;
    PeekCell prefs_rows[8];
    char prefs_help[192];
    unsigned long card_bg, side_bg, selected_bg;
    long long leave_at, down_at, peek_at;
    double peek_progress;
    int peek_target;
    long long click_at, mail_click_at, mail_leave_at;
    int click_slot, mail_count, mail_open, mail_row_click, mail_dirty, paper_shown;
    int mail_rect[4], mail_panel_rect[4];
    int paper_xy[12];
    PeekCell mail_rows[6];
    unsigned long mail_blue,mail_ring;
    unsigned long mail_fill,mail_row_bg,mail_hot_bg,mail_button,mail_rule;
    unsigned long mail_tile[3],mail_accent[3];
    XftColor mail_status[3];
    int mail_hot,mail_displayed;
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
    XFillRectangle(h->d,h->tip_bitmap,h->gc,x,y,w,height);
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
    XFillPolygon(h->d,h->tip_bitmap,h->gc,teeth,32,Nonconvex,CoordModeOrigin);
    XSetForeground(h->d,h->gc,h->bg);
    XFillArc(h->d,h->tip_bitmap,h->gc,x-4,y-4,8,8,0,360*64);
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
void hud_number(Hud *h,int i,const char *number,int tone){
    if(i<0||i>=slots)return;
    snprintf(h->figures[i],sizeof(h->figures[i]),"%s",number);
    h->figure_tone[i]=clamp(tone,0,4);
}
void hud_bitmap(Hud *h,int w,int height,const unsigned char *pixels){
    if(w!=h->w||height!=h->height)return;
    if(!h->bitmap||h->bw!=w||h->bh!=height){
        if(h->bitmap)XFreePixmap(h->d,h->bitmap);
        h->bitmap=XCreatePixmap(h->d,h->bar,w,height,DefaultDepth(h->d,DefaultScreen(h->d)));
        h->bw=w;h->bh=height;
        XftDrawChange(h->bar_draw,h->bitmap);
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
    if(h->numbers)for(int i=0;i<h->visible_slots;i++){
        int x=(vertical(h)?0:pad+i*step(h))+item-2;
        int y=pad+(vertical(h)?i*item:0)+item/2+
            (h->prefs_font->ascent-h->prefs_font->descent)/2;
        XftDrawStringUtf8(h->bar_draw,&h->number_ink[h->figure_tone[i]],h->prefs_font,
            x,y,(const FcChar8*)h->figures[i],strlen(h->figures[i]));
    }
    draw(h);XFlush(h->d);
}
static void place(Hud *h) {
    int v = vertical(h);
    h->w = v ? (h->numbers ? wide : item) : 2 * pad + h->visible_slots * step(h);
    h->height = 2 * pad + (v ? h->visible_slots * item : item);
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
    h->tip_mapped=0;h->tip_motion_at=0;
    XUnmapWindow(h->d,h->tip);
}
static int text_width(Hud *h, XftFont *font, const char *s) {
    XGlyphInfo ext;
    XftTextExtentsUtf8(h->d,font,(const FcChar8*)s,strlen(s),&ext);
    return ext.xOff;
}
// HUDExpandedSurface.tsx keeps the Simple peek mounted and animates its
// left/top/width with HUD_METRICS.ease for 0.4 s (hud-metrics.ts).
static double peek_switch_ease(double elapsed){
    double t=elapsed/400.0;
    if(t<=0)return 0;
    if(t>=1)return 1;
    double lo=0,hi=1;
    for(int i=0;i<12;i++){
        double m=(lo+hi)*.5,u=1-m;
        double x=3*u*u*m*.2+3*u*m*m*.2+m*m*m;
        if(x<t)lo=m;else hi=m;
    }
    double m=(lo+hi)*.5,u=1-m;
    return 3*u*u*m*.8+3*u*m*m+m*m*m;
}
static void tip_geometry(Hud *h,long long now,int *x,int *y,int *width) {
    double t=h->tip_motion_at?peek_switch_ease(now-h->tip_motion_at):1;
    *x=lround(h->tip_from_x+(h->tip_x-h->tip_from_x)*t);
    *y=lround(h->tip_from_y+(h->tip_y-h->tip_from_y)*t);
    *width=lround(h->tip_from_w+(h->tip_w-h->tip_from_w)*t);
    if(t>=1)h->tip_motion_at=0;
}
static void paint_peek(Hud *h);
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
    int x,y,w;
    tip_geometry(h,millis(),&x,&y,&w);
    XMoveResizeWindow(h->d,h->tip,x+dx,y+dy,w,h->tip_h);
    if(h->tip_draw_w!=w){
        h->tip_draw_w=w;
        shape(h,h->tip,w,h->tip_h,peek_height/2,NULL);
        paint_peek(h);
    }
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
    if(!h->peek_at){
        if(h->tip_motion_at)peek_frame(h);
        return;
    }
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
    if(h->tip_x!=x||h->tip_y!=y||h->tip_w!=width||!h->tip_mapped){
        long long now=millis();
        if(h->tip_mapped&&h->motion){
            tip_geometry(h,now,&h->tip_from_x,&h->tip_from_y,&h->tip_from_w);
            h->tip_motion_at=now;
        }else{
            h->tip_from_x=x;h->tip_from_y=y;h->tip_from_w=width;h->tip_motion_at=0;
        }
        h->tip_x=x;h->tip_y=y;h->tip_w=width;h->tip_h=height;
    }
    peek_frame(h);
    if(!h->tip_mapped){
        h->tip_mapped=1;
        XMapWindow(h->d,h->tip);
        XWindowChanges z={.sibling=h->bar,.stack_mode=Below};
        XConfigureWindow(h->d,h->tip,CWSibling|CWStackMode,&z);
    }
    paint_peek(h);
}
static void paint_peek(Hud *h) {
    if(h->hover<0)return;
    int width=h->tip_draw_w,height=h->tip_h;
    if(width<=0||height<=0)return;
    int n=h->counts[h->hover];
    PeekCell *cells=h->cells[h->hover];
    if(!h->tip_bitmap || h->tip_bitmap_width<width){
        if(h->tip_bitmap)XFreePixmap(h->d,h->tip_bitmap);
        h->tip_bitmap_width=width;
        h->tip_bitmap=XCreatePixmap(h->d,h->tip,width,height,DefaultDepth(h->d,DefaultScreen(h->d)));
        XftDrawChange(h->tip_draw,h->tip_bitmap);
    }
    box(h,0,0,width,height,h->bg);
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
    XCopyArea(h->d,h->tip_bitmap,h->tip,h->gc,0,0,width,height,0,0);
}
static int slot_at(Hud *h,int x,int y){
    int p=(vertical(h)?y:x)-pad;
    int i=p/step(h);
    return p<0||i>=h->visible_slots?-1:i;
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
    int run=vertical(h)?2*pad+h->visible_slots*item:2*pad+h->visible_slots*step(h);
    int travel=vertical(h)?h->sh-run:h->sw-run;
    int offset=(vertical(h)?center_y:center_x)-run/2;
    h->offset=travel>0?(double)clamp(offset,0,travel)/travel:.5;
    place(h);
}

enum { prefs_w=760, prefs_h=535, sidebar_w=176, prefs_top=107, prefs_row=49 };
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
void hud_slots(Hud *h,int visible) {
    visible=clamp(visible,4,slots);
    if(h->visible_slots==visible)return;
    h->visible_slots=visible;hide_tip(h);place(h);XFlush(h->d);
}

void hud_appearance(Hud *h,int edge,double offset,int numbers,int motion) {
    h->edge=edge;h->offset=offset;h->numbers=numbers;h->motion=motion;
    hide_tip(h);place(h);XFlush(h->d);
}


enum { mail_size=84, mail_header=72, mail_row=120, paper_size=44 };
int hud_drag_active(Hud *h) {return h->pressed&&h->moved;}
void hud_mail_bounds(Hud *h,int *rects) {
    int values[]={h->x,h->y,h->w,h->height,h->sx,h->sy,h->sw,h->sh};
    memcpy(rects,values,sizeof(values));
}
static void mail_text(XftDraw *draw,XftColor *ink,XftFont *font,int x,int y,const char *text){
    XftDrawStringUtf8(draw,ink,font,x,y,(const FcChar8*)text,strlen(text));
}
static void draw_badge(Hud *h) {
    if(h->mail_count<1)return;
    XClearWindow(h->d,h->notice);
    XSetForeground(h->d,h->gc,h->mail_ring);
    XDrawArc(h->d,h->notice,h->gc,1,1,mail_size-3,mail_size-3,0,360*64);
    XSetForeground(h->d,h->gc,h->mail_blue);
    XPoint icon[]={{26,57},{26,31},{56,31},{56,51},{34,51},{26,57}};
    XSetLineAttributes(h->d,h->gc,2,LineSolid,CapRound,JoinRound);
    XDrawLines(h->d,h->notice,h->gc,icon,6,CoordModeOrigin);
    for(int y=36;y<=45;y+=4)XDrawLine(h->d,h->notice,h->gc,33,y,49,y);
    XSetLineAttributes(h->d,h->gc,0,LineSolid,CapButt,JoinMiter);
    XSetForeground(h->d,h->gc,h->side_bg);
    XFillArc(h->d,h->notice,h->gc,52,4,30,30,0,360*64);
    char count[8];snprintf(count,sizeof(count),"%d",h->mail_count);
    mail_text(h->notice_draw,&h->text_color,h->prefs_font,67-text_width(h,h->prefs_font,count)/2,24,count);
}
static void mail_rounded(Hud*h,int x,int y,int w,int height,int r,unsigned long color) {
    if(w<=2*r||height<=2*r)return;
    Window dst=h->mail_panel;
    XSetForeground(h->d,h->gc,color);
    XFillRectangle(h->d,dst,h->gc,x+r,y,w-2*r,height);
    XFillRectangle(h->d,dst,h->gc,x,y+r,w,height-2*r);
    XFillArc(h->d,dst,h->gc,x,y,2*r,2*r,0,360*64);
    XFillArc(h->d,dst,h->gc,x+w-2*r,y,2*r,2*r,0,360*64);
    XFillArc(h->d,dst,h->gc,x,y+height-2*r,2*r,2*r,0,360*64);
    XFillArc(h->d,dst,h->gc,x+w-2*r,y+height-2*r,2*r,2*r,0,360*64);
}
static int mail_target(Hud*h,int x,int y) {
    int width=h->mail_panel_rect[2];
    if(x>=width-198&&x<width-22&&y>=14&&y<59)return 92;
    if(x<16||x>=width-16||y<mail_header)return 0;
    int row=(y-mail_header)/mail_row,inside=(y-mail_header)%mail_row;
    if(row>=h->mail_displayed||inside>=mail_row-12)return 0;
    if(x>=width-88&&x<width-29&&inside>=26&&inside<84)return 320+row;
    return 300+row;
}
static void draw_mail_panel(Hud *h) {
    if(!h->mail_open)return;
    int width=h->mail_panel_rect[2],height=h->mail_panel_rect[3];
    XSetForeground(h->d,h->gc,h->mail_fill);
    XFillRectangle(h->d,h->mail_panel,h->gc,0,0,width,height);
    mail_text(h->mail_draw,&h->text_color,h->mail_header_font,27,48,"消息");
    char count_text[8];snprintf(count_text,sizeof(count_text),"%d",h->mail_count);
    mail_rounded(h,121,17,55,40,11,h->mail_button);
    mail_text(h->mail_draw,&h->text_color,h->mail_small,148-text_width(h,h->mail_small,count_text)/2,45,count_text);
    mail_rounded(h,width-198,14,176,45,11,h->mail_hot==92?h->mail_hot_bg:h->mail_row_bg);
    mail_text(h->mail_draw,&h->text_color,h->mail_small,width-110-text_width(h,h->mail_small,"全部清空")/2,45,"全部清空");
    XSetForeground(h->d,h->gc,h->mail_rule);
    XDrawLine(h->d,h->mail_panel,h->gc,22,66,width-23,66);
    for(int i=0;i<h->mail_displayed;i++){
        int y=mail_header+i*mail_row;
        PeekCell*row=&h->mail_rows[i];
        int tone=clamp(row->tone,0,2);
        int hover=h->mail_hot==300+i||h->mail_hot==320+i;
        mail_rounded(h,16,y,width-32,mail_row-12,15,hover?h->mail_hot_bg:h->mail_row_bg);
        mail_rounded(h,30,y+28,52,52,13,h->mail_tile[tone]);
        XSetForeground(h->d,h->gc,h->mail_accent[tone]);
        XSetLineAttributes(h->d,h->gc,3,LineSolid,CapRound,JoinRound);
        if(!tone){
            XPoint mark[]={{46,y+55},{53,y+62},{67,y+45}};
            XDrawLines(h->d,h->mail_panel,h->gc,mark,3,CoordModeOrigin);
        } else if(tone==2){
            XDrawLine(h->d,h->mail_panel,h->gc,56,y+42,56,y+55);
            XDrawPoint(h->d,h->mail_panel,h->gc,56,y+62);
        }
        XSetLineAttributes(h->d,h->gc,0,LineSolid,CapButt,JoinMiter);
        if(tone==1)mail_text(h->mail_draw,&h->mail_status[tone],h->mail_small,56-text_width(h,h->mail_small,"?")/2,y+62,"?");
        mail_text(h->mail_draw,&h->text_color,h->mail_title,106,y+39,row->label);
        mail_text(h->mail_draw,&h->mail_status[tone],h->mail_small,106,y+79,row->value);
        int close_x=width-58,close_y=y+55;
        if(h->mail_hot==320+i)mail_rounded(h,width-88,y+26,59,58,12,h->mail_button);
        XSetForeground(h->d,h->gc,h->mail_hot==320+i?h->fg:h->mail_ring);
        XSetLineAttributes(h->d,h->gc,3,LineSolid,CapRound,JoinRound);
        XDrawLine(h->d,h->mail_panel,h->gc,close_x-7,close_y-7,close_x+7,close_y+7);
        XDrawLine(h->d,h->mail_panel,h->gc,close_x-7,close_y+7,close_x+7,close_y-7);
        XSetLineAttributes(h->d,h->gc,0,LineSolid,CapButt,JoinMiter);
    }
}
void hud_mail_row(Hud *h,int i,const char *title,const char *body,int tone){
    if(i<0||i>=6)return;
    h->mail_dirty=1;
    h->mail_rows[i].tone=tone;
    snprintf(h->mail_rows[i].label,sizeof(h->mail_rows[i].label),"%s",title);
    snprintf(h->mail_rows[i].value,sizeof(h->mail_rows[i].value),"%s",body);
}
void hud_mail_sync(Hud *h,int count,int open,int x,int y,int px,int py,int pw,int ph,int displayed){
    h->mail_displayed=clamp(displayed,0,6);
    int previous=h->mail_count;
    h->mail_count=count;
    if(count>0){
        if(!previous||x!=h->mail_rect[0]||y!=h->mail_rect[1]){
            XMoveResizeWindow(h->d,h->notice,x,y,mail_size,mail_size);
            shape(h,h->notice,mail_size,mail_size,mail_size/2,NULL);
            if(!previous)XMapRaised(h->d,h->notice);
        }
        h->mail_rect[0]=x;h->mail_rect[1]=y;
        if(previous!=count)draw_badge(h);
    }else if(previous>0)XUnmapWindow(h->d,h->notice);
    int was=h->mail_open;
    h->mail_open=open&&count>0;
    if(h->mail_open){
        int bounds[]={px,py,pw,ph};
        if(!was||memcmp(h->mail_panel_rect,bounds,sizeof(bounds))!=0){
            memcpy(h->mail_panel_rect,bounds,sizeof(bounds));
            XMoveResizeWindow(h->d,h->mail_panel,px,py,pw,ph);
            shape(h,h->mail_panel,pw,ph,22,NULL);
        }
        if(!was)XMapRaised(h->d,h->mail_panel);
        if(!was||previous!=count||h->mail_dirty)draw_mail_panel(h);
        h->mail_dirty=0;
    }else if(was)XUnmapWindow(h->d,h->mail_panel);
}
static void draw_paper(Hud *h) {
    if(!h->paper_shown)return;
    XPoint p[7];
    for(int i=0;i<6;i++)p[i]=(XPoint){h->paper_xy[2*i],h->paper_xy[2*i+1]};
    p[6]=p[0];
    XSetForeground(h->d,h->gc,h->fg);
    XFillPolygon(h->d,h->paper,h->gc,p,6,Nonconvex,CoordModeOrigin);
    XSetForeground(h->d,h->gc,h->mail_blue);
    XDrawLines(h->d,h->paper,h->gc,p,7,CoordModeOrigin);
    XDrawLine(h->d,h->paper,h->gc,p[0].x,p[0].y,p[2].x,p[2].y);
    XDrawLine(h->d,h->paper,h->gc,p[0].x,p[0].y,p[4].x,p[4].y);
}
void hud_mail_flight(Hud *h,int shown,int x,int y,const int *points){
    if(!shown){if(h->paper_shown)XUnmapWindow(h->d,h->paper);h->paper_shown=0;return;}
    if(XShapeQueryExtension(h->d,&(int){0},&(int){0})){
        XPoint poly[6];
        for(int i=0;i<6;i++)poly[i]=(XPoint){points[2*i],points[2*i+1]};
        Region r=XPolygonRegion(poly,6,WindingRule),input=XCreateRegion();
        XShapeCombineRegion(h->d,h->paper,ShapeBounding,0,0,r,ShapeSet);
        XShapeCombineRegion(h->d,h->paper,ShapeInput,0,0,input,ShapeSet);
        XDestroyRegion(r);XDestroyRegion(input);
    }
    memcpy(h->paper_xy,points,sizeof(h->paper_xy));
    XMoveResizeWindow(h->d,h->paper,x,y,paper_size,paper_size);
    if(!h->paper_shown)XMapRaised(h->d,h->paper);
    h->paper_shown=1;
    draw_paper(h);
}

Hud *hud_open(int edge, double offset, int numbers,int motion) {
    Display *d = XOpenDisplay(NULL);
    if (!d) return NULL;
    Hud *h = calloc(1, sizeof(*h));
    if (!h) { XCloseDisplay(d); return NULL; }
    h->d=d; h->root=DefaultRootWindow(d);h->visible_slots=4;
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
    h->mail_panel=XCreateWindow(d,h->root,0,0,1,1,0,CopyFromParent,InputOutput,
        CopyFromParent,CWOverrideRedirect|CWBackPixel,&a);
    h->paper=XCreateWindow(d,h->root,0,0,1,1,0,CopyFromParent,InputOutput,
        CopyFromParent,CWOverrideRedirect|CWBackPixel,&a);
    XStoreName(d,h->notice,"Balance HUD messages");
    XStoreName(d,h->mail_panel,"Balance HUD inbox");
    XStoreName(d,h->paper,"Balance HUD paper flight");
    XSelectInput(d,h->notice,ExposureMask|ButtonPressMask|EnterWindowMask|LeaveWindowMask);
    XSelectInput(d,h->mail_panel,ExposureMask|ButtonPressMask|EnterWindowMask|LeaveWindowMask|PointerMotionMask);
    XSelectInput(d,h->paper,ExposureMask);
    XStoreName(d,h->bar,"Balance HUD");
    XStoreName(d,h->tip,"Balance HUD quota details");
    XSelectInput(d,h->bar,ExposureMask|ButtonPressMask|ButtonReleaseMask|
                          PointerMotionMask|EnterWindowMask|LeaveWindowMask);
    XSelectInput(d,h->tip,ExposureMask|EnterWindowMask|LeaveWindowMask|ButtonPressMask);
    h->gc=XCreateGC(d,h->bar,0,NULL);
    int screen=DefaultScreen(d);
    Visual *visual=DefaultVisual(d,screen);
    Colormap cmap=DefaultColormap(d,screen);
    h->font=XftFontOpenName(d,screen,"sans:pixelsize=21");
    h->notice_font=XftFontOpenName(d,screen,"sans:lang=zh-cn:pixelsize=19");
    h->mail_small=XftFontOpenName(d,screen,"sans:lang=zh-cn:pixelsize=24");
    h->mail_header_font=XftFontOpenName(d,screen,"sans:lang=zh-cn:pixelsize=32");
    h->mail_title=XftFontOpenName(d,screen,"sans:weight=demibold:pixelsize=32");
    h->mail_fill=color(h,"#101418",h->bg);
    h->mail_row_bg=color(h,"#191f25",h->bg);
    h->mail_hot_bg=color(h,"#222c36",h->bg);
    h->mail_button=color(h,"#2c3947",h->bg);
    h->mail_rule=color(h,"#26313b",h->bg);
    const char *tile[]={"#15362e","#1b314a","#3b2c24"};
    const char *accent[]={"#72dcb1","#92baff","#f0be85"};
    for(int i=0;i<3;i++){
        h->mail_tile[i]=color(h,tile[i],h->bg);
        h->mail_accent[i]=color(h,accent[i],h->fg);
        XftColorAllocName(d,visual,cmap,accent[i],&h->mail_status[i]);
    }
    h->notice_draw=XftDrawCreate(d,h->notice,visual,cmap);
    h->mail_draw=XftDrawCreate(d,h->mail_panel,visual,cmap);
    h->mail_ring=color(h,"#74787b",h->fg);
    h->mail_blue=color(h,"#7eaaff",h->fg);
    h->prefs_font=XftFontOpenName(d,screen,"sans:pixelsize=16");
    h->heading=XftFontOpenName(d,screen,"sans:weight=demibold:pixelsize=21");
    h->tip_draw=XftDrawCreate(d,h->tip,visual,cmap);
    h->bar_draw=XftDrawCreate(d,h->bar,visual,cmap);
    h->card_bg=color(h,"#242629",h->bg);h->side_bg=color(h,"#1a1c1f",h->bg);
    h->selected_bg=color(h,"#225da3",h->bg);
    h->prefs=XCreateSimpleWindow(d,h->root,0,0,prefs_w,prefs_h,0,h->bg,h->card_bg);
    XStoreName(d,h->prefs,"Balance HUD Settings");
    XSizeHints hints={.flags=PMinSize|PMaxSize,.min_width=prefs_w,.min_height=prefs_h,.max_width=prefs_w,.max_height=prefs_h};
    XSetWMNormalHints(d,h->prefs,&hints);
    h->wm_delete=XInternAtom(d,"WM_DELETE_WINDOW",False);
    h->opacity=XInternAtom(d,"_NET_WM_WINDOW_OPACITY",False);
    h->completion=XInternAtom(d,"_BALANCE_HUD_COMPLETION",False);
    XSetWMProtocols(d,h->prefs,&h->wm_delete,1);
    XSelectInput(d,h->prefs,ExposureMask|ButtonPressMask|KeyPressMask|StructureNotifyMask);
    h->prefs_draw=XftDrawCreate(d,h->prefs,visual,cmap);
    if(!h->font||!h->heading||!h->prefs_font||!h->tip_draw||!h->prefs_draw||!h->notice_draw||!h->notice_font||!h->mail_draw||!h->bar_draw||!h->mail_small||!h->mail_title||!h->mail_header_font){hud_close(h);return NULL;}
    XftColorAllocName(d,visual,cmap,"#f2f4f6",&h->text_color);
    XftColorAllocName(d,visual,cmap,"#878787",&h->muted_text);
    const char *ink[]={"#f2f4f6","#2dd980","#ffc55e","#ff6878"};
    for(int i=0;i<4;i++) XftColorAllocName(d,visual,cmap,ink[i],&h->tones[i]);
    const char *figures[]={"#a5aeb5","#87919a","#ff6878","#ffc55e","#2dd980"};
    for(int i=0;i<5;i++)XftColorAllocName(d,visual,cmap,figures[i],&h->number_ink[i]);
    place(h);
    XMapRaised(d,h->bar);
    XSetSelectionOwner(d,h->completion,h->bar,CurrentTime);
    XFlush(d);
    return h;
}
// Local X11 IPC: lifecycle code and opaque local session key.
int hud_post_completion(int code,unsigned int session) {
    Display *d=XOpenDisplay(NULL);
    if(!d)return 0;
    Atom message=XInternAtom(d,"_BALANCE_HUD_COMPLETION",True);
    Window owner=message?XGetSelectionOwner(d,message):None;
    if(owner==None){XCloseDisplay(d);return 0;}
    XEvent e={0};
    e.xclient.type=ClientMessage;
    e.xclient.window=owner;
    e.xclient.message_type=message;
    e.xclient.format=32;
    e.xclient.data.l[0]=code;
    e.xclient.data.l[1]=(long)session;
    int ok=XSendEvent(d,owner,False,NoEventMask,&e);
    XCloseDisplay(d);
    return ok!=0;
}

unsigned int hud_hook_session(Hud *h){return h->hook_session;}

void hud_close(Hud *h) {
    if(h->bar_draw)XftDrawDestroy(h->bar_draw);
    if(h->bitmap)XFreePixmap(h->d,h->bitmap);
    if(h->mail_draw)XftDrawDestroy(h->mail_draw);
    if(h->notice_draw)XftDrawDestroy(h->notice_draw);
    if(h->notice_font)XftFontClose(h->d,h->notice_font);
    if(h->mail_small)XftFontClose(h->d,h->mail_small);
    if(h->mail_title)XftFontClose(h->d,h->mail_title);
    if(h->mail_header_font)XftFontClose(h->d,h->mail_header_font);
    if(h->tip_draw)XftDrawDestroy(h->tip_draw);
    if(h->tip_bitmap)XFreePixmap(h->d,h->tip_bitmap);
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
    if((h->peek_at||h->tip_motion_at)&&timeout>16)timeout=16;
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
        if(e.xany.window==h->paper){
            if(e.type==Expose&&e.xexpose.count==0)draw_paper(h);
            continue;
        }
        if(e.xany.window==h->notice||e.xany.window==h->mail_panel){
            int badge=e.xany.window==h->notice;
            if(e.type==Expose&&e.xexpose.count==0){if(badge)draw_badge(h);else draw_mail_panel(h);}
            if(e.type==EnterNotify){
                h->mail_leave_at=0;
                if(badge&&!h->mail_open)return 90;
            }
            if(e.type==MotionNotify&&!badge){
                int hot=mail_target(h,e.xmotion.x,e.xmotion.y);
                if(hot!=h->mail_hot){h->mail_hot=hot;draw_mail_panel(h);}
            }
            if(e.type==LeaveNotify&&h->mail_open){
                h->mail_leave_at=millis()+180;
                if(h->mail_hot){h->mail_hot=0;draw_mail_panel(h);}
            }
            if(e.type==ButtonPress){
                if(!badge&&e.xbutton.button==Button4)return 93;
                if(!badge&&e.xbutton.button==Button5)return 94;
                if(e.xbutton.button==Button3)return 91;
                if(e.xbutton.button==Button1){
                    if(badge)return 90;
                    int hit=mail_target(h,e.xbutton.x,e.xbutton.y);
                    if(hit==92||hit>=320){h->mail_click_at=0;return hit;}
                    if(hit>=300){
                        long long now=millis();
                        if(h->mail_click_at&&h->mail_row_click==hit&&now-h->mail_click_at<=450){
                            h->mail_click_at=0;return hit;
                        }
                        h->mail_click_at=now;h->mail_row_click=hit;
                    }

                }
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
        if(e.type==ClientMessage && e.xclient.window==h->bar &&
           e.xclient.message_type==h->completion && e.xclient.format==32){
            long code=e.xclient.data.l[0];
            if(code>=1&&code<64){
                h->hook_session=(unsigned int)e.xclient.data.l[1];
                return 200+(int)code;
            }
            continue;
        }
        switch(e.type){
        case Expose:
            // Expose can be queued by mapping the tooltip. Clear/draw without remapping.
            if(e.xexpose.count==0 && e.xexpose.window==h->bar)draw(h);
            if(e.xexpose.count==0 && e.xexpose.window==h->tip)paint_peek(h);
            break;
        case LeaveNotify:
            if(h->hover>=0)h->leave_at=millis()+(h->motion?120:0);
            break;
        case EnterNotify:
            h->leave_at=0;
            // Re-entering from the peek may deliver EnterNotify without a
            // MotionNotify; resolve the new mark from the crossing itself.
            if(e.xcrossing.window==h->bar && !h->pressed)
                pick(h,e.xcrossing.x,e.xcrossing.y);
            else peek_goal(h,1,millis());
            break;
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
                if(e.xbutton.x>=h->tip_draw_w-gear)return 4;
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
    if(h->mail_leave_at&&now>=h->mail_leave_at){h->mail_leave_at=0;return 91;}
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
