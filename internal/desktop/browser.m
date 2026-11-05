#import "desktop.h"
#include "_cgo_export.h"

// Each session owns an ordinary native browser window. AppKit lays out its
// controls and web view; the Hypercube page has no browser placement state.
@interface HCBrowser : NSObject <NSWindowDelegate, WKNavigationDelegate, WKUIDelegate, NSTextFieldDelegate>
- (instancetype)initWithSession:(NSString *)session;
- (void)navigate:(NSString *)address request:(uint64_t)request;
- (void)eval:(NSString *)script request:(uint64_t)request;
- (void)close;
@end

static NSMutableDictionary<NSString *, HCBrowser *> *browsers;

static NSArray<NSString *> *observedKeys(void) {
    return @[@"URL", @"title", @"canGoBack", @"canGoForward"];
}

static void succeed(uint64_t request, NSString *text) {
    hypercubeDone(request, (char *)text.UTF8String, NULL);
}

static void fail(uint64_t request, NSString *message) {
    hypercubeDone(request, NULL, (char *)message.UTF8String);
}

// An address without a scheme uses https, as in an ordinary address bar.
static NSURL *addressURL(NSString *address) {
    address = [address stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceCharacterSet]];
    if (![address containsString:@"://"] && ![address hasPrefix:@"about:"]) address = [@"https://" stringByAppendingString:address];
    return [NSURL URLWithString:address];
}

static BOOL isFragmentNavigation(NSURL *from, NSURL *to) {
    if (!from || [from isEqual:to]) return NO;
    NSURLComponents *current = [NSURLComponents componentsWithURL:from resolvingAgainstBaseURL:NO];
    NSURLComponents *target = [NSURLComponents componentsWithURL:to resolvingAgainstBaseURL:NO];
    current.fragment = nil;
    target.fragment = nil;
    return [current.URL isEqual:target.URL];
}

@implementation HCBrowser {
    NSString *_session;
    NSWindow *_window;
    WKWebView *_view;
    NSTextField *_address;
    NSButton *_back, *_forward;
    // Agent navigations waiting for WebKit's completion notification.
    NSMapTable<WKNavigation *, NSNumber *> *_navigations;
}

- (NSButton *)button:(NSString *)symbol label:(NSString *)label action:(SEL)action {
    NSButton *button = [NSButton buttonWithImage:[NSImage imageWithSystemSymbolName:symbol accessibilityDescription:label]
                                        target:self action:action];
    button.bezelStyle = NSBezelStyleTexturedRounded;
    button.accessibilityLabel = label;
    button.toolTip = label;
    return button;
}

- (instancetype)initWithSession:(NSString *)session {
    self = [super init];
    _session = session;
    _navigations = [NSMapTable strongToStrongObjectsMapTable];
    WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
    configuration.websiteDataStore = WKWebsiteDataStore.defaultDataStore;
    _view = [[WKWebView alloc] initWithFrame:NSZeroRect configuration:configuration];
    _view.navigationDelegate = self;
    _view.UIDelegate = self;
    if (@available(macOS 13.3, *)) _view.inspectable = YES;

    NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
    _window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1000, 720) styleMask:style backing:NSBackingStoreBuffered defer:NO];
    _window.releasedWhenClosed = NO;
    _window.delegate = self;
    _window.contentMinSize = NSMakeSize(500, 300);
    [_window center];

    _back = [self button:@"chevron.left" label:@"Back" action:@selector(back:)];
    _forward = [self button:@"chevron.right" label:@"Forward" action:@selector(forward:)];
    NSButton *reload = [self button:@"arrow.clockwise" label:@"Reload" action:@selector(reload:)];
    NSButton *external = [self button:@"safari" label:@"Open in default browser" action:@selector(external:)];
    _address = [[NSTextField alloc] initWithFrame:NSZeroRect];
    _address.placeholderString = @"Address";
    _address.accessibilityLabel = @"Address";
    _address.target = self;
    _address.action = @selector(enteredAddress:);
    _address.delegate = self;
    [_address setContentHuggingPriority:NSLayoutPriorityDefaultLow forOrientation:NSLayoutConstraintOrientationHorizontal];
    NSStackView *bar = [NSStackView stackViewWithViews:@[_back, _forward, reload, _address, external]];
    bar.spacing = 8;
    bar.edgeInsets = NSEdgeInsetsMake(8, 8, 8, 8);
    bar.alignment = NSLayoutAttributeCenterY;

    NSView *content = _window.contentView;
    bar.translatesAutoresizingMaskIntoConstraints = NO;
    _view.translatesAutoresizingMaskIntoConstraints = NO;
    [content addSubview:bar];
    [content addSubview:_view];
    [NSLayoutConstraint activateConstraints:@[
        [bar.topAnchor constraintEqualToAnchor:content.topAnchor],
        [bar.leadingAnchor constraintEqualToAnchor:content.leadingAnchor],
        [bar.trailingAnchor constraintEqualToAnchor:content.trailingAnchor],
        [_view.topAnchor constraintEqualToAnchor:bar.bottomAnchor],
        [_view.leadingAnchor constraintEqualToAnchor:content.leadingAnchor],
        [_view.trailingAnchor constraintEqualToAnchor:content.trailingAnchor],
        [_view.bottomAnchor constraintEqualToAnchor:content.bottomAnchor],
    ]];
    for (NSString *key in observedKeys()) [_view addObserver:self forKeyPath:key options:0 context:nil];
    [self updateControls];
    return self;
}

- (void)show {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    if (_window.miniaturized) [_window deminiaturize:nil];
    [_window makeKeyAndOrderFront:nil];
}

- (void)updateControls {
    _window.title = [NSString stringWithFormat:@"%@ — %@", _view.title.length ? _view.title : @"Browser", _session];
    // Keep an address being typed while the page changes in the background.
    if (!_address.currentEditor) _address.stringValue = _view.URL.absoluteString ?: @"";
    _back.enabled = _view.canGoBack;
    _forward.enabled = _view.canGoForward;
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
    [self updateControls];
}

- (void)back:(id)sender { [_view goBack]; }
- (void)forward:(id)sender { [_view goForward]; }
- (void)reload:(id)sender { [_view reload]; }
- (void)external:(id)sender {
    if (_view.URL) [[NSWorkspace sharedWorkspace] openURL:_view.URL];
}
- (void)enteredAddress:(id)sender {
    NSURL *url = addressURL(_address.stringValue);
    if (url) [_view loadRequest:[NSURLRequest requestWithURL:url]];
    [_window makeFirstResponder:_view];
    [self updateControls];
}
- (BOOL)control:(NSControl *)control textView:(NSTextView *)textView doCommandBySelector:(SEL)selector {
    if (selector != @selector(cancelOperation:)) return NO;
    [_window makeFirstResponder:_view];
    [self updateControls];
    return YES;
}

- (void)navigate:(NSString *)address request:(uint64_t)request {
    NSURL *url = addressURL(address);
    if (!url) {
        fail(request, [NSString stringWithFormat:@"%@ is not a URL", address]);
        return;
    }
    [self show];
    // WebKit does not call didFinishNavigation for a fragment change.
    if (isFragmentNavigation(_view.URL, url)) {
        [_view callAsyncJavaScript:@"location.hash = new URL(url).hash; return JSON.stringify({url: location.href, title: document.title});"
                        arguments:@{@"url": url.absoluteString}
                          inFrame:nil
                   inContentWorld:WKContentWorld.pageWorld
                completionHandler:^(id result, NSError *error) {
            if (error) fail(request, error.localizedDescription);
            else succeed(request, result);
        }];
        return;
    }
    WKNavigation *navigation = [_view loadRequest:[NSURLRequest requestWithURL:url]];
    if (!navigation) {
        fail(request, [NSString stringWithFormat:@"%@ could not be loaded", address]);
        return;
    }
    [_navigations setObject:@(request) forKey:navigation];
}

- (NSNumber *)takeRequest:(WKNavigation *)navigation {
    NSNumber *request = [_navigations objectForKey:navigation];
    [_navigations removeObjectForKey:navigation];
    return request;
}

- (void)webView:(WKWebView *)view didFinishNavigation:(WKNavigation *)navigation {
    NSNumber *request = [self takeRequest:navigation];
    if (!request) return;
    // WebKit's title property can lag behind didFinishNavigation. Read the
    // loaded document to return its title with the navigation result.
    [view callAsyncJavaScript:@"return JSON.stringify({url: location.href, title: document.title});"
                   arguments:nil inFrame:nil inContentWorld:WKContentWorld.pageWorld
           completionHandler:^(id result, NSError *error) {
        if (error) fail(request.unsignedLongLongValue, error.localizedDescription);
        else succeed(request.unsignedLongLongValue, result);
    }];
}

- (void)webView:(WKWebView *)view didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    NSNumber *request = [self takeRequest:navigation];
    if (request) fail(request.unsignedLongLongValue, error.localizedDescription);
}

- (void)webView:(WKWebView *)view didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    [self webView:view didFailNavigation:navigation withError:error];
}

- (void)eval:(NSString *)script request:(uint64_t)request {
    NSString *body = [NSString stringWithFormat:@"const value = await (async () => {\n%@\n})();\nreturn JSON.stringify(value) ?? 'undefined';", script];
    [_view callAsyncJavaScript:body arguments:nil inFrame:nil inContentWorld:WKContentWorld.pageWorld completionHandler:^(id result, NSError *error) {
        if (error) {
            fail(request, error.userInfo[@"WKJavaScriptExceptionMessage"] ?: error.localizedDescription);
            return;
        }
        succeed(request, [result isKindOfClass:[NSString class]] ? result : @"undefined");
    }];
}

- (void)close { [_window close]; }

- (void)windowWillClose:(NSNotification *)notification {
    // Removal releases the manager's ownership after this delegate returns.
    for (NSString *key in observedKeys()) [_view removeObserver:self forKeyPath:key];
    _view.navigationDelegate = nil;
    _view.UIDelegate = nil;
    [_view stopLoading];
    for (NSNumber *request in _navigations.objectEnumerator) fail(request.unsignedLongLongValue, @"The browser was closed");
    [_navigations removeAllObjects];
    HCHideFromDockIfNoWindows(_window);
    [browsers removeObjectForKey:_session];
}

// Preserve the existing single-page behavior for links opening new windows.
- (WKWebView *)webView:(WKWebView *)view createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features {
    [view loadRequest:action.request];
    return nil;
}

- (void)webView:(WKWebView *)view runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler {
    HCRunOpenPanel(parameters, _window, completionHandler);
}
@end

static HCBrowser *browserFor(NSString *session) {
    HCBrowser *browser = browsers[session];
    if (!browser) {
        browser = [[HCBrowser alloc] initWithSession:session];
        browsers[session] = browser;
    }
    return browser;
}

void HCBrowsersStart(void) {
    browsers = [NSMutableDictionary dictionary];
}

void hypercubeNavigate(const char *session, const char *url, uint64_t request) {
    NSString *s = @(session), *u = @(url);
    dispatch_async(dispatch_get_main_queue(), ^{ [browserFor(s) navigate:u request:request]; });
}

void hypercubeEval(const char *session, const char *script, uint64_t request) {
    NSString *s = @(session), *code = @(script);
    dispatch_async(dispatch_get_main_queue(), ^{ [browserFor(s) eval:code request:request]; });
}

void hypercubeClose(const char *session, uint64_t request) {
    NSString *s = @(session);
    dispatch_async(dispatch_get_main_queue(), ^{
        [browsers[s] close];
        succeed(request, @"");
    });
}
