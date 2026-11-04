#import "desktop.h"

// HCApp owns the application's objects and answers the menus, the Dock, and
// requests to quit.
@interface HCApp : NSObject <NSApplicationDelegate>
@property HCWindow *window;
@property NSStatusItem *statusItem;
@end

// Menu shortcuts reach the web view so its normal keyboard handler can decide
// whether the focused control owns them. Clicking a menu item still invokes it.
@interface HCShortcutMenu : NSMenu
@end
@implementation HCShortcutMenu
- (BOOL)performKeyEquivalent:(NSEvent *)event {
    return NO;
}
@end

@interface HCAppMenu : NSMenu
@end
@implementation HCAppMenu
- (BOOL)performKeyEquivalent:(NSEvent *)event {
    if ([event.charactersIgnoringModifiers isEqual:@","] && (event.modifierFlags & NSEventModifierFlagCommand)) return NO;
    return [super performKeyEquivalent:event];
}
@end

static HCApp *application;

@implementation HCApp
- (void)show:(id)sender {
    [_window show];
}

- (void)shortcut:(NSMenuItem *)sender {
    [_window dispatch:@"hypercube-shortcut" detail:@{@"id": sender.representedObject}];
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

    NSMenu *app = [[HCAppMenu alloc] initWithTitle:@"Hypercube"];
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

void HCSetShortcuts(NSArray *items) {
    NSMenu *bar = NSApp.mainMenu;
    for (NSString *title in @[@"App", @"Session", @"View", @"Help"]) {
        if ([title isEqual:@"App"]) {
            NSMenu *app = [bar itemWithTitle:@"Hypercube"].submenu;
            NSMenuItem *old = [app itemWithTag:1];
            if (old) { [app removeItemAtIndex:[app indexOfItem:old] + 1]; [app removeItem:old]; }
            for (NSDictionary *entry in items) {
                if (![entry[@"section"] isEqual:title]) continue;
                NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:entry[@"title"] action:@selector(shortcut:) keyEquivalent:entry[@"binding"][@"key"]];
                item.keyEquivalentModifierMask = NSEventModifierFlagCommand;
                item.target = application;
                item.representedObject = entry[@"id"];
                item.tag = 1;
                [app insertItem:item atIndex:0];
                [app insertItem:[NSMenuItem separatorItem] atIndex:1];
            }
            continue;
        }
        NSMenuItem *old = [bar itemWithTitle:title];
        if (old) [bar removeItem:old];
        NSMenu *menu = [[HCShortcutMenu alloc] initWithTitle:title];
        for (NSDictionary *entry in items) {
            if (![entry[@"section"] isEqual:title]) continue;
            NSDictionary *b = entry[@"binding"];
            NSString *key = b[@"key"];
            if ([key isEqual:@"ArrowDown"]) key = [NSString stringWithCharacters:(unichar[]){NSDownArrowFunctionKey} length:1];
            if ([key isEqual:@"ArrowUp"]) key = [NSString stringWithCharacters:(unichar[]){NSUpArrowFunctionKey} length:1];
            NSEventModifierFlags mods = ([b[@"metaKey"] boolValue] ? NSEventModifierFlagCommand : 0) |
                ([b[@"altKey"] boolValue] ? NSEventModifierFlagOption : 0) |
                ([b[@"ctrlKey"] boolValue] ? NSEventModifierFlagControl : 0) |
                ([b[@"shiftKey"] boolValue] ? NSEventModifierFlagShift : 0);
            NSMenuItem *item = [menu addItemWithTitle:entry[@"title"] action:@selector(shortcut:) keyEquivalent:key];
            item.keyEquivalentModifierMask = mods;
            item.target = application;
            item.representedObject = entry[@"id"];
        }
        NSMenuItem *holder = [[NSMenuItem alloc] initWithTitle:title action:nil keyEquivalent:@""];
        holder.submenu = menu;
        NSInteger index = [bar indexOfItemWithTitle:@"Window"];
        [bar insertItem:holder atIndex:[title isEqual:@"Help"] ? bar.numberOfItems : index];
        if ([title isEqual:@"Help"]) NSApp.helpMenu = menu;
    }
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
    NSString *label = count ? [NSString stringWithFormat:@"%ld", (long)count] : nil;
    NSApp.dockTile.badgeLabel = label;
    application.statusItem.button.title = label ?: @"";
    application.statusItem.button.accessibilityLabel = label
        ? [NSString stringWithFormat:@"Hypercube, %@ waiting on you", label] : @"Hypercube";
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
