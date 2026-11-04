#import "desktop.h"

// HCApp owns the application's objects and answers the menus, the Dock, and
// requests to quit.
@interface HCApp : NSObject <NSApplicationDelegate>
@property HCWindow *window;
@property NSStatusItem *statusItem;
@end

static HCApp *application;

@implementation HCApp
- (void)show:(id)sender {
    [_window show];
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)app hasVisibleWindows:(BOOL)visible {
    [_window show];
    return NO;
}

// A quit the user chooses, from a menu or the Dock, ends the application loop
// rather than the process, so hypercubeRun returns and the app shuts down its
// agents. Logging out, restarting, and shutting down carry a reason and end
// the process now.
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)app {
    NSAppleEventDescriptor *event = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
    if ([event attributeDescriptorForKeyword:kAEQuitReason]) return NSTerminateNow;
    [NSApp stop:nil];
    NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint modifierFlags:0
                                      timestamp:0 windowNumber:0 context:nil subtype:0 data1:0 data2:0];
    [NSApp postEvent:wake atStart:YES];
    return NSTerminateCancel;
}
@end

static void addItem(NSMenu *menu, NSString *title, SEL action, NSString *key, NSEventModifierFlags modifiers) {
    NSMenuItem *item = [menu addItemWithTitle:title action:action keyEquivalent:key];
    item.keyEquivalentModifierMask = modifiers;
}

static void addSubmenu(NSMenu *bar, NSMenu *menu) {
    [bar addItemWithTitle:menu.title action:nil keyEquivalent:@""].submenu = menu;
}

// mainMenu is the standard App, Edit, and Window menus, which also give the
// web views their keyboard shortcuts.
static NSMenu *mainMenu(void) {
    NSEventModifierFlags command = NSEventModifierFlagCommand;
    NSMenu *bar = [[NSMenu alloc] init];

    NSMenu *app = [[NSMenu alloc] initWithTitle:@"Hypercube"];
    addItem(app, @"Hide Hypercube", @selector(hide:), @"h", command);
    addItem(app, @"Hide Others", @selector(hideOtherApplications:), @"h", command | NSEventModifierFlagOption);
    addItem(app, @"Show All", @selector(unhideAllApplications:), @"", 0);
    [app addItem:[NSMenuItem separatorItem]];
    addItem(app, @"Quit Hypercube", @selector(terminate:), @"q", command);
    addSubmenu(bar, app);

    NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
    addItem(edit, @"Undo", @selector(undo:), @"z", command);
    addItem(edit, @"Redo", @selector(redo:), @"z", command | NSEventModifierFlagShift);
    [edit addItem:[NSMenuItem separatorItem]];
    addItem(edit, @"Cut", @selector(cut:), @"x", command);
    addItem(edit, @"Copy", @selector(copy:), @"c", command);
    addItem(edit, @"Paste", @selector(paste:), @"v", command);
    addItem(edit, @"Select All", @selector(selectAll:), @"a", command);
    addSubmenu(bar, edit);

    NSMenu *window = [[NSMenu alloc] initWithTitle:@"Window"];
    addItem(window, @"Minimize", @selector(performMiniaturize:), @"m", command);
    addItem(window, @"Zoom", @selector(performZoom:), @"", 0);
    addItem(window, @"Close", @selector(performClose:), @"w", command);
    [window addItem:[NSMenuItem separatorItem]];
    addItem(window, @"Bring All to Front", @selector(arrangeInFront:), @"", 0);
    addSubmenu(bar, window);
    NSApp.windowsMenu = window;
    return bar;
}

static NSStatusItem *statusItem(const void *icon, size_t iconLen) {
    NSStatusItem *item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    NSImage *image = [[NSImage alloc] initWithData:[NSData dataWithBytes:icon length:iconLen]];
    image.template = YES;
    image.size = NSMakeSize(18, 18);
    item.button.image = image;
    item.button.imagePosition = NSImageLeft;
    item.button.accessibilityLabel = @"Hypercube";

    NSMenu *menu = [[NSMenu alloc] init];
    [menu addItemWithTitle:@"Show Hypercube" action:@selector(show:) keyEquivalent:@""].target = application;
    [menu addItem:[NSMenuItem separatorItem]];
    [menu addItemWithTitle:@"Quit Hypercube" action:@selector(terminate:) keyEquivalent:@"q"];
    item.menu = menu;
    return item;
}

void HCSetBadge(NSInteger count) {
    NSString *label = count ? [NSString stringWithFormat:@"%ld", (long)count] : @"";
    NSApp.dockTile.badgeLabel = count ? label : nil;
    application.statusItem.button.title = label;
    application.statusItem.button.accessibilityLabel = count
        ? [NSString stringWithFormat:@"Hypercube, %ld sessions waiting on you", (long)count] : @"Hypercube";
}

void hypercubeRun(const char *url, const void *icon, size_t iconLen) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        app.activationPolicy = NSApplicationActivationPolicyAccessory;
        application = [[HCApp alloc] init];
        app.delegate = application;
        app.mainMenu = mainMenu();
        application.window = [[HCWindow alloc] initWithURL:[NSURL URLWithString:@(url)]];
        HCBrowsersStart(application.window);
        HCNotificationsStart(application.window);
        application.statusItem = statusItem(icon, iconLen);
        [app run];
    }
}
