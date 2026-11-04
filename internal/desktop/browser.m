#import "desktop.h"
#include "_cgo_export.h"

// HCBrowsers keeps each session's browser: a web view over the Browser tab's
// body, where the page places it. The views live until the app quits and use
// the default website data store, so they share logins, which last across
// restarts.
@interface HCBrowsers : NSObject <WKScriptMessageHandler, WKNavigationDelegate, WKUIDelegate>
@end

static HCBrowsers *browsers;

static NSArray<NSString *> *reportedKeys(void) {
    return @[@"URL", @"title", @"loading", @"canGoBack", @"canGoForward"];
}

static void succeed(uint64_t request, NSString *text) {
    hypercubeDone(request, (char *)text.UTF8String, NULL);
}

static void fail(uint64_t request, NSString *message) {
    hypercubeDone(request, NULL, (char *)message.UTF8String);
}

// addressURL reads an address as a browser's address bar does: one without
// a scheme is an https URL.
static NSURL *addressURL(NSString *address) {
    if (![address isKindOfClass:[NSString class]]) return nil;
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

@implementation HCBrowsers {
    HCWindow *_window;
    NSMutableDictionary<NSString *, WKWebView *> *_views;
    // The agents' navigations that have yet to finish, to their requests.
    NSMapTable<WKNavigation *, NSNumber *> *_navigations;
}

- (instancetype)initWithWindow:(HCWindow *)window {
    self = [super init];
    _window = window;
    _views = [NSMutableDictionary dictionary];
    _navigations = [NSMapTable strongToStrongObjectsMapTable];
    [window.webView.configuration.userContentController addScriptMessageHandler:self name:@"hypercube"];
    return self;
}

- (WKWebView *)viewFor:(NSString *)session {
    WKWebView *view = _views[session];
    if (view) return view;
    view = [[WKWebView alloc] initWithFrame:NSZeroRect configuration:[[WKWebViewConfiguration alloc] init]];
    view.navigationDelegate = self;
    view.UIDelegate = self;
    view.hidden = YES;
    if (@available(macOS 13.3, *)) view.inspectable = YES;
    _views[session] = view;
    for (NSString *key in reportedKeys()) [view addObserver:self forKeyPath:key options:0 context:nil];
    [_window.window.contentView addSubview:view positioned:NSWindowAbove relativeTo:_window.webView];
    return view;
}

// report sends the page a browser's state. reveal asks the page to show it.
- (void)report:(WKWebView *)view reveal:(BOOL)reveal {
    [_window dispatch:@"hypercube-browser" detail:@{
        @"session": [_views allKeysForObject:view].firstObject,
        @"url": view.URL.absoluteString ?: @"",
        @"title": view.title ?: @"",
        @"loading": @(view.loading),
        @"canGoBack": @(view.canGoBack),
        @"canGoForward": @(view.canGoForward),
        @"reveal": @(reveal),
    }];
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
    [self report:object reveal:NO];
}

// place shows the session's view at rect, in the page's CSS pixels, and hides
// every other view. A hidden view gives up the keyboard to the page.
- (void)place:(id)session rect:(id)rect {
    NSRect frame = NSZeroRect;
    if ([rect isKindOfClass:[NSDictionary class]]) {
        CGFloat zoom = _window.webView.pageZoom;
        NSRect inPage = NSMakeRect([rect[@"x"] doubleValue] * zoom, [rect[@"y"] doubleValue] * zoom, [rect[@"width"] doubleValue] * zoom, [rect[@"height"] doubleValue] * zoom);
        frame = [_window.webView convertRect:inPage toView:_window.window.contentView];
    }
    NSWindow *window = _window.window;
    [_views enumerateKeysAndObjectsUsingBlock:^(NSString *key, WKWebView *view, BOOL *stop) {
        BOOL shown = [key isEqual:session];
        if (shown) view.frame = frame;
        if (!shown && [window.firstResponder isKindOfClass:[NSView class]] && [(NSView *)window.firstResponder isDescendantOf:view]) {
            [window makeFirstResponder:self->_window.webView];
        }
        view.hidden = !shown;
    }];
}

// A page that loads has placed no browser yet and asks for every browser's
// state. It then places the shown browser and passes on the toolbar's buttons
// and address.
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    // Only the UI page may control browsers; embedded frames must not.
    if (!message.frameInfo.isMainFrame) return;
    NSDictionary *body = message.body;
    if (![body isKindOfClass:[NSDictionary class]]) return;
    NSString *type = body[@"type"], *session = body[@"session"];
    if ([type isEqual:@"ready"]) {
        [self place:nil rect:nil];
        for (WKWebView *view in _views.allValues) [self report:view reveal:NO];
        return;
    }
    if ([type isEqual:@"place"]) {
        [self place:session rect:body[@"rect"]];
        return;
    }
    if (![session isKindOfClass:[NSString class]]) return;
    if ([type isEqual:@"navigate"]) {
        NSURL *url = addressURL(body[@"url"]);
        if (url) [[self viewFor:session] loadRequest:[NSURLRequest requestWithURL:url]];
        return;
    }
    WKWebView *view = _views[session];
    if ([type isEqual:@"back"]) [view goBack];
    else if ([type isEqual:@"forward"]) [view goForward];
    else if ([type isEqual:@"reload"]) [view reload];
    else if ([type isEqual:@"external"] && view.URL) [[NSWorkspace sharedWorkspace] openURL:view.URL];
}

- (void)navigate:(NSString *)session to:(NSString *)address request:(uint64_t)request {
    NSURL *url = addressURL(address);
    if (!url) {
        fail(request, [NSString stringWithFormat:@"%@ is not a URL", address]);
        return;
    }
    WKWebView *view = [self viewFor:session];
    // WebKit does not call didFinishNavigation for a fragment change.
    if (isFragmentNavigation(view.URL, url)) {
        [view callAsyncJavaScript:@"location.hash = new URL(url).hash; return JSON.stringify({url: location.href, title: document.title});"
                       arguments:@{@"url": url.absoluteString}
                         inFrame:nil
                  inContentWorld:WKContentWorld.pageWorld
               completionHandler:^(id result, NSError *error) {
            if (error) fail(request, error.localizedDescription);
            else succeed(request, result);
        }];
        [self report:view reveal:YES];
        return;
    }
    WKNavigation *navigation = [view loadRequest:[NSURLRequest requestWithURL:url]];
    if (!navigation) {
        fail(request, [NSString stringWithFormat:@"%@ could not be loaded", address]);
        return;
    }
    [_navigations setObject:@(request) forKey:navigation];
    [self report:view reveal:YES];
}

- (NSNumber *)takeRequest:(WKNavigation *)navigation {
    NSNumber *request = [_navigations objectForKey:navigation];
    [_navigations removeObjectForKey:navigation];
    return request;
}

- (void)webView:(WKWebView *)view didFinishNavigation:(WKNavigation *)navigation {
    NSNumber *request = [self takeRequest:navigation];
    if (!request) return;
    NSDictionary *page = @{@"url": view.URL.absoluteString ?: @"", @"title": view.title ?: @""};
    NSData *json = [NSJSONSerialization dataWithJSONObject:page options:0 error:nil];
    succeed(request.unsignedLongLongValue, [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding]);
}

- (void)webView:(WKWebView *)view didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    NSNumber *request = [self takeRequest:navigation];
    if (request) fail(request.unsignedLongLongValue, error.localizedDescription);
}

- (void)webView:(WKWebView *)view didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    [self webView:view didFailNavigation:navigation withError:error];
}

- (void)eval:(NSString *)script in:(NSString *)session request:(uint64_t)request {
    NSString *body = [NSString stringWithFormat:@"const value = await (async () => {\n%@\n})();\nreturn JSON.stringify(value) ?? 'undefined';", script];
    [[self viewFor:session] callAsyncJavaScript:body arguments:nil inFrame:nil inContentWorld:WKContentWorld.pageWorld completionHandler:^(id result, NSError *error) {
        if (error) {
            fail(request, error.userInfo[@"WKJavaScriptExceptionMessage"] ?: error.localizedDescription);
            return;
        }
        succeed(request, [result isKindOfClass:[NSString class]] ? result : @"undefined");
    }];
}

// close removes the session's browser and tells the page, which closes its
// tab.
- (void)close:(NSString *)session request:(uint64_t)request {
    WKWebView *view = _views[session];
    if (view) {
        [_views removeObjectForKey:session];
        for (NSString *key in reportedKeys()) [view removeObserver:self forKeyPath:key];
        [view stopLoading];
        view.navigationDelegate = nil;
        view.UIDelegate = nil;
        NSWindow *window = _window.window;
        if ([window.firstResponder isKindOfClass:[NSView class]] && [(NSView *)window.firstResponder isDescendantOf:view]) {
            [window makeFirstResponder:_window.webView];
        }
        [view removeFromSuperview];
        [_window dispatch:@"hypercube-browser-closed" detail:@{@"session": session}];
    }
    succeed(request, @"");
}

// A page's new window opens in its own browser.
- (WKWebView *)webView:(WKWebView *)view createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features {
    [view loadRequest:action.request];
    return nil;
}

- (void)webView:(WKWebView *)view runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler {
    HCRunOpenPanel(parameters, _window.window, completionHandler);
}
@end

void HCBrowsersStart(HCWindow *window) {
    browsers = [[HCBrowsers alloc] initWithWindow:window];
}

void hypercubeNavigate(const char *session, const char *url, uint64_t request) {
    NSString *s = @(session), *u = @(url);
    dispatch_async(dispatch_get_main_queue(), ^{
        [browsers navigate:s to:u request:request];
    });
}

void hypercubeEval(const char *session, const char *script, uint64_t request) {
    NSString *s = @(session), *code = @(script);
    dispatch_async(dispatch_get_main_queue(), ^{
        [browsers eval:code in:s request:request];
    });
}

void hypercubeClose(const char *session, uint64_t request) {
    NSString *s = @(session);
    dispatch_async(dispatch_get_main_queue(), ^{
        [browsers close:s request:request];
    });
}
