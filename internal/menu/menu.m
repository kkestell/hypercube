#import <Cocoa/Cocoa.h>

@interface HypercubeMenu : NSObject
- (void)showHypercube:(NSMenuItem *)sender;
- (void)quit:(NSMenuItem *)sender;
@end

@implementation HypercubeMenu
- (void)showHypercube:(NSMenuItem *)sender {
    [[NSWorkspace sharedWorkspace] openURL:[sender representedObject]];
}

// Quit ends the application loop, so hypercubeMenuRun returns and the app shuts
// down its agents.
- (void)quit:(NSMenuItem *)sender {
    [NSApp stop:nil];
    NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint modifierFlags:0
                                      timestamp:0 windowNumber:0 context:nil subtype:0 data1:0 data2:0];
    [NSApp postEvent:wake atStart:YES];
}
@end

void hypercubeMenuRun(const char *url, const void *icon, size_t iconLen) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];

        NSStatusItem *item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
        NSImage *image = [[NSImage alloc] initWithData:[NSData dataWithBytes:icon length:iconLen]];
        [image setTemplate:YES];
        [image setSize:NSMakeSize(18, 18)];
        [[item button] setImage:image];
        [[item button] setAccessibilityLabel:@"Hypercube"];
        [image release];

        HypercubeMenu *controller = [[HypercubeMenu alloc] init];
        NSMenu *menu = [[NSMenu alloc] init];
        NSMenuItem *show = [menu addItemWithTitle:@"Show Hypercube" action:@selector(showHypercube:) keyEquivalent:@""];
        [show setTarget:controller];
        [show setRepresentedObject:[NSURL URLWithString:[NSString stringWithUTF8String:url]]];
        [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *quit = [menu addItemWithTitle:@"Quit Hypercube" action:@selector(quit:) keyEquivalent:@"q"];
        [quit setTarget:controller];
        [item setMenu:menu];
        [menu release];

        [app run];
        [controller release];
    }
}
