#import <Cocoa/Cocoa.h>

@interface HypercubeMenu : NSObject
- (void)showHypercube:(NSMenuItem *)sender;
@end

@implementation HypercubeMenu
- (void)showHypercube:(NSMenuItem *)sender {
    [[NSWorkspace sharedWorkspace] openURL:[sender representedObject]];
}
@end

void hypercubeMenuRun(const char *url) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];

        NSStatusItem *item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
        NSImage *image = [NSImage imageWithSystemSymbolName:@"cube" accessibilityDescription:@"Hypercube"];
        [image setTemplate:YES];
        [image setSize:NSMakeSize(18, 18)];
        [[item button] setImage:image];

        HypercubeMenu *controller = [[HypercubeMenu alloc] init];
        NSMenu *menu = [[NSMenu alloc] init];
        NSMenuItem *show = [menu addItemWithTitle:@"Show Hypercube" action:@selector(showHypercube:) keyEquivalent:@""];
        [show setTarget:controller];
        [show setRepresentedObject:[NSURL URLWithString:[NSString stringWithUTF8String:url]]];
        [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *quit = [menu addItemWithTitle:@"Quit Hypercube" action:@selector(terminate:) keyEquivalent:@"q"];
        [quit setTarget:app];
        [item setMenu:menu];
        [menu release];

        [app run];
        [controller release];
    }
}
