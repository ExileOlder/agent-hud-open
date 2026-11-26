typedef struct Hud Hud;
Hud *hud_open(int edge, double offset, int numbers,int motion);
void hud_close(Hud *h);
void hud_row(Hud *h, int slot, const char *mark, const char *number, const char *details, int percent, int stale);
int hud_step(Hud *h, int timeout);
int hud_edge(Hud *h);
double hud_offset(Hud *h);
int hud_numbers(Hud *h);

void hud_bitmap(Hud *,int,int,const unsigned char *);
int hud_hover(Hud *);
int hud_motion(Hud *);
