#import <UserNotifications/UserNotifications.h>
#import "desktop.h"

// HCNotifier posts the alerts the app process sends and answers the user's
// clicks on them.
@interface HCNotifier : NSObject <UNUserNotificationCenterDelegate>
@property HCWindow *window;
@end

static HCNotifier *notifier;

@implementation HCNotifier
// An alert that passed the focus check shows even while the app is active
// with its window not key.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)notification withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)response withCompletionHandler:(void (^)(void))completionHandler {
    NSString *session = response.notification.request.content.userInfo[@"session"];
    dispatch_async(dispatch_get_main_queue(), ^{
        [self.window show];
        [self.window dispatch:@"hypercube-notification" detail:@{@"session": session}];
    });
    completionHandler();
}
@end

// HCNotificationsStart asks for permission to show notifications and answers
// their clicks. Notifications need the application bundle; a bare executable
// shows none.
void HCNotificationsStart(HCWindow *window) {
    if (![NSBundle mainBundle].bundleIdentifier) return;
    notifier = [[HCNotifier alloc] init];
    notifier.window = window;
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
    center.delegate = notifier;
    [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound completionHandler:^(BOOL granted, NSError *error) {}];
}

void hypercubeNotify(const char *key, const char *session, const char *title, const char *subtitle, const char *text) {
    NSString *identifier = @(key), *sessionID = @(session);
    NSString *titleText = @(title), *subtitleText = @(subtitle), *body = @(text);
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!notifier || (NSApp.isActive && notifier.window.window.isKeyWindow)) return;
        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = titleText;
        content.subtitle = subtitleText;
        content.body = body;
        content.userInfo = @{@"session": sessionID};
        content.sound = [UNNotificationSound defaultSound];
        [[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:[UNNotificationRequest requestWithIdentifier:identifier content:content trigger:nil] withCompletionHandler:nil];
    });
}
