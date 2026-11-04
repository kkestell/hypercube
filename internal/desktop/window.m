#import "desktop.h"

// HCPageView keeps its last mouse down, which a drag of the window starts from.
@interface HCPageView : WKWebView
@property(readonly) NSEvent *lastMouseDown;
@end

@implementation HCPageView
- (void)mouseDown:(NSEvent *)event {
    _lastMouseDown = event;
    [super mouseDown:event];
}
@end

// The title bar is a compact toolbar row plus this much, which the window's
// buttons are centered in by moving them down half of it.
static const CGFloat HCTitleBarExtra = 6;

@implementation HCWindow {
    NSURL *_home;
    // The title bar's height and the width its buttons take from the left.
    CGFloat _titleBarHeight, _titleBarInset;
}

- (instancetype)initWithURL:(NSURL *)url {
    self = [super init];
    _home = url;
    _webView = [[HCPageView alloc] initWithFrame:NSZeroRect configuration:[[WKWebViewConfiguration alloc] init]];
    _webView.navigationDelegate = self;
    _webView.UIDelegate = self;
    _webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    if (@available(macOS 13.3, *)) _webView.inspectable = YES;
    [_webView addObserver:self forKeyPath:@"title" options:0 context:nil];
    [_webView.configuration.userContentController addScriptMessageHandler:self name:@"window"];

    // The page fills the window, title bar included, and draws the title bar's
    // row, its tab bars, beside the window's buttons.
    NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable | NSWindowStyleMaskFullSizeContentView;
    _window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1280, 800) styleMask:style backing:NSBackingStoreBuffered defer:NO];
    _window.releasedWhenClosed = NO;
    _window.delegate = self;
    _window.title = @"Hypercube";
    _window.titlebarAppearsTransparent = YES;
    _window.titleVisibility = NSWindowTitleHidden;
    // An empty compact toolbar makes the title bar as tall as a toolbar row and
    // centers the window's buttons in it, so the page's tab bars fill that row.
    _window.toolbar = [[NSToolbar alloc] initWithIdentifier:@"HypercubeTitleBar"];
    _window.toolbarStyle = NSWindowToolbarStyleUnifiedCompact;
    _window.contentMinSize = NSMakeSize(640, 400);
    _webView.frame = _window.contentView.bounds;
    [_window.contentView addSubview:_webView];
    [_window center];
    [_window setFrameUsingName:@"Hypercube"];
    [_window setFrameAutosaveName:@"Hypercube"];
    _titleBarHeight = NSHeight(_window.frame) - NSHeight(_window.contentLayoutRect) + HCTitleBarExtra;
    // The buttons get the same margin on their right as on their left.
    NSButton *close = [_window standardWindowButton:NSWindowCloseButton], *zoom = [_window standardWindowButton:NSWindowZoomButton];
    _titleBarInset = NSMaxX([zoom convertRect:zoom.bounds toView:nil]) + NSMinX([close convertRect:close.bounds toView:nil]);
    [self centerWindowButtons];
    [_webView loadRequest:[NSURLRequest requestWithURL:url]];
    return self;
}

// centerWindowButtons moves the window's buttons down to the middle of the
// taller title bar. The window puts them back when it lays out its title bar,
// so this runs again after each change of size or state.
- (void)centerWindowButtons {
    if (_window.styleMask & NSWindowStyleMaskFullScreen) return;
    for (NSNumber *kind in @[@(NSWindowCloseButton), @(NSWindowMiniaturizeButton), @(NSWindowZoomButton)]) {
        NSButton *button = [_window standardWindowButton:(NSWindowButton)kind.integerValue];
        NSView *bar = button.superview;
        if (!button || !bar) continue;
        NSPoint origin = button.frame.origin;
        CGFloat middle = (NSHeight(bar.frame) - NSHeight(button.frame)) / 2;
        origin.y = bar.isFlipped ? middle + HCTitleBarExtra / 2 : middle - HCTitleBarExtra / 2;
        [button setFrameOrigin:origin];
    }
}

- (void)windowDidResize:(NSNotification *)notification { [self centerWindowButtons]; }
- (void)windowDidBecomeKey:(NSNotification *)notification { [self centerWindowButtons]; }
- (void)windowDidExitFullScreen:(NSNotification *)notification { [self centerWindowButtons]; }

// The app is in the Dock and the app switcher only while the window shows.
- (void)show {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    if (_window.miniaturized) [_window deminiaturize:nil];
    [_window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (BOOL)windowShouldClose:(NSWindow *)sender {
    [_window orderOut:nil];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    return NO;
}

- (void)dispatch:(NSString *)name detail:(NSDictionary *)detail {
    [_webView callAsyncJavaScript:@"dispatchEvent(new CustomEvent(name, {detail}))"
                        arguments:@{@"name": name, @"detail": detail}
                          inFrame:nil
                   inContentWorld:WKContentWorld.pageWorld
                completionHandler:nil];
}

// reportTitleBar sends the page a hypercube-title-bar event, {height, inset},
// with the title bar's height and the width of the window's buttons. A full
// screen window has no title bar in the page.
- (void)reportTitleBar:(BOOL)fullScreen {
    [self dispatch:@"hypercube-title-bar" detail:fullScreen ? @{@"height": @0, @"inset": @0} : @{@"height": @(_titleBarHeight / _webView.pageZoom), @"inset": @(_titleBarInset / _webView.pageZoom)}];
}

- (void)windowWillEnterFullScreen:(NSNotification *)notification {
    [self reportTitleBar:YES];
}

- (void)windowWillExitFullScreen:(NSNotification *)notification {
    [self reportTitleBar:NO];
}

// The page reports the badge count, asks for the title bar, and moves or zooms
// the window from a mouse down in its title bar row.
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!message.frameInfo.isMainFrame) return;
    NSDictionary *body = message.body;
    if (![body isKindOfClass:[NSDictionary class]]) return;
    NSString *type = body[@"type"];
    if ([type isEqual:@"title-bar"]) {
        [self reportTitleBar:(_window.styleMask & NSWindowStyleMaskFullScreen) != 0];
    } else if ([type isEqual:@"badge"]) {
        NSNumber *count = body[@"count"];
        if ([count isKindOfClass:[NSNumber class]]) HCSetBadge(count.integerValue);
    } else if ([type isEqual:@"shortcuts"]) {
        HCSetShortcuts(body[@"items"]);
    } else if ([type isEqual:@"page-zoom"]) {
        NSString *action = body[@"action"];
        if ([action isEqual:@"zoomIn"]) _webView.pageZoom *= 1.2;
        else if ([action isEqual:@"zoomOut"]) _webView.pageZoom /= 1.2;
        else if ([action isEqual:@"zoomReset"]) _webView.pageZoom = 1;
        [self reportTitleBar:(_window.styleMask & NSWindowStyleMaskFullScreen) != 0];
    } else if ([type isEqual:@"drag"]) {
        NSEvent *down = ((HCPageView *)_webView).lastMouseDown;
        if (down && (NSEvent.pressedMouseButtons & 1)) [_window performWindowDragWithEvent:down];
    } else if ([type isEqual:@"zoom"]) {
        NSString *action = [NSUserDefaults.standardUserDefaults stringForKey:@"AppleActionOnDoubleClick"];
        if ([action isEqual:@"Minimize"]) [_window performMiniaturize:nil];
        else if (![action isEqual:@"None"]) [_window performZoom:nil];
    }
}

// The window's title is the page's.
- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
    _window.title = _webView.title.length ? _webView.title : @"Hypercube";
}

- (BOOL)isHome:(NSURL *)url {
    return [url.scheme isEqual:_home.scheme] && [url.host isEqual:_home.host] && [url.port isEqual:_home.port];
}

// Links away from the UI open in the default browser.
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    if (!action.targetFrame.isMainFrame || [self isHome:action.request.URL]) {
        decisionHandler(WKNavigationActionPolicyAllow);
        return;
    }
    [[NSWorkspace sharedWorkspace] openURL:action.request.URL];
    decisionHandler(WKNavigationActionPolicyCancel);
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features {
    [[NSWorkspace sharedWorkspace] openURL:action.request.URL];
    return nil;
}

- (void)webView:(WKWebView *)webView runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler {
    HCRunOpenPanel(parameters, _window, completionHandler);
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
    [webView reload];
}
@end

void HCRunOpenPanel(WKOpenPanelParameters *parameters, NSWindow *window, void (^completion)(NSArray<NSURL *> *)) {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.allowsMultipleSelection = parameters.allowsMultipleSelection;
    panel.canChooseDirectories = parameters.allowsDirectories;
    [panel beginSheetModalForWindow:window completionHandler:^(NSModalResponse response) {
        completion(response == NSModalResponseOK ? panel.URLs : nil);
    }];
}
