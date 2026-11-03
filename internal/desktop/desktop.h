// The classes and functions the application's files share.
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// HCWindow is the window that shows the Hypercube UI. Closing it hides it,
// and the app keeps running in the menu bar.
@interface HCWindow : NSObject <NSWindowDelegate, WKNavigationDelegate, WKUIDelegate>
@property(readonly) NSWindow *window;
@property(readonly) WKWebView *webView;
- (instancetype)initWithURL:(NSURL *)url;
- (void)show;
// dispatch sends the page a CustomEvent with the name and detail.
- (void)dispatch:(NSString *)name detail:(NSDictionary *)detail;
@end

// HCRunOpenPanel answers a web page's file input with an open panel.
void HCRunOpenPanel(WKOpenPanelParameters *parameters, NSWindow *window, void (^completion)(NSArray<NSURL *> *));

// HCBrowsersStart gives the window's page the sessions' browsers.
void HCBrowsersStart(HCWindow *window);
