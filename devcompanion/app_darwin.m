#import <Cocoa/Cocoa.h>
#import <stdbool.h>

extern void goOnTraySettingsClicked();
extern void goOnTrayQuitClicked();
extern void goOnTalkButtonClicked();

// ── クリック透過: main-queue タイマーによるポーリング ────────────────────────
// CSS座標(top-left) → NSWindow座標(bottom-left) 変換後に C 配列として保持し、
// 150ms ごとに hit test して setIgnoresMouseEvents を切り替える。
// ObjC オブジェクト（NSMutableArray）を static に置くと ARC の
// objc_storeStrong がブロック入れ子コンテキストで誤動作するため、
// 純粋な C 配列で管理する。

#define MAX_HIT_RECTS 16
static NSRect   g_hitRects[MAX_HIT_RECTS];
static int      g_hitRectCount    = 0;
static BOOL     g_fullInteractive = NO;
static dispatch_source_t g_hitTimer = NULL;

static NSWindow* mainWailsWindow(void) {
    for (NSWindow *w in [NSApp windows]) {
        if ([w isKindOfClass:[NSPanel class]]) continue;
        if ([w.className containsString:@"Window"]) return w;
    }
    return nil;
}

// main queue 上でのみ呼ぶこと。C 配列のみ参照するため ObjC 管理不要。
static void applyHitTest(void) {
    NSWindow *win = mainWailsWindow();
    if (!win) return;

    if (g_fullInteractive) {
        [win setIgnoresMouseEvents:NO];
        return;
    }
    if (g_hitRectCount == 0) {
        [win setIgnoresMouseEvents:YES];
        return;
    }
    NSPoint mouse  = [NSEvent mouseLocation]; // スクリーン座標 bottom-left
    CGFloat localX = mouse.x - win.frame.origin.x;
    CGFloat localY = mouse.y - win.frame.origin.y;
    NSPoint local  = NSMakePoint(localX, localY);

    BOOL hit = NO;
    for (int i = 0; i < g_hitRectCount; i++) {
        if (NSPointInRect(local, g_hitRects[i])) { hit = YES; break; }
    }
    [win setIgnoresMouseEvents:!hit];
}

// 設定/オンボーディング時: ウィンドウ全体を interactive / 通常に戻す
void SetWindowFullInteractive(bool full) {
    dispatch_async(dispatch_get_main_queue(), ^{
        g_fullInteractive = (BOOL)full;
        applyHitTest();
    });
}

// フロントエンドから CSS座標(top-left origin)の矩形リストを受け取り、
// window-local NSRect（bottom-left）に変換して C 配列に保存する。
// rects: [x1,y1,w1,h1, x2,y2,w2,h2, ...], count: 矩形数
//
// rects は Go 管理のメモリ（CGo の一時配列）。dispatch_async より前に
// スタックローカル NSRect 配列にコピーし、ObjC オブジェクトを一切使わない。
void UpdateInteractiveRectsNative(double *rects, int count) {
    // Go メモリをスタック上の NSRect 配列に即座に変換・コピーする。
    // dispatch_async のブロックには値渡し可能な POD データのみ渡す。
    int n = count;
    if (n > MAX_HIT_RECTS) n = MAX_HIT_RECTS;
    if (n < 0) n = 0;

    // ウィンドウ高さはここでは取れないため dispatch 内で取得する。
    // double 値だけ先にスタックにコピーしてブロックに値渡しする。
    // (最大 MAX_HIT_RECTS * 4 = 64 個の double = 512 bytes → スタック安全)
    double vals[MAX_HIT_RECTS * 4];
    for (int i = 0; i < n * 4; i++) {
        vals[i] = (rects && i < count * 4) ? rects[i] : 0.0;
    }
    // ブロックへのコピーのためスタック配列を固定サイズ構造体に包む
    // (可変長配列はブロックキャプチャ不可なため)
    typedef struct { double v[MAX_HIT_RECTS * 4]; } RectVals;
    RectVals rv;
    for (int i = 0; i < MAX_HIT_RECTS * 4; i++) rv.v[i] = (i < n * 4) ? vals[i] : 0.0;
    int capturedN = n;

    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *win = mainWailsWindow();
        CGFloat wh = win ? win.frame.size.height : 0;

        g_hitRectCount = 0;
        for (int i = 0; i < capturedN; i++) {
            double cssX = rv.v[i * 4 + 0];
            double cssY = rv.v[i * 4 + 1];
            double cssW = rv.v[i * 4 + 2];
            double cssH = rv.v[i * 4 + 3];
            double nsY  = wh - cssY - cssH; // CSS top-left → NSWindow bottom-left
            g_hitRects[g_hitRectCount++] = NSMakeRect(cssX, nsY, cssW, cssH);
        }

        // main-queue タイマーで 150ms ごとに hit test（初回のみ登録）
        if (!g_hitTimer) {
            g_hitTimer = dispatch_source_create(DISPATCH_SOURCE_TYPE_TIMER, 0, 0,
                                                dispatch_get_main_queue());
            uint64_t iv = 150 * NSEC_PER_MSEC;
            dispatch_source_set_timer(g_hitTimer,
                                      dispatch_time(DISPATCH_TIME_NOW, (int64_t)iv),
                                      iv, 20 * NSEC_PER_MSEC);
            dispatch_source_set_event_handler(g_hitTimer, ^{ applyHitTest(); });
            dispatch_resume(g_hitTimer);
        }
        applyHitTest();
    });
}

// ── トレイ ────────────────────────────────────────────────────────────────────

static NSPanel *talkButtonPanel;
static NSButton *talkButton;

@interface NativeTrayHandler : NSObject
- (void)onSettings:(id)sender;
- (void)onQuit:(id)sender;
@end
@implementation NativeTrayHandler
- (void)onSettings:(id)sender { goOnTraySettingsClicked(); }
- (void)onQuit:(id)sender    { goOnTrayQuitClicked(); }
@end

@interface NativeTalkHandler : NSObject
- (void)onTalk:(id)sender;
@end
@implementation NativeTalkHandler
- (void)onTalk:(id)sender { goOnTalkButtonClicked(); }
@end

static NSStatusItem *statusItem;
static NativeTrayHandler *trayHandler;
static NativeTalkHandler *talkHandler;

void SetupNativeTray(const char* iconPath, const char* settingsLabel, const char* quitLabel) {
    NSString *path     = iconPath      ? [NSString stringWithUTF8String:iconPath]      : nil;
    NSString *settings = settingsLabel ? [NSString stringWithUTF8String:settingsLabel] : @"設定を開く";
    NSString *quit     = quitLabel     ? [NSString stringWithUTF8String:quitLabel]     : @"終了";

    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem == nil) {
            statusItem  = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
            [statusItem retain];
            trayHandler = [[NativeTrayHandler alloc] init];
            [trayHandler retain];
        }
        NSImage *image = path ? [[NSImage alloc] initWithContentsOfFile:path] : nil;
        if (image) {
            [image setSize:NSMakeSize(18, 18)];
            statusItem.button.image = image;
        } else {
            statusItem.button.title = @"🌸";
        }
        NSMenu *menu = [[NSMenu alloc] init];
        [menu addItemWithTitle:settings action:@selector(onSettings:) keyEquivalent:@","];
        [menu itemArray].lastObject.target = trayHandler;
        [menu addItem:[NSMenuItem separatorItem]];
        [menu addItemWithTitle:quit action:@selector(onQuit:) keyEquivalent:@"q"];
        [menu itemArray].lastObject.target = trayHandler;
        statusItem.menu = menu;
    });
}

// ── ネイティブ話してボタンパネル ──────────────────────────────────────────────

static NSRect talkButtonFrame(bool isTop) {
    NSScreen *screen = [[NSScreen screens] objectAtIndex:0];
    NSRect screenFrame = screen.visibleFrame;
    CGFloat width = 110, height = 36;
    CGFloat x = screenFrame.origin.x + (screenFrame.size.width - width) / 2;
    CGFloat y = isTop
        ? screenFrame.origin.y + screenFrame.size.height - height - 8
        : screenFrame.origin.y + 8;
    return NSMakeRect(x, y, width, height);
}

void ShowTalkButtonPanel(bool isTop, const char* title) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (talkHandler == nil) {
            talkHandler = [[NativeTalkHandler alloc] init];
            [talkHandler retain];
        }
        if (talkButtonPanel == nil) {
            NSRect frame = talkButtonFrame(isTop);
            talkButtonPanel = [[NSPanel alloc]
                initWithContentRect:frame
                styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
                backing:NSBackingStoreBuffered
                defer:NO];
            [talkButtonPanel setLevel:NSStatusWindowLevel];
            [talkButtonPanel setOpaque:NO];
            [talkButtonPanel setBackgroundColor:[NSColor clearColor]];
            [talkButtonPanel setCollectionBehavior:
                NSWindowCollectionBehaviorCanJoinAllSpaces |
                NSWindowCollectionBehaviorStationary |
                NSWindowCollectionBehaviorIgnoresCycle];
            [talkButtonPanel setFloatingPanel:YES];
            [talkButtonPanel retain];

            talkButton = [[NSButton alloc] initWithFrame:NSMakeRect(0, 0, frame.size.width, frame.size.height)];
            talkButton.bezelStyle = NSBezelStyleRounded;
            talkButton.target = talkHandler;
            talkButton.action = @selector(onTalk:);
            [talkButton retain];
            [talkButtonPanel.contentView addSubview:talkButton];
        }
        NSString *label = title ? [NSString stringWithUTF8String:title] : @"💬 話して";
        [talkButton setTitle:label];

        NSRect frame = talkButtonFrame(isTop);
        [talkButtonPanel setFrame:frame display:NO];
        [talkButtonPanel orderFront:nil];
    });
}

void HideTalkButtonPanel() {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (talkButtonPanel) [talkButtonPanel orderOut:nil];
    });
}

void RepositionTalkButtonPanel(bool isTop) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (talkButtonPanel) {
            NSRect frame = talkButtonFrame(isTop);
            [talkButtonPanel setFrame:frame display:YES];
        }
    });
}
