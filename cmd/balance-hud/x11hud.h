typedef struct Hud Hud;
Hud *hud_open(int edge, double offset, int numbers,int motion);
void hud_close(Hud *h);
void hud_cell(Hud *,int,int,const char *,const char *,int);
void hud_peek(Hud *,int,int);
int hud_step(Hud *h, int timeout);
int hud_edge(Hud *h);
double hud_offset(Hud *h);
int hud_numbers(Hud *h);

void hud_number(Hud *,int,const char *,int);
void hud_bitmap(Hud *,int,int,const unsigned char *);
int hud_hover(Hud *);

void hud_settings_row(Hud *,int,const char *,const char *);
void hud_settings_sync(Hud *,int,int,const char *);
void hud_settings_show(Hud *);
void hud_appearance(Hud *,int,double,int,int);

int hud_post_completion(int,unsigned int);
unsigned int hud_hook_session(Hud *);

void hud_slots(Hud *,int);

void hud_mail_bounds(Hud *,int *);
void hud_mail_row(Hud *,int,const char *,const char *);
void hud_mail_sync(Hud *,int,int,int,int,int,int,int,int);
void hud_mail_flight(Hud *,int,int,int,const int *);
int hud_drag_active(Hud *);
