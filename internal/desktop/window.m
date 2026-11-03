#import "desktop.h"

@implementation HCWindow {
    NSURL *_home;
}

- (instancetype)initWithURL:(NSURL *)url {
    self = [super init];
    _home = url;
    _webView = [[WKWebView alloc] initWithFrame:NSZeroRect configuration:[[WKWebViewConfiguration alloc] init]];
    _webView.navigationDelegate = self;
    _webView.UIDelegate = self;
    _webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    if (@available(macOS 13.3, *)) _webView.inspectable = YES;
    [_webView addObserver:self forKeyPath:@"title" options:0 context:nil];

    NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
    _window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1280, 800) styleMask:style backing:NSBackingStoreBuffered defer:NO];
    _window.releasedWhenClosed = NO;
    _window.delegate = self;
    _window.title = @"Hypercube";
    _window.contentMinSize = NSMakeSize(640, 400);
    _webView.frame = _window.contentView.bounds;
    [_window.contentView addSubview:_webView];
    [_window center];
    [_window setFrameUsingName:@"Hypercube"];
    [_window setFrameAutosaveName:@"Hypercube"];
    [_webView loadRequest:[NSURLRequest requestWithURL:url]];
    return self;
}

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
