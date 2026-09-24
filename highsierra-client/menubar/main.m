// AWG-HS.app is the menu bar front end for the awg-hs service.
#import <Cocoa/Cocoa.h>

#import "AppDelegate.h"

// NSApplication holds its delegate weakly, so keep it alive here.
static AppDelegate *delegate;

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        delegate = [[AppDelegate alloc] init];
        app.delegate = delegate;
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [app run];
    }
    return 0;
}
